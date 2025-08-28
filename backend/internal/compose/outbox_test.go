package compose

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/password"
)

// fakeSentSaver records IMAP APPEND calls the worker would make to keep a
// copy in the sender's Sent folder.
type fakeSentSaver struct {
	appends   []appendCall
	lastToken string
}

type appendCall struct {
	email  string
	folder string
	raw    string
}

func (f *fakeSentSaver) AppendRaw(email, token, folder, raw string, flags []string) error {
	f.appends = append(f.appends, appendCall{email: email, folder: folder, raw: raw})
	f.lastToken = token
	return nil
}

func newOutboxTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "outbox.db")), &gorm.Config{
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
	return db
}

func TestSaveSentAppendsCopy(t *testing.T) {
	db := newOutboxTestDB(t)
	saver := &fakeSentSaver{}
	w := NewOutboxWorker(db, "mta:25", "test-secret", nil, saver)

	raw := "From: u1@example.com\r\nTo: admin@example.com\r\nSubject: hi\r\n\r\nbody\r\n"
	w.saveSent(context.Background(), models.Outbox{
		ID:           1,
		AccountEmail: "u1@example.com",
		AccountID:    0,
		RawMessage:   raw,
	})

	if len(saver.appends) != 1 {
		t.Fatalf("appends = %d, want 1", len(saver.appends))
	}
	got := saver.appends[0]
	if got.email != "u1@example.com" || got.folder != "Sent" || got.raw != raw {
		t.Fatalf("append = %+v, want u1@example.com/Sent with the delivered raw", got)
	}

	// A worker app token must be provisioned and hash-verifiable, and the
	// encrypted secret persisted so a restart keeps the credential.
	var tok models.Token
	if err := db.First(&tok, "user_email = ?", "u1@example.com").Error; err != nil {
		t.Fatalf("token row: %v", err)
	}
	if tok.IP != "outbox-worker" || !password.VerifyPBKDF2SHA256(tok.Password, saver.lastToken) {
		t.Fatalf("token row %+v does not match the appended credential", tok)
	}
	var wt models.WorkerToken
	if err := db.First(&wt, "user_email = ?", "u1@example.com").Error; err != nil {
		t.Fatalf("worker token row: %v", err)
	}
	if secret, err := crypto.Decrypt("test-secret", wt.TokenEnc); err != nil || secret != saver.lastToken {
		t.Fatalf("decrypted worker token %q (err %v), want %q", secret, err, saver.lastToken)
	}
}

func TestSaveSentReusesTokenAcrossSends(t *testing.T) {
	db := newOutboxTestDB(t)
	saver := &fakeSentSaver{}
	w := NewOutboxWorker(db, "mta:25", "test-secret", nil, saver)

	for i := 0; i < 2; i++ {
		w.saveSent(context.Background(), models.Outbox{
			ID:           uint(i + 1),
			AccountEmail: "u1@example.com",
			RawMessage:   "raw\r\n",
		})
	}
	if len(saver.appends) != 2 {
		t.Fatalf("appends = %d, want 2", len(saver.appends))
	}
	if saver.appends[0].raw != saver.appends[1].raw {
		t.Fatalf("second send did not keep its own copy: %+v", saver.appends)
	}
	var tokens []models.Token
	if err := db.Find(&tokens, "user_email = ?", "u1@example.com").Error; err != nil {
		t.Fatalf("list tokens: %v", err)
	}
	if len(tokens) != 1 {
		t.Fatalf("token rows = %d, want 1 (reused across sends)", len(tokens))
	}
}

func TestSaveSentPersistsAcrossRestart(t *testing.T) {
	db := newOutboxTestDB(t)
	saver1 := &fakeSentSaver{}
	w1 := NewOutboxWorker(db, "mta:25", "test-secret", nil, saver1)
	w1.saveSent(context.Background(), models.Outbox{
		ID: 1, AccountEmail: "u1@example.com", RawMessage: "raw\r\n",
	})

	// A restarted worker starts with an empty cache and must recover the
	// same credential from worker_token instead of minting a new one.
	saver2 := &fakeSentSaver{}
	w2 := NewOutboxWorker(db, "mta:25", "test-secret", nil, saver2)
	w2.saveSent(context.Background(), models.Outbox{
		ID: 2, AccountEmail: "u1@example.com", RawMessage: "raw\r\n",
	})

	if saver1.lastToken != saver2.lastToken {
		t.Fatalf("token changed across restart: %q != %q", saver1.lastToken, saver2.lastToken)
	}
	var count int64
	if err := db.Model(&models.Token{}).Where("user_email = ?", "u1@example.com").Count(&count).Error; err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if count != 1 {
		t.Fatalf("token rows after restart = %d, want 1", count)
	}
}

func TestSaveSentSkipsExternalAccount(t *testing.T) {
	db := newOutboxTestDB(t)
	saver := &fakeSentSaver{}
	w := NewOutboxWorker(db, "mta:25", "test-secret", nil, saver)

	w.saveSent(context.Background(), models.Outbox{
		ID: 1, AccountEmail: "u1@example.com", AccountID: 7, RawMessage: "raw\r\n",
	})
	if len(saver.appends) != 0 {
		t.Fatalf("external account got a Sent copy: %+v", saver.appends)
	}
	var count int64
	if err := db.Model(&models.Token{}).Count(&count).Error; err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if count != 0 {
		t.Fatalf("token rows = %d, want 0 for external account", count)
	}
}

func TestSaveSentSkipsWithoutMailClient(t *testing.T) {
	db := newOutboxTestDB(t)
	w := NewOutboxWorker(db, "mta:25", "test-secret", nil, nil)
	w.saveSent(context.Background(), models.Outbox{
		ID: 1, AccountEmail: "u1@example.com", RawMessage: "raw\r\n",
	})
	var count int64
	if err := db.Model(&models.Token{}).Count(&count).Error; err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if count != 0 {
		t.Fatalf("token rows = %d, want 0 when no mail client is wired", count)
	}
}

func TestWorkerTokenRoundTrip(t *testing.T) {
	db := newOutboxTestDB(t)
	w := NewOutboxWorker(db, "mta:25", "test-secret", nil, &fakeSentSaver{})

	tok, err := w.workerToken(context.Background(), "a@example.com")
	if err != nil {
		t.Fatalf("workerToken: %v", err)
	}
	// App tokens are 32-char hex.
	if len(tok) != 32 {
		t.Fatalf("token %q has length %d, want 32", tok, len(tok))
	}
}
