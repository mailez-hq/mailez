package mailbox

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMailReadAll(t *testing.T) {
	fake := &fakeGateway{}
	app, _ := newTestApp(t, fake)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mail/read-all", strings.NewReader(`{"folder":"Inbox"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if len(fake.marked) != 1 || fake.marked[0] != "Inbox" {
		t.Fatalf("mark all read called with %v", fake.marked)
	}
}
