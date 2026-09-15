// Integration test for the ban gate on the mail-proxy auth surface: the
// same failure counter feeds the web and SMTP/IMAP paths, a banned source
// IP gets the protocol throttling answer even with correct credentials,
// and a successful login clears the counter.
package stack

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/ban"
	"mailez/backend/internal/core/models"
)

const bannedTestIP = "203.0.113.7"

// seedMailUser creates the domain and one enabled mailbox so the auth
// surface has someone to authenticate.
func seedMailUser(t *testing.T, h *Handler) {
	t.Helper()
	for _, v := range []any{
		&models.Domain{Name: "example.com"},
		&models.User{
			Email: "alice@example.com", Localpart: "alice", DomainName: "example.com",
			Password: testUserHash(), Enabled: true, EnableImap: true,
		},
	} {
		if err := h.DB.Create(v).Error; err != nil {
			t.Fatalf("seed %T: %v", v, err)
		}
	}
}

func mailAuth(t *testing.T, app *fiber.App, user, pass string) (string, http.Header) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/stack/auth/email", nil)
	req.Header.Set("Auth-Method", "plain")
	req.Header.Set("Auth-Protocol", "smtp")
	req.Header.Set("Auth-User", user)
	req.Header.Set("Auth-Pass", pass)
	req.Header.Set("Client-Ip", bannedTestIP)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("auth/email: %v", err)
	}
	defer resp.Body.Close()
	return resp.Status, resp.Header
}

func TestAuthEmailBanGate(t *testing.T) {
	h, app := newContractHarness(t)
	seedMailUser(t, h)

	// Attach a real ban engine with a low threshold so two failures trip it.
	cfg := h.Cfg
	cfg.BanMaxRetry = 2
	cfg.BanFindTimeSec = 600
	h.Auth.Bans = ban.New(h.DB, h.Auth.Store, cfg)
	h.Auth.SetLoginLimits(100, 100)

	// One failure: normal rejection, nothing banned yet.
	status, headers := mailAuth(t, app, "alice@example.com", "wrong")
	if status[:3] != "200" || headers.Get("Auth-Status") != "Authentication credentials invalid" || headers.Get("Auth-Wait") != "0" {
		t.Fatalf("first failure: %s %q wait=%q", status, headers.Get("Auth-Status"), headers.Get("Auth-Wait"))
	}

	// Second failure reaches the threshold and bans the source IP.
	status, headers = mailAuth(t, app, "alice@example.com", "wrong")
	if status[:3] != "200" || headers.Get("Auth-Status") != "Authentication credentials invalid" {
		t.Fatalf("second failure: %s %q", status, headers.Get("Auth-Status"))
	}
	var rec models.BanRecord
	if err := h.DB.Where("ip = ?", bannedTestIP).First(&rec).Error; err != nil {
		t.Fatalf("no ban record after two failures: %v", err)
	}
	if !rec.Until.After(time.Now()) || rec.LiftedAt != nil {
		t.Fatalf("ban record not active: %+v", rec)
	}

	// While banned, even correct credentials get the throttle answer.
	status, headers = mailAuth(t, app, "alice@example.com", "secret123")
	if headers.Get("Auth-Status") != "Too many authentication failures" {
		t.Fatalf("banned correct login: %s %q", status, headers.Get("Auth-Status"))
	}
	if headers.Get("Auth-Wait") == "0" || headers.Get("Auth-Error-Code") == "" {
		t.Fatalf("banned answer missing wait/code: %+v", map[string]string{
			"wait": headers.Get("Auth-Wait"), "code": headers.Get("Auth-Error-Code"),
		})
	}

	// Lift the ban (the admin endpoint writes the same columns), then a
	// correct login succeeds and clears the failure counter.
	now := time.Now()
	if err := h.DB.Model(&rec).Updates(map[string]any{"lifted_at": now, "until": now}).Error; err != nil {
		t.Fatalf("lift: %v", err)
	}
	status, headers = mailAuth(t, app, "alice@example.com", "secret123")
	if headers.Get("Auth-Status") != "OK" {
		t.Fatalf("post-lift correct login: %s %q", status, headers.Get("Auth-Status"))
	}

	// Counter was reset by the success: one failure is below the
	// threshold of two, so the client is not banned again.
	status, headers = mailAuth(t, app, "alice@example.com", "wrong")
	if headers.Get("Auth-Status") != "Authentication credentials invalid" || headers.Get("Auth-Wait") != "0" {
		t.Fatalf("post-reset failure: %s %q wait=%q", status, headers.Get("Auth-Status"), headers.Get("Auth-Wait"))
	}
	var active int64
	h.DB.Model(&models.BanRecord{}).
		Where("ip = ? AND lifted_at IS NULL AND until > ?", bannedTestIP, time.Now()).
		Count(&active)
	if active != 0 {
		t.Fatal("single post-reset failure banned again")
	}
}

// A nil Bans gate must not change the auth contract (community builds and
// deployments that never arm the engine).
func TestAuthEmailWithoutBanEngine(t *testing.T) {
	h, app := newContractHarness(t)
	seedMailUser(t, h)
	if h.Auth.Bans != nil {
		t.Fatal("harness unexpectedly armed")
	}
	h.Auth.SetLoginLimits(100, 100)
	_, headers := mailAuth(t, app, "alice@example.com", "wrong")
	if headers.Get("Auth-Status") != "Authentication credentials invalid" {
		t.Fatalf("wrong pw without engine: %q", headers.Get("Auth-Status"))
	}
	_, headers = mailAuth(t, app, "alice@example.com", "secret123")
	if headers.Get("Auth-Status") != "OK" {
		t.Fatalf("correct pw without engine: %q", headers.Get("Auth-Status"))
	}
}
