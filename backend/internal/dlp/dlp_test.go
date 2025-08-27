package dlp

import (
	"context"
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func newTestDLP(t *testing.T) *Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "dlp.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	app := core.New(db, auth.NewManager(db, auth.NewMemoryStore(), "s", time.Hour), core.Config{Domain: "example.com"})
	return New(app)
}

func rule(name, pattern, action, scope string, approvers ...string) models.DlpRule {
	r := models.DlpRule{
		Name: name, Pattern: pattern, Enabled: true, Action: models.DlpAction(action),
		Scope: scope, Severity: "medium", HoldHours: 48,
	}
	if len(approvers) > 0 {
		r.Approvers = strings.Join(approvers, ",")
	}
	return r
}

func TestCheckPassNoRules(t *testing.T) {
	s := newTestDLP(t)
	res, err := s.CheckRaw(context.Background(), "alice@example.com", "alice@example.com",
		[]string{"bob@other.test"}, []byte("From: alice@example.com\r\nSubject: hi\r\n\r\nhello\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "pass" {
		t.Fatalf("action = %q, want pass", res.Action)
	}
}

func TestCheckBlock(t *testing.T) {
	s := newTestDLP(t)
	r := rule("机密", "机密", "block", "all")
	if err := s.DB.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	res, err := s.CheckRaw(context.Background(), "alice@example.com", "alice@example.com",
		[]string{"bob@other.test"}, []byte("Subject: 报告\r\n\r\n本邮件包含机密内容，请勿外发。\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "block" || !strings.Contains(res.Reason, "机密") {
		t.Fatalf("got %+v", res)
	}
}

func TestCheckHoldCreatesApproval(t *testing.T) {
	s := newTestDLP(t)
	var notices int
	s.send = func(from string, to []string, raw []byte) error {
		notices++
		if len(to) != 1 || to[0] != "approver@example.com" {
			t.Errorf("notice recipient = %v", to)
		}
		return nil
	}
	r := rule("涉密", "涉密", "hold", "all", "approver@example.com")
	if err := s.DB.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	res, err := s.CheckRaw(context.Background(), "alice@example.com", "alice@example.com",
		[]string{"bob@other.test"}, []byte("Subject: 合作\r\n\r\n涉及涉密信息需要审批。\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "hold" || res.ID == 0 {
		t.Fatalf("got %+v", res)
	}
	var p models.PendingApproval
	if err := s.DB.First(&p, res.ID).Error; err != nil {
		t.Fatal(err)
	}
	if p.Status != "pending" || p.Recipients != "bob@other.test" {
		t.Errorf("pending = %+v", p)
	}
	if notices != 1 {
		t.Errorf("notices = %d, want 1", notices)
	}
}

func TestCheckScopeRestrictsDomain(t *testing.T) {
	s := newTestDLP(t)
	r := rule("部门机密", "部门机密", "block", "domain:other.com")
	if err := s.DB.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	res, err := s.CheckRaw(context.Background(), "alice@example.com", "alice@example.com",
		[]string{"bob@other.test"}, []byte("Subject: 部门机密\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "pass" {
		t.Fatalf("action = %q, want pass (rule scoped elsewhere)", res.Action)
	}
}

func TestCheckBlockScansBase64Body(t *testing.T) {
	s := newTestDLP(t)
	r := rule("身份证", `\d{17}[\dXx]`, "block", "all")
	r.IsRegex = true
	if err := s.DB.Create(&r).Error; err != nil {
		t.Fatal(err)
	}
	body := base64.StdEncoding.EncodeToString([]byte("附件里包含身份证号 11010119900307755X"))
	raw := "Subject: 资料\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: base64\r\n\r\n" + body + "\r\n"
	res, err := s.CheckRaw(context.Background(), "alice@example.com", "alice@example.com",
		[]string{"bob@other.test"}, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "block" {
		t.Fatalf("action = %q, want block (base64 body should be decoded)", res.Action)
	}
}

func TestDecideApproveDelivers(t *testing.T) {
	s := newTestDLP(t)
	raw := []byte("From: alice@example.com\r\nTo: bob@other.test\r\nSubject: 审批\r\n\r\n正文\r\n")
	p := &models.PendingApproval{
		RuleName: "涉密", SenderEmail: "alice@example.com", From: "alice@example.com",
		Recipients: "bob@other.test", Subject: "审批", Raw: raw, Status: "pending",
	}
	if err := s.DB.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	var sent []string
	s.send = func(from string, to []string, data []byte) error {
		sent = to
		if string(data) != string(raw) {
			t.Errorf("delivered bytes mismatch")
		}
		return nil
	}
	if _, err := s.decideInternal(p.ID, "approve", "", "approver@example.com"); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0] != "bob@other.test" {
		t.Fatalf("sent = %v", sent)
	}
	if err := s.DB.First(p, p.ID).Error; err != nil {
		t.Fatal(err)
	}
	if p.Status != "approved" {
		t.Fatalf("status = %q", p.Status)
	}
}

func TestDecideRejectNotifiesSender(t *testing.T) {
	s := newTestDLP(t)
	raw := []byte("From: alice@example.com\r\nTo: bob@other.test\r\nSubject: 审批\r\n\r\n正文\r\n")
	p := &models.PendingApproval{
		RuleName: "涉密", SenderEmail: "alice@example.com", From: "alice@example.com",
		Recipients: "bob@other.test", Subject: "审批", Raw: raw, Status: "pending",
	}
	if err := s.DB.Create(p).Error; err != nil {
		t.Fatal(err)
	}
	var noticeTo []string
	s.send = func(from string, to []string, data []byte) error {
		noticeTo = to
		return nil
	}
	if _, err := s.decideInternal(p.ID, "reject", "领导不同意", "approver@example.com"); err != nil {
		t.Fatal(err)
	}
	if len(noticeTo) != 1 || noticeTo[0] != "alice@example.com" {
		t.Fatalf("notice = %v", noticeTo)
	}
	if err := s.DB.First(p, p.ID).Error; err != nil {
		t.Fatal(err)
	}
	if p.Status != "rejected" || p.Reason != "领导不同意" {
		t.Fatalf("pending = %+v", p)
	}
}
