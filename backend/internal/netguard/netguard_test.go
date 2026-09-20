package netguard

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateRejectsLocalTargets(t *testing.T) {
	for _, u := range []string{
		"",
		"ftp://example.com/hook",
		"http://localhost:8080/hook",
		"http://api.localhost/x",
		"http://metadata.google.internal/computeMetadata/v1/",
		"http://127.0.0.1:9000/",
		"https://10.0.0.5/vcard",
		"http://192.168.1.1/admin",
		"http://169.254.169.254/latest/meta-data",
		"http://100.64.1.2/",
		"http://[::1]/",
		"http://0.0.0.0/",
	} {
		if err := Validate(u); err == nil {
			t.Errorf("Validate(%q) = nil, want error", u)
		}
	}
	for _, u := range []string{
		"https://hooks.example.com/mailez",
		"http://mail.example.org:8080/inbox",
	} {
		if err := Validate(u); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", u, err)
		}
	}
}

// The dial-time guard refuses loopback even when Validate accepted the
// hostname: a public DNS name can point at 127.0.0.1.
func TestClientRefusesPrivateDialTargets(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	if _, err := NewClient(5 * time.Second).Get(srv.URL); !errors.Is(err, ErrBlocked) {
		t.Fatalf("Get(%s) error = %v, want ErrBlocked", srv.URL, err)
	}
	if hits != 0 {
		t.Fatalf("handler saw %d requests, want 0", hits)
	}
}

func TestClientNeverFollowsRedirects(t *testing.T) {
	c := NewClient(time.Second)
	err := c.CheckRedirect(nil, nil)
	if !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect = %v, want http.ErrUseLastResponse", err)
	}
}
