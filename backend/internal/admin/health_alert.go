// Change-based health alerting. A worker re-runs the health-check set on a
// schedule, diffs every probe's status against the last announced state and
// reports only transitions — degradation alerts, plus a recovery notice
// when a probe returns to ok. A newly observed status must hold for two
// consecutive runs before it is announced, so a one-off DNS timeout does
// not page anyone. The first run after startup only records a baseline.
package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/cluster"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
)

// HealthAlertWorker diffs health-check runs and reports the transitions.
type HealthAlertWorker struct {
	DB  *gorm.DB
	Cfg core.Config
}

// NewHealthAlertWorker wires the worker.
func NewHealthAlertWorker(db *gorm.DB, cfg core.Config) *HealthAlertWorker {
	return &HealthAlertWorker{DB: db, Cfg: cfg}
}

// Run ticks until cancelled.
func (w *HealthAlertWorker) Run(ctx context.Context) {
	if w.Cfg.HealthAlert == "off" {
		return
	}
	minutes := w.Cfg.HealthAlertIntervalMin
	if minutes < 5 {
		minutes = 5
	}
	t := time.NewTicker(time.Duration(minutes) * time.Minute)
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

func (w *HealthAlertWorker) flush() {
	// Singleton across replicas.
	if !cluster.TryHold(w.DB, "health_alert", cluster.LeaseTTL) {
		return
	}
	checker := &Handler{App: &core.App{DB: w.DB, Cfg: w.Cfg}}
	observed := collectProbes(checker)

	var stored []models.HealthSnapshot
	if err := w.DB.Find(&stored).Error; err != nil {
		log.Printf("health alert: load snapshots: %v", err)
		return
	}
	byKey := make(map[string]models.HealthSnapshot, len(stored))
	for _, s := range stored {
		byKey[s.Key] = s
	}
	events, rows, drop := reconcileProbes(byKey, observed, mutedKeys(w.Cfg.HealthAlertMute), time.Now().UTC())
	for i := range rows {
		if err := w.DB.Save(&rows[i]).Error; err != nil {
			log.Printf("health alert: save snapshot %s: %v", rows[i].Key, err)
		}
	}
	for _, key := range drop {
		if err := w.DB.Delete(&models.HealthSnapshot{Key: key}).Error; err != nil {
			log.Printf("health alert: drop snapshot %s: %v", key, err)
		}
	}
	if len(events) == 0 {
		return
	}
	if w.Cfg.HealthAlertWebhook != "" {
		w.postWebhook(events)
	}
	w.sendMail(events)
}

// probeKey identifies one check: "system:<id>" or "domain:<name>:<id>".
// collectProbes runs the same checks the health center reports, keyed for
// diffing.
func collectProbes(h *Handler) map[string]CheckItem {
	out := map[string]CheckItem{}
	ctx := context.Background()
	for _, item := range h.systemChecks(ctx) {
		out["system:"+item.ID] = item
	}
	var domains []models.Domain
	h.DB.Order("name").Find(&domains)
	for _, d := range domains {
		report := h.checkDomain(ctx, d)
		for _, item := range report.Items {
			out["domain:"+d.Name+":"+item.ID] = item
		}
	}
	return out
}

// HealthEvent is one announced transition.
type HealthEvent struct {
	Key    string `json:"key"`
	From   string `json:"from"`
	To     string `json:"to"`
	Detail string `json:"detail,omitempty"`
	Since  string `json:"since"`
}

// reconcileProbes is the pure heart of the alerter: stored holds the last
// announced state per probe key, observed the fresh run. It returns the
// transitions to announce, the snapshot rows to persist, and the keys whose
// probes vanished (removed domains) and should be forgotten.
func reconcileProbes(stored map[string]models.HealthSnapshot, observed map[string]CheckItem, muted map[string]bool, now time.Time) (events []HealthEvent, rows []models.HealthSnapshot, drop []string) {
	for key, item := range observed {
		if muted[key] {
			continue
		}
		snap, exists := stored[key]
		if !exists {
			// First sight: record a baseline, stay silent.
			rows = append(rows, models.HealthSnapshot{Key: key, Status: item.Status, Since: now, UpdatedAt: now})
			continue
		}
		updated := snap
		updated.UpdatedAt = now
		switch {
		case item.Status == snap.Status:
			// Settled back to the announced status.
			updated.Pending = ""
		case snap.Pending == item.Status:
			// The candidate status held for a second consecutive run:
			// announce the transition.
			events = append(events, HealthEvent{
				Key:    key,
				From:   snap.Status,
				To:     item.Status,
				Detail: item.Detail,
				Since:  snap.PendingSince.Format(time.RFC3339),
			})
			updated.Status = item.Status
			updated.Since = snap.PendingSince
			updated.Pending = ""
		default:
			// New candidate status; wait for the next run to confirm.
			updated.Pending = item.Status
			updated.PendingSince = now
		}
		rows = append(rows, updated)
	}
	for key := range stored {
		if _, alive := observed[key]; !alive && !muted[key] {
			drop = append(drop, key)
		}
	}
	return events, rows, drop
}

func mutedKeys(csv string) map[string]bool {
	out := map[string]bool{}
	for _, k := range strings.Split(csv, ",") {
		if k = strings.TrimSpace(k); k != "" {
			out[k] = true
		}
	}
	return out
}

func (w *HealthAlertWorker) postWebhook(events []HealthEvent) {
	payload, err := json.Marshal(map[string]any{
		"type":       "health_alert",
		"host":       w.Cfg.Hostname,
		"checked_at": time.Now().UTC().Format(time.RFC3339),
		"events":     events,
	})
	if err != nil {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(w.Cfg.HealthAlertWebhook, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("health alert: webhook: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("health alert: webhook answered %d", resp.StatusCode)
	}
}

func (w *HealthAlertWorker) sendMail(events []HealthEvent) {
	to := adminRecipient(w.DB, w.Cfg)
	if to == "" {
		log.Printf("health alert: no recipient, skipping email")
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Mailez 健康告警 %s\n\n", time.Now().Format("2006-01-02 15:04"))
	for _, e := range events {
		fmt.Fprintf(&b, "· %s：%s → %s（%s）\n", probeLabel(e.Key), e.From, e.To, e.Detail)
	}
	b.WriteString("\n恢复为正常的项目也在上面列出。详情见管理后台「健康体检」页面。\n")
	subject := "Mailez 健康告警"
	if len(events) == 1 && events[0].To == statusOK {
		subject = "Mailez 健康恢复"
	}
	raw := mail.BuildMessage(to, []string{to}, nil, subject, b.String(), "", nil)
	if err := mail.SubmitMTA(w.Cfg.MailMtaAddr, to, []string{to}, raw); err != nil {
		log.Printf("health alert: submit to %s: %v", to, err)
	}
}

// probeLabel renders a probe key for the plain-text mail body:
// domain:example.com:mx → example.com · mx, system:cert → 系统 · cert.
func probeLabel(key string) string {
	if s, ok := strings.CutPrefix(key, "domain:"); ok {
		if i := strings.LastIndex(s, ":"); i >= 0 {
			return s[:i] + " · " + s[i+1:]
		}
		return s
	}
	if s, ok := strings.CutPrefix(key, "system:"); ok {
		return "系统 · " + s
	}
	return key
}

// adminRecipient resolves the operations mailbox: explicit config wins,
// else the first enabled global admin.
func adminRecipient(db *gorm.DB, cfg core.Config) string {
	if cfg.AdminEmail != "" {
		return cfg.AdminEmail
	}
	var u models.User
	if err := db.First(&u, "global_admin = ? AND enabled = ?", true, true).Error; err != nil {
		return ""
	}
	return u.Email
}
