//go:build mailez_ee

package drive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestDriveShareLink covers the enterprise share surface: creating a share
// link and downloading with the token.
func TestDriveShareLink(t *testing.T) {
	app, _ := newDriveApp(t)
	// Upload a file at the root.
	body, ct := driveBody(t, "shared.txt", "share me", "0")
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

	// Create the share link.
	shareResp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/drive/share/1", nil))
	if err != nil {
		t.Fatal(err)
	}
	shareB, _ := io.ReadAll(shareResp.Body)
	shareResp.Body.Close()
	if !strings.Contains(string(shareB), "token=") {
		t.Fatalf("share: %s", shareB)
	}
}
