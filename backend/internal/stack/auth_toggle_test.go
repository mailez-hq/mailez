package stack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

func authStatus(t *testing.T, app *fiber.App, protocol, user, pass, port string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/stack/auth/email", nil)
	req.Header.Set("Auth-Method", "plain")
	req.Header.Set("Auth-Protocol", protocol)
	req.Header.Set("Auth-Port", port)
	req.Header.Set("Auth-User", user)
	req.Header.Set("Auth-Pass", pass)
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s/%s on port %s: status %d", protocol, user, port, resp.StatusCode)
	}
	return resp.Header.Get("Auth-Status")
}

func setToggles(t *testing.T, h *Handler, email string, imap, pop bool) {
	t.Helper()
	if err := h.DB.Model(&models.User{}).Where("email = ?", email).
		Updates(map[string]any{"enable_imap": imap, "enable_pop": pop}).Error; err != nil {
		t.Fatalf("set toggles for %s: %v", email, err)
	}
}

// TestAuthEmailProtocolToggles pins which credential may use which protocol
// under which toggles. Every case runs on more than one port: the port must
// never influence the answer.
func TestAuthEmailProtocolToggles(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	const user = "bob@example.com"
	const passwordCred = "secret123"
	const appToken = "abcdef0123456789abcdef0123456789"

	hash, err := password.HashPBKDF2SHA256(appToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.DB.Create(&models.Token{UserEmail: user, Password: hash}).Error; err != nil {
		t.Fatalf("seed app token: %v", err)
	}
	sid, err := h.Auth.CreateSession(context.Background(), user)
	if err != nil {
		t.Fatal(err)
	}
	sessionCred, err := h.Auth.CreateTempToken(context.Background(), user, sid)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		protocol string
		cred     string
		imap     bool
		pop      bool
		disabled bool
		wantOK   bool
	}{
		{"password imap on", "imap", passwordCred, true, true, false, true},
		{"password imap off", "imap", passwordCred, false, true, false, false},
		{"password pop3 on", "pop3", passwordCred, true, true, false, true},
		{"password pop3 off", "pop3", passwordCred, true, false, false, false},
		{"password submission", "smtp", passwordCred, false, false, false, true},
		{"app token imap on", "imap", appToken, true, true, false, true},
		{"app token imap off", "imap", appToken, false, true, false, false},
		{"app token pop3 off", "pop3", appToken, true, false, false, false},
		{"webmail session imap on", "imap", sessionCred, true, true, false, true},
		{"webmail session imap off", "imap", sessionCred, false, true, false, true},
		{"webmail session pop3 off", "pop3", sessionCred, true, false, false, false},
		{"disabled account", "smtp", passwordCred, true, true, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setToggles(t, h, user, tc.imap, tc.pop)
			if err := h.DB.Model(&models.User{}).Where("email = ?", user).
				Update("enabled", !tc.disabled).Error; err != nil {
				t.Fatal(err)
			}
			for _, port := range []string{"143", "993"} {
				got := authStatus(t, app, tc.protocol, user, tc.cred, port) == "OK"
				if got != tc.wantOK {
					t.Fatalf("port %s: got OK=%v, want %v", port, got, tc.wantOK)
				}
			}
		})
	}

	// The engine's listen ports and the internal admin ports must all agree.
	setToggles(t, h, user, false, true)
	for _, port := range []string{"110", "1143", "1587", "4190", "11490"} {
		if got := authStatus(t, app, "imap", user, passwordCred, port); got == "OK" {
			t.Fatalf("imap password on port %s bypassed enable_imap=false", port)
		}
	}
}
