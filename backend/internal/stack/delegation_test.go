package stack

// Delegation contract tests: send-as grants on the directory sender lookup
// and full-access delegated logins on the auth/email endpoint.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

// seedDelegation grants bob send-as (and optionally full access) on alice's
// mailbox.
func seedDelegation(t *testing.T, h *Handler, canSend, fullAccess bool) {
	t.Helper()
	if err := h.DB.Create(&models.MailDelegation{
		OwnerEmail:    "alice@example.com",
		DelegateEmail: "bob@example.com",
		CanSend:       canSend,
		FullAccess:    fullAccess,
	}).Error; err != nil {
		t.Fatalf("seed delegation: %v", err)
	}
}

func TestDirectorySenderDelegation(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	// Without a grant, a non-spoofing user (alice) may not use another
	// user's address of the same domain (bob).
	if code, _ := doGet(t, app, "/stack/directory/senders/alice@example.com"); code != 200 {
		t.Fatalf("alice own sender: got %d", code)
	}
	req := httptest.NewRequest(http.MethodGet, "/stack/directory/senders/bob@example.com", nil)
	req.Header.Set("X-Auth-User", "alice@example.com")
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bob using alice without grant: got %d, want 404", resp.StatusCode)
	}

	// With a send-as grant bob may use alice's address.
	seedDelegation(t, h, true, false)
	req = httptest.NewRequest(http.MethodGet, "/stack/directory/senders/alice@example.com", nil)
	req.Header.Set("X-Auth-User", "bob@example.com")
	resp, err = app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	body := make([]byte, 512)
	n, _ := resp.Body.Read(body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body[:n]), `"allowed":true`) {
		t.Fatalf("bob using alice with grant: got %d %q", resp.StatusCode, string(body[:n]))
	}

	// The grant is owner-scoped: bob still cannot use carol's address.
	req = httptest.NewRequest(http.MethodGet, "/stack/directory/senders/carol@spoof.example.com", nil)
	req.Header.Set("X-Auth-User", "bob@example.com")
	resp, err = app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("bob using carol: got %d, want 404", resp.StatusCode)
	}

	// Allow-spoofing is bound to the user who owns the grant: carol may use
	// any address of her own domain, but alice may not use carol's address.
	req = httptest.NewRequest(http.MethodGet, "/stack/directory/senders/anyone@spoof.example.com", nil)
	req.Header.Set("X-Auth-User", "carol@spoof.example.com")
	resp, err = app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("carol spoofing own domain: got %d, want 200", resp.StatusCode)
	}
	req = httptest.NewRequest(http.MethodGet, "/stack/directory/senders/carol@spoof.example.com", nil)
	req.Header.Set("X-Auth-User", "alice@example.com")
	resp, err = app.Test(req, 5000)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("alice using carol spoof address: got %d, want 404", resp.StatusCode)
	}
}

func TestAuthEmailDelegatedLogin(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	bobSid, err := h.Auth.CreateSession(context.Background(), "bob@example.com")
	if err != nil {
		t.Fatal(err)
	}
	bobTok, err := h.Auth.CreateTempToken(context.Background(), "bob@example.com", bobSid)
	if err != nil {
		t.Fatal(err)
	}

	auth := func(user, pass, port string) (*http.Response, map[string]string) {
		req := httptest.NewRequest(http.MethodGet, "/stack/auth/email", nil)
		req.Header.Set("Auth-Method", "plain")
		req.Header.Set("Auth-Protocol", "imap")
		req.Header.Set("Auth-Port", port)
		req.Header.Set("Auth-User", user)
		req.Header.Set("Auth-Pass", pass)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		out := map[string]string{}
		for _, k := range []string{"Auth-Status", "Auth-User", "Auth-Port"} {
			out[k] = resp.Header.Get(k)
		}
		return resp, out
	}

	// Bob's own token logs in as bob (port 1143 = webmail token path).
	resp, out := auth("bob@example.com", bobTok, "1143")
	if resp.StatusCode != http.StatusOK || out["Auth-Status"] != "OK" {
		t.Fatalf("bob own token: %d %+v", resp.StatusCode, out)
	}

	// Without a full-access grant bob's token cannot open alice's mailbox.
	resp, out = auth("alice@example.com", bobTok, "1143")
	if out["Auth-Status"] == "OK" {
		t.Fatalf("bob token as alice without grant: %+v", out)
	}

	// With full access the same token logs into alice's mailbox.
	seedDelegation(t, h, true, true)
	resp, out = auth("alice@example.com", bobTok, "1143")
	if resp.StatusCode != http.StatusOK || out["Auth-Status"] != "OK" || out["Auth-User"] != "alice@example.com" {
		t.Fatalf("bob token as alice with full access: %d %+v", resp.StatusCode, out)
	}

	// Send-only grants do not open the mailbox.
	h.DB.Model(&models.MailDelegation{}).Where("owner_email = ? AND delegate_email = ?", "alice@example.com", "bob@example.com").
		Update("full_access", false)
	resp, out = auth("alice@example.com", bobTok, "1143")
	if out["Auth-Status"] == "OK" {
		t.Fatalf("bob token as alice with send-only grant: %+v", out)
	}
}

// TestAuthEmailDelegatedAppToken covers desktop clients that authenticate
// with an app token: the delegate's own app token opens the
// owner's mailbox only while a full-access grant exists.
func TestAuthEmailDelegatedAppToken(t *testing.T) {
	h, app := newContractHarness(t)
	seedContractData(t, h)

	const bobToken = "abcdef0123456789abcdef0123456789"
	hash, err := password.HashPBKDF2SHA256(bobToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.DB.Create(&models.Token{UserEmail: "bob@example.com", Password: hash}).Error; err != nil {
		t.Fatal(err)
	}
	auth := func(user, pass string) string {
		req := httptest.NewRequest(http.MethodGet, "/stack/auth/email", nil)
		req.Header.Set("Auth-Method", "plain")
		req.Header.Set("Auth-Protocol", "imap")
		req.Header.Set("Auth-Port", "993")
		req.Header.Set("Auth-User", user)
		req.Header.Set("Auth-Pass", pass)
		resp, err := app.Test(req, 5000)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.Header.Get("Auth-Status")
	}

	// Bob's app token logs into his own mailbox.
	if status := auth("bob@example.com", bobToken); status != "OK" {
		t.Fatalf("bob app token own mailbox: %q", status)
	}
	// Without a grant it cannot open alice's mailbox.
	if status := auth("alice@example.com", bobToken); status == "OK" {
		t.Fatal("bob app token opened alice mailbox without grant")
	}
	// With a full-access grant it can.
	seedDelegation(t, h, true, true)
	if status := auth("alice@example.com", bobToken); status != "OK" {
		t.Fatalf("bob app token as alice with grant: %q", status)
	}
	// A send-only grant is not enough.
	h.DB.Model(&models.MailDelegation{}).Where("owner_email = ? AND delegate_email = ?", "alice@example.com", "bob@example.com").
		Update("full_access", false)
	if status := auth("alice@example.com", bobToken); status == "OK" {
		t.Fatal("bob app token opened alice mailbox with send-only grant")
	}
}
