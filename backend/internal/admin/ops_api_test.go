// API-level integration tests for the operations features: the ban flow
// from failed logins through the admin list/lift endpoints, the backup
// trigger/verify endpoints against a real SQLite control plane, and the
// shape of the health report. Everything runs through a real Fiber app on
// a temp database — no mocks.
package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/ban"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

var opsUserHash = sync.OnceValue(func() string {
	h, err := password.Hash("secret123")
	if err != nil {
		panic(err)
	}
	return h
})

// newOpsApp builds a composite app: public SSO routes (real manager, so
// login failures feed the ban engine) plus the admin routes behind an
// authenticated group. The config hook lets tests switch features on.
func newOpsApp(t *testing.T, mutate func(*core.Config)) (*fiber.App, *gorm.DB) {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "ops.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
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
	if err := db.Create(&models.Domain{Name: "t.example", MaxUsers: 5, MaxAliases: 5}).Error; err != nil {
		t.Fatalf("seed domain: %v", err)
	}
	if err := db.Create(&models.User{
		Email: "root@t.example", Localpart: "root", DomainName: "t.example",
		Password: opsUserHash(), Enabled: true, GlobalAdmin: true,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	cfg := core.Config{
		SecretKey: "ops-test-secret",
		DBDriver:  "sqlite",
		DBDSN:     dsn,
		UploadDir: filepath.Join(dir, "uploads"),
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o700); err != nil {
		t.Fatalf("mkdir uploads: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.UploadDir, "hello.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write upload: %v", err)
	}
	if mutate != nil {
		mutate(&cfg)
	}

	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	// Keep the stock brute-force limiters out of the way: the ban engine
	// under test is the only thing that should answer 429 here.
	mgr.SetLoginLimits(100, 100)
	mgr.Bans = ban.New(db, mgr.Store, cfg)

	app := core.New(db, mgr, cfg)
	h := New(app)
	f := fiber.New()
	mgr.RegisterSSO(f)
	authed := f.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "root@t.example", DomainName: "t.example", Enabled: true, GlobalAdmin: true})
		return c.Next()
	})
	h.Register(authed)
	return f, db
}

func loginOps(t *testing.T, app *fiber.App, email, pw string) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "pw": pw})
	req := httptest.NewRequest(http.MethodPost, "/sso/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 10000)
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func getOps(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil), 20000)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// Full ban lifecycle: repeated failures ban the source IP, the admin list
// shows the record, lifting it restores access, and a successful login
// resets the failure counter so old misses cannot stack into a new ban.
func TestOpsBanLifecycle(t *testing.T) {
	app, _ := newOpsApp(t, func(c *core.Config) {
		c.BanMaxRetry = 3
		c.BanFindTimeSec = 600
	})

	// Three wrong passwords trip the threshold.
	for i := 0; i < 3; i++ {
		if code, _ := loginOps(t, app, "root@t.example", "wrong"); code != http.StatusUnauthorized {
			t.Fatalf("wrong login %d: status = %d, want 401", i+1, code)
		}
	}
	// The fourth attempt is refused before credentials are checked.
	code, body := loginOps(t, app, "root@t.example", "wrong")
	if code != http.StatusTooManyRequests || !strings.Contains(body, "banned due to authentication failures") {
		t.Fatalf("banned login: status = %d body = %s", code, body)
	}

	// The admin list shows one active web ban.
	code, body = getOps(t, app, "/api/v1/admin/bans")
	if code != http.StatusOK {
		t.Fatalf("ban list: status = %d", code)
	}
	var bans []models.BanRecord
	if err := json.Unmarshal([]byte(body), &bans); err != nil {
		t.Fatalf("ban list json: %v (%s)", err, body)
	}
	if len(bans) != 1 || bans[0].Surface != "web" || !bans[0].Until.After(time.Now()) {
		t.Fatalf("ban list = %+v", bans)
	}

	// Lifting via the admin endpoint ends the ban.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/bans/"+itoa(int(bans[0].ID)), nil)
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Fatalf("lift: status = %d err = %v", resp.StatusCode, err)
	}
	if code, body := getOps(t, app, "/api/v1/admin/bans"); code != http.StatusOK || strings.Contains(body, "web") {
		t.Fatalf("ban list after lift: %d %s", code, body)
	}
	// Access is back to credential-level rejection.
	if code, _ := loginOps(t, app, "root@t.example", "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("post-lift wrong login: status = %d, want 401", code)
	}

	// A successful login clears the failure counter: with one pre-reset
	// miss, two further misses stay below the threshold. A broken reset
	// would stack them to three and silently re-ban the IP.
	if code, _ := loginOps(t, app, "root@t.example", "secret123"); code != http.StatusOK {
		t.Fatalf("correct login: status = %d", code)
	}
	if code, _ := loginOps(t, app, "root@t.example", "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("post-reset wrong login: %d", code)
	}
	if code, body := loginOps(t, app, "root@t.example", "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("stacked failures banned anyway: status = %d body = %s", code, body)
	}
	if code, body := getOps(t, app, "/api/v1/admin/bans"); strings.Contains(body, "web") || code != http.StatusOK {
		t.Fatalf("unexpected ban after reset: %d %s", code, body)
	}
}

type opsBackupRun struct {
	ID         uint       `json:"id"`
	FinishedAt *time.Time `json:"finished_at"`
	OK         bool       `json:"ok"`
	Size       int64      `json:"size"`
	Target     string     `json:"target"`
	Detail     string     `json:"detail"`
}

type opsBackupStatus struct {
	Configured bool           `json:"configured"`
	Target     string         `json:"target"`
	KeySet     bool           `json:"key_set"`
	Runs       []opsBackupRun `json:"runs"`
}

// Backup flow through the API: trigger, wait for the run to finish, then
// verify the published archive.
func TestOpsBackupFlow(t *testing.T) {
	app, _ := newOpsApp(t, func(c *core.Config) {
		c.BackupTarget = "local:" + filepath.Join(t.TempDir(), "backups")
		c.BackupKey = "ops-backup-secret"
		c.BackupKeep = 5
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backup/run", nil)
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != http.StatusAccepted {
		t.Fatalf("trigger: status = %d err = %v", resp.StatusCode, err)
	}

	var status opsBackupStatus
	deadline := time.Now().Add(30 * time.Second)
	for {
		code, body := getOps(t, app, "/api/v1/admin/backup")
		if code != http.StatusOK {
			t.Fatalf("status: %d %s", code, body)
		}
		status = opsBackupStatus{}
		if err := json.Unmarshal([]byte(body), &status); err != nil {
			t.Fatalf("status json: %v (%s)", err, body)
		}
		if !status.Configured || !status.KeySet {
			t.Fatalf("status misreports config: %+v", status)
		}
		if len(status.Runs) > 0 && status.Runs[0].FinishedAt != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("backup did not finish in time: %+v", status)
		}
		time.Sleep(300 * time.Millisecond)
	}
	run := status.Runs[0]
	if !run.OK || run.Size == 0 || run.Target == "" || !strings.HasPrefix(run.Detail, "mailez-backup-") {
		t.Fatalf("finished run wrong: %+v", run)
	}

	// Verification re-reads the archive from the target and decrypts it:
	// control.db plus one uploaded file.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/backup/verify/"+itoa(int(run.ID)), nil)
	resp, err = app.Test(req, 20000)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("verify: status = %d err = %v", resp.StatusCode, err)
	}
	var verdict struct {
		OK      bool `json:"ok"`
		Entries int  `json:"entries"`
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err := json.Unmarshal(b, &verdict); err != nil || !verdict.OK || verdict.Entries != 2 {
		t.Fatalf("verify = %s (%v)", b, err)
	}

	// Verifying a run without an archive fails cleanly.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/admin/backup/verify/999999", nil)
	if resp, _ := app.Test(req); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("verify missing run: status = %d, want 422", resp.StatusCode)
	}
}

// Without a configured target the trigger is refused and the status
// reports the feature as off.
func TestOpsBackupUnconfigured(t *testing.T) {
	app, _ := newOpsApp(t, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/backup/run", nil)
	if resp, _ := app.Test(req); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unconfigured trigger: status = %d, want 400", resp.StatusCode)
	}
	code, body := getOps(t, app, "/api/v1/admin/backup")
	if code != http.StatusOK {
		t.Fatalf("status: %d %s", code, body)
	}
	var status opsBackupStatus
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		t.Fatalf("status json: %v", err)
	}
	if status.Configured || status.KeySet || len(status.Runs) != 0 {
		t.Fatalf("status = %+v, want all-off", status)
	}
}

// The health report carries the new probes with well-formed statuses and
// stays silent about disabled features.
func TestOpsHealthReportShape(t *testing.T) {
	app, _ := newOpsApp(t, nil)

	code, body := getOps(t, app, "/api/v1/admin/health")
	if code != http.StatusOK {
		t.Fatalf("health: %d %s", code, body)
	}
	var report struct {
		CheckedAt time.Time      `json:"checked_at"`
		Domains   []DomainReport `json:"domains"`
		System    []CheckItem    `json:"system"`
	}
	if err := json.Unmarshal([]byte(body), &report); err != nil {
		t.Fatalf("health json: %v (%s)", err, body)
	}

	valid := map[string]bool{statusOK: true, statusWarn: true, statusFail: true, statusUnknown: true}
	ids := map[string]CheckItem{}
	for _, item := range report.System {
		if !valid[item.Status] {
			t.Fatalf("system check %s has invalid status %q", item.ID, item.Status)
		}
		ids[item.ID] = item
	}
	for _, want := range []string{"database", "bans", "outbound25", "engine_imap", "engine_mta"} {
		if _, have := ids[want]; !have {
			t.Fatalf("system report missing %q: %s", want, body)
		}
	}
	if _, have := ids["backup"]; have {
		t.Fatal("backup probe must be absent while backups are unconfigured")
	}
	if bans := ids["bans"]; !strings.Contains(bans.Detail, "0") {
		t.Fatalf("bans detail = %q, want the empty-baseline text", bans.Detail)
	}
	if len(report.Domains) != 1 || report.Domains[0].Domain != "t.example" || len(report.Domains[0].Items) == 0 {
		t.Fatalf("domain reports = %+v", report.Domains)
	}
}
