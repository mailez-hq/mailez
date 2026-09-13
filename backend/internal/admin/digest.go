// Administrator digest email: a daily (or weekly) operations summary in the
// Mail-in-a-Box spirit — counts, recent activity, failed logins and the
// domain health verdict, delivered to the admin mailbox through the engine
// MTA. One replica sends it (DB lease) and a system flag dedupes per day.
package admin

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/cluster"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
)

const (
	flagDigestDaily  = "digest:daily:last"
	flagDigestWeekly = "digest:weekly:last"
)

// DigestWorker sends the admin summary email on schedule.
type DigestWorker struct {
	DB  *gorm.DB
	Cfg core.Config
}

// NewDigestWorker wires the worker.
func NewDigestWorker(db *gorm.DB, cfg core.Config) *DigestWorker {
	return &DigestWorker{DB: db, Cfg: cfg}
}

// Run ticks until cancelled; the actual send happens once per day inside the
// configured hour window.
func (w *DigestWorker) Run(ctx context.Context) {
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.flush()
		}
	}
}

func (w *DigestWorker) flush() {
	if w.Cfg.AdminDigest != "daily" && w.Cfg.AdminDigest != "weekly" {
		return
	}
	// Singleton across replicas.
	if !cluster.TryHold(w.DB, "admin_digest", cluster.LeaseTTL) {
		return
	}
	now := time.Now()
	if now.Hour() != w.Cfg.DigestHour {
		return
	}
	today := now.Format("2006-01-02")
	if w.Cfg.AdminDigest == "daily" {
		if w.flag(flagDigestDaily) == today {
			return
		}
		w.send(false, today)
		w.setFlag(flagDigestDaily, today)
		return
	}
	// Weekly: Monday only.
	if now.Weekday() != time.Monday {
		return
	}
	week := now.Format("2006-01-02")
	if w.flag(flagDigestWeekly) == week {
		return
	}
	w.send(true, week)
	w.setFlag(flagDigestWeekly, week)
}

func (w *DigestWorker) flag(key string) string {
	var f models.SystemFlag
	if err := w.DB.First(&f, "key = ?", key).Error; err != nil {
		return ""
	}
	return f.Value
}

func (w *DigestWorker) setFlag(key, value string) {
	if err := w.DB.Save(&models.SystemFlag{Key: key, Value: value}).Error; err != nil {
		log.Printf("admin digest: save flag %s: %v", key, err)
	}
}

// recipient resolves the digest mailbox: explicit config wins, else the
// first enabled global admin.
func (w *DigestWorker) recipient() string {
	if w.Cfg.AdminEmail != "" {
		return w.Cfg.AdminEmail
	}
	var u models.User
	if err := w.DB.First(&u, "global_admin = ? AND enabled = ?", true, true).Error; err != nil {
		return ""
	}
	return u.Email
}

// send builds and delivers the digest. weekly widens the activity window to
// seven days.
func (w *DigestWorker) send(weekly bool, label string) {
	to := w.recipient()
	if to == "" {
		log.Printf("admin digest: no recipient, skipping")
		return
	}
	since := time.Now().Add(-24 * time.Hour)
	window := "24 小时"
	if weekly {
		since = time.Now().AddDate(0, 0, -7)
		window = "7 天"
	}

	var users, enabled, domains, aliases, newUsers int64
	w.DB.Model(&models.User{}).Count(&users)
	w.DB.Model(&models.User{}).Where("enabled = ?", true).Count(&enabled)
	w.DB.Model(&models.Domain{}).Count(&domains)
	w.DB.Model(&models.Alias{}).Where("disabled = ?", false).Count(&aliases)
	w.DB.Model(&models.User{}).Where("created_at >= ?", since).Count(&newUsers)

	var logins, loginFails, sends int64
	w.DB.Model(&models.AuditLog{}).Where(
		"(path = ? OR action LIKE ?) AND status = ?", "/sso/login", "login%", 200,
	).Where("created_at >= ?", since).Count(&logins)
	w.DB.Model(&models.AuditLog{}).Where(
		"path = ? AND status = ?", "/sso/login", 401,
	).Where("created_at >= ?", since).Count(&loginFails)
	w.DB.Model(&models.AuditLog{}).Where(
		"path = ? AND status = ?", "/mail/send", 200,
	).Where("created_at >= ?", since).Count(&sends)

	// Health verdict reuses the health-center checks.
	checker := &Handler{App: &core.App{DB: w.DB, Cfg: w.Cfg}}
	var dmodels []models.Domain
	w.DB.Order("name").Find(&dmodels)
	var problemDomains []string
	fails, warns := 0, 0
	for _, d := range dmodels {
		report := checker.checkDomain(context.Background(), d)
		for _, item := range report.Items {
			switch item.Status {
			case statusFail:
				fails++
			case statusWarn, statusUnknown:
				warns++
			}
		}
		if failsPerDomain(report) > 0 {
			problemDomains = append(problemDomains, fmt.Sprintf("%s（%s）", report.Domain, strings.Join(failedIDs(report), "/")))
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Mailez 运维摘要 %s\n\n", label)
	fmt.Fprintf(&b, "统计窗口：%s\n\n", window)
	fmt.Fprintf(&b, "账号：%d（启用 %d），域名：%d，别名：%d\n", users, enabled, domains, aliases)
	fmt.Fprintf(&b, "新增账号：%d\n", newUsers)
	fmt.Fprintf(&b, "登录：%d 次（失败尝试 %d 次）\n", logins, loginFails)
	fmt.Fprintf(&b, "发信：%d 封\n\n", sends)
	fmt.Fprintf(&b, "域名体检：%d 项失败，%d 项警告\n", fails, warns)
	if len(problemDomains) > 0 {
		fmt.Fprintf(&b, "需关注：%s\n", strings.Join(problemDomains, "、"))
	}
	b.WriteString("\n详情见管理后台「健康体检」页面。\n")

	subject := "Mailez 运维摘要 " + label
	raw := mail.BuildMessage(to, []string{to}, nil, subject, b.String(), "", nil)
	if err := mail.SubmitMTA(w.Cfg.MailMtaAddr, to, []string{to}, raw); err != nil {
		log.Printf("admin digest: submit to %s: %v", to, err)
	}
}

func failsPerDomain(r DomainReport) int {
	n := 0
	for _, item := range r.Items {
		if item.Status == statusFail {
			n++
		}
	}
	return n
}

func failedIDs(r DomainReport) []string {
	var ids []string
	for _, item := range r.Items {
		if item.Status == statusFail {
			ids = append(ids, item.ID)
		}
	}
	return ids
}
