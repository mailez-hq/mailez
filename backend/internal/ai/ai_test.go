package ai

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
)

type fakeProvider struct{ out string }

func (f fakeProvider) Name() string { return "fake" }
func (f fakeProvider) Chat(_ context.Context, _, _ string) (string, error) {
	return f.out, nil
}

func TestPrioritizeParsesScores(t *testing.T) {
	m := &Manager{envFallback: fakeProvider{out: `{"1": {"score": 5, "category": "work"}, "2": {"score": 2, "category": "newsletter"}, "3": {"score": 7, "category": "other"}}`}}
	res, err := m.Prioritize(context.Background(), []PriorityItem{
		{UID: 1}, {UID: 2}, {UID: 3},
	})
	if err != nil {
		t.Fatalf("prioritize: %v", err)
	}
	if res.Scores[1] != 5 || res.Scores[2] != 2 {
		t.Errorf("scores: %v", res.Scores)
	}
	if res.Scores[3] != 5 {
		t.Errorf("score must clamp to 5, got %d", res.Scores[3])
	}
	if res.Categories[1] != "work" || res.Categories[2] != "newsletter" {
		t.Errorf("categories: %v", res.Categories)
	}
}

func TestPrioritizeToleratesFences(t *testing.T) {
	m := &Manager{envFallback: fakeProvider{out: "```json\n{\"7\": {\"score\": 4, \"category\": \"finance\"}}\n```"}}
	res, err := m.Prioritize(context.Background(), []PriorityItem{{UID: 7}})
	if err != nil {
		t.Fatalf("prioritize: %v", err)
	}
	if res.Scores[7] != 4 {
		t.Errorf("scores: %v", res.Scores)
	}
	if res.Categories[7] != "finance" {
		t.Errorf("categories: %v", res.Categories)
	}
}

func TestPrioritizeDisabled(t *testing.T) {
	m := &Manager{}
	if _, err := m.Prioritize(context.Background(), []PriorityItem{{UID: 1}}); err != ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestPrioritizeGarbageResponse(t *testing.T) {
	m := &Manager{envFallback: fakeProvider{out: "sorry, no scores"}}
	if _, err := m.Prioritize(context.Background(), []PriorityItem{{UID: 1}}); err == nil {
		t.Fatal("expected error for garbage response")
	}
}

func TestInterpretSearch(t *testing.T) {
	m := &Manager{
		envFallback: fakeProvider{
			out: `{"keywords":["contract"],"from":"amy@example.com","to":"","subject":"","has_attachment":true,"before":"2026-01-31","after":"2026-01-01"}`,
		},
	}
	spec, err := m.InterpretSearch(context.Background(), "contracts with attachments from Amy in January")
	if err != nil {
		t.Fatalf("interpret: %v", err)
	}
	if len(spec.Keywords) != 1 || spec.Keywords[0] != "contract" {
		t.Errorf("keywords: %v", spec.Keywords)
	}
	if spec.From != "amy@example.com" || !spec.HasAttachment {
		t.Errorf("spec: %+v", spec)
	}
	if spec.Before != "2026-01-31" || spec.After != "2026-01-01" {
		t.Errorf("dates: %+v", spec)
	}
}

func TestSmartReplies(t *testing.T) {
	m := &Manager{envFallback: fakeProvider{out: `["好的，收到！", "我稍后处理，谢谢。", "可以，就这么办。"]`}}
	replies, err := m.SmartReplies(context.Background(), "请帮忙确认一下明天的会议时间")
	if err != nil {
		t.Fatalf("smart replies: %v", err)
	}
	if len(replies) != 3 {
		t.Fatalf("replies = %v", replies)
	}
	if replies[0] != "好的，收到！" {
		t.Fatalf("first reply = %q", replies[0])
	}
}

func TestSmartRepliesBulletFallback(t *testing.T) {
	m := &Manager{envFallback: fakeProvider{out: "- Thanks, noted!\n- Will do!"}}
	replies, err := m.SmartReplies(context.Background(), "reminder")
	if err != nil {
		t.Fatalf("smart replies: %v", err)
	}
	if len(replies) != 2 || replies[0] != "Thanks, noted!" {
		t.Fatalf("replies = %v", replies)
	}
}

func TestSmartRepliesDisabled(t *testing.T) {
	m := &Manager{}
	if _, err := m.SmartReplies(context.Background(), "hi"); err != ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestDraftNewWritesFromSubjectAndHint(t *testing.T) {
	m := &Manager{envFallback: fakeProvider{out: "确认参加，明天见。"}}
	draft, err := m.DraftNew(context.Background(), ToneFormal, "会议确认", "确认参加，并说明会提前到场")
	if err != nil {
		t.Fatalf("draft new: %v", err)
	}
	if draft != "确认参加，明天见。" {
		t.Fatalf("draft = %q", draft)
	}
}

func TestDraftNewDisabled(t *testing.T) {
	m := &Manager{}
	if _, err := m.DraftNew(context.Background(), ToneConcise, "主题", ""); err != ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestComposeFromInstructionParsesJSON(t *testing.T) {
	m := &Manager{envFallback: fakeProvider{out: `{"to":["小明"],"subject":"下周不去旅游了","body":"小明，我下周有事去不了旅游了，抱歉。"}`}}
	draft, err := m.ComposeFromInstruction(context.Background(), "给小明写封邮件，说我下周不去旅游了")
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if len(draft.To) != 1 || draft.To[0] != "小明" || draft.Subject == "" || draft.Body == "" {
		t.Fatalf("draft = %+v", draft)
	}
}

func TestComposeFromInstructionToleratesFences(t *testing.T) {
	m := &Manager{envFallback: fakeProvider{out: "```json\n{\"to\":[\"a@example.com\"],\"subject\":\"s\",\"body\":\"b\"}\n```"}}
	draft, err := m.ComposeFromInstruction(context.Background(), "写邮件给 a@example.com")
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	if len(draft.To) != 1 || draft.To[0] != "a@example.com" {
		t.Fatalf("draft = %+v", draft)
	}
}

func TestComposeFromInstructionDisabled(t *testing.T) {
	m := &Manager{}
	if _, err := m.ComposeFromInstruction(context.Background(), "给小明写封邮件"); err != ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}

func TestLoadPrefersDefaultEnabledProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ai.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	seed := []models.AiConfig{
		{Name: "b", Enabled: true, IsDefault: false, Provider: "openai", BaseURL: "https://b.invalid", Model: "m"},
		{Name: "a", Enabled: true, IsDefault: true, Provider: "openai", BaseURL: "https://a.invalid", Model: "m"},
		{Name: "off", Enabled: false, IsDefault: false, Provider: "openai", BaseURL: "https://off.invalid", Model: "m"},
	}
	for i := range seed {
		if err := db.Create(&seed[i]).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	m := &Manager{db: db, secretKey: "test-secret"}
	p := m.load()
	if p == nil {
		t.Fatal("load returned nil")
	}
	if got := p.(*OpenAI).baseURL; got != "https://a.invalid" {
		t.Fatalf("default provider base = %q, want https://a.invalid", got)
	}
}

func TestLoadFallsBackToFirstEnabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ai.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&models.AiConfig{Name: "x", Enabled: true, IsDefault: false, Provider: "openai", BaseURL: "https://x.invalid", Model: "m"}).Error; err != nil {
		t.Fatalf("seed x: %v", err)
	}
	if err := db.Create(&models.AiConfig{Name: "y", Enabled: true, IsDefault: false, Provider: "openai", BaseURL: "https://y.invalid", Model: "m"}).Error; err != nil {
		t.Fatalf("seed y: %v", err)
	}
	m := &Manager{db: db, secretKey: "test-secret"}
	p := m.load()
	if p == nil {
		t.Fatal("load returned nil")
	}
	if got := p.(*OpenAI).baseURL; got != "https://x.invalid" {
		t.Fatalf("fallback base = %q, want https://x.invalid", got)
	}
}

func TestLoadDisabledReturnsNil(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ai.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&models.AiConfig{Name: "off", Enabled: false, Provider: "openai", BaseURL: "https://off.invalid", Model: "m"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := &Manager{db: db, secretKey: "test-secret"}
	if m.load() != nil {
		t.Fatal("disabled row must not produce a provider")
	}
}

func TestInterpretSearchFences(t *testing.T) {
	m := &Manager{envFallback: fakeProvider{out: "```json\n{\"keywords\":[\"invoice\"]}\n```"}}
	spec, err := m.InterpretSearch(context.Background(), "invoices")
	if err != nil {
		t.Fatalf("interpret: %v", err)
	}
	if len(spec.Keywords) != 1 || spec.Keywords[0] != "invoice" {
		t.Errorf("spec: %+v", spec)
	}
}

func TestInterpretSearchDisabled(t *testing.T) {
	m := &Manager{}
	if _, err := m.InterpretSearch(context.Background(), "anything"); err != ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}
