package drive

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// A share link is meant to open for someone who has no account here, so the
// download route must sit outside RequireAuth and authorize by token alone.
// Before this the route lived in the authenticated group and every anonymous
// visitor got a 401 no matter how valid the token was.
func TestShareDownloadWithoutSession(t *testing.T) {
	app, svc := newDriveApp(t)

	body, ct := driveBody(t, "note.txt", "hello share", "")
	upReq := httptest.NewRequest(http.MethodPost, "/api/v1/drive/upload", body)
	upReq.Header.Set("Content-Type", ct)
	upResp, err := app.Test(upReq)
	if err != nil {
		t.Fatal(err)
	}
	upB, _ := io.ReadAll(upResp.Body)
	upResp.Body.Close()
	if upResp.StatusCode != http.StatusOK {
		t.Fatalf("upload: %d %s", upResp.StatusCode, upB)
	}

	shareReq := httptest.NewRequest(http.MethodGet, "/api/v1/drive/share/1", nil)
	shareResp, err := app.Test(shareReq)
	if err != nil {
		t.Fatal(err)
	}
	shareB, _ := io.ReadAll(shareResp.Body)
	shareResp.Body.Close()
	var share struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(shareB, &share); err != nil || share.URL == "" {
		t.Fatalf("share: %d %s", shareResp.StatusCode, shareB)
	}
	token := share.URL[strings.Index(share.URL, "token=")+len("token="):]

	// Public first, then the authenticated group — the order server.go uses.
	pub := fiber.New()
	svc.RegisterPublic(pub.Group("/api/v1"))
	svc.Register(pub.Group("/api/v1", func(c *fiber.Ctx) error {
		c.Locals("user", &models.User{Email: "alice@example.com", DomainName: "example.com", Enabled: true})
		return c.Next()
	}))

	anon := httptest.NewRequest(http.MethodGet, "/api/v1/drive/download/1?token="+token, nil)
	anonResp, err := pub.Test(anon)
	if err != nil {
		t.Fatal(err)
	}
	anonBody, _ := io.ReadAll(anonResp.Body)
	anonResp.Body.Close()
	if anonResp.StatusCode != http.StatusOK || string(anonBody) != "hello share" {
		t.Fatalf("anonymous download: %d %q, want 200 %q", anonResp.StatusCode, anonBody, "hello share")
	}

	// The token is the only key: without it an anonymous caller gets nothing.
	denied := httptest.NewRequest(http.MethodGet, "/api/v1/drive/download/1", nil)
	deniedResp, err := pub.Test(denied)
	if err != nil {
		t.Fatal(err)
	}
	deniedResp.Body.Close()
	if deniedResp.StatusCode != http.StatusForbidden {
		t.Fatalf("download without token: %d, want 403", deniedResp.StatusCode)
	}
}
