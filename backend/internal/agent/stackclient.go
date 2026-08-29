package agent

import (
	"net/http"
	"time"
)

// SecretHeader authenticates requests to the control plane's internal
// /stack API; its value must match the backend's MAILEZ_STACK_SECRET.
const SecretHeader = "X-Stack-Secret"

type secretTransport struct {
	base   http.RoundTripper
	secret string
}

func (t secretTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.secret != "" {
		req = req.Clone(req.Context())
		req.Header.Set(SecretHeader, t.secret)
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}

// StackHTTPClient returns a client that presents the shared stack secret on
// every request (no-op when secret is empty, keeping unauthenticated local
// dev working).
func StackHTTPClient(secret string, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: secretTransport{secret: secret},
	}
}

// StackSecret reads the shared internal API secret from the environment.
func StackSecret() string { return Getenv("MAILEZ_STACK_SECRET", "") }
