package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

// openAIServer is a minimal OpenAI-compatible /chat/completions stub used to
// test the connection-test endpoint without external network access.
func openAIServer(t *testing.T, fail bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
}

func doJSON(t *testing.T, app *fiber.App, method, path string, body string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req, 30000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func aiViewFrom(t *testing.T, raw string) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode view %q: %v", raw, err)
	}
	return v
}

func TestAICreateListDelete(t *testing.T) {
	app := newTestApp(t)
	code, raw := doJSON(t, app, "POST", "/api/v1/config/ai",
		`{"name":"DeepSeek 生产","base_url":"https://api.deepseek.com","api_key":"sk-test","model":"deepseek-chat"}`)
	if code != 201 {
		t.Fatalf("create status %d: %s", code, raw)
	}
	v := aiViewFrom(t, raw)
	if v["enabled"] != false || v["has_api_key"] != true || v["name"] != "DeepSeek 生产" {
		t.Fatalf("create view: %v", v)
	}
	id := v["id"].(float64)

	code, raw = doJSON(t, app, "GET", "/api/v1/config/ai", "")
	if code != 200 || !strings.Contains(raw, "DeepSeek 生产") {
		t.Fatalf("list status %d: %s", code, raw)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(raw), &list); err != nil || len(list) != 1 {
		t.Fatalf("list decode: %v %s", err, raw)
	}

	code, raw = doJSON(t, app, "DELETE", fmt.Sprintf("/api/v1/config/ai/%d", uint(id)), "")
	if code != 204 {
		t.Fatalf("delete status %d: %s", code, raw)
	}
	code, raw = doJSON(t, app, "GET", "/api/v1/config/ai", "")
	if code != 200 || !strings.Contains(raw, "[]") {
		t.Fatalf("list after delete: %d %s", code, raw)
	}
}

func TestAIEnableRequiresPassingTest(t *testing.T) {
	app := newTestApp(t)
	code, raw := doJSON(t, app, "POST", "/api/v1/config/ai",
		`{"name":"p1","base_url":"https://api.deepseek.com","api_key":"sk-test","model":"deepseek-chat"}`)
	if code != 201 {
		t.Fatalf("create status %d: %s", code, raw)
	}
	id := uint(aiViewFrom(t, raw)["id"].(float64))

	// Enabling before any test must be rejected.
	code, raw = doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/config/ai/%d", id), `{"enabled":true}`)
	if code != 400 {
		t.Fatalf("enable without test status %d, want 400: %s", code, raw)
	}
	// Test without a stored key must be rejected too.
	code, raw = doJSON(t, app, "POST", "/api/v1/config/ai", `{"name":"nokey","base_url":"https://x"}`)
	if code != 201 {
		t.Fatalf("create nokey: %d %s", code, raw)
	}
	nokeyID := uint(aiViewFrom(t, raw)["id"].(float64))
	code, raw = doJSON(t, app, "POST", fmt.Sprintf("/api/v1/config/ai/%d/test", nokeyID), "")
	if code != 400 {
		t.Fatalf("test without key status %d, want 400: %s", code, raw)
	}
}

func TestAITestEnableDefaultLifecycle(t *testing.T) {
	app := newTestApp(t)
	srv := openAIServer(t, false)
	defer srv.Close()

	code, raw := doJSON(t, app, "POST", "/api/v1/config/ai",
		fmt.Sprintf(`{"name":"a","base_url":%q,"api_key":"sk-a","model":"m"}`, srv.URL))
	if code != 201 {
		t.Fatalf("create a: %d %s", code, raw)
	}
	idA := uint(aiViewFrom(t, raw)["id"].(float64))
	code, raw = doJSON(t, app, "POST", "/api/v1/config/ai",
		fmt.Sprintf(`{"name":"b","base_url":%q,"api_key":"sk-b","model":"m"}`, srv.URL))
	if code != 201 {
		t.Fatalf("create b: %d %s", code, raw)
	}
	idB := uint(aiViewFrom(t, raw)["id"].(float64))

	// Test both providers against the stub.
	for _, id := range []uint{idA, idB} {
		code, raw = doJSON(t, app, "POST", fmt.Sprintf("/api/v1/config/ai/%d/test", id), "")
		if code != 200 {
			t.Fatalf("test %d status %d: %s", id, code, raw)
		}
		v := aiViewFrom(t, raw)
		if v["last_test_ok"] != true || v["last_test_at"] == nil {
			t.Fatalf("test view %d: %v", id, v)
		}
	}

	// Enable both, then make A default; B's default flag must clear.
	for _, id := range []uint{idA, idB} {
		code, raw = doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/config/ai/%d", id), `{"enabled":true}`)
		if code != 200 {
			t.Fatalf("enable %d status %d: %s", id, code, raw)
		}
	}
	code, raw = doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/config/ai/%d", idA), `{"is_default":true}`)
	if code != 200 || aiViewFrom(t, raw)["is_default"] != true {
		t.Fatalf("set default A: %d %s", code, raw)
	}
	code, raw = doJSON(t, app, "GET", "/api/v1/config/ai", "")
	if code != 200 {
		t.Fatalf("list: %d %s", code, raw)
	}
	var list []map[string]any
	_ = json.Unmarshal([]byte(raw), &list)
	defaults := 0
	for _, p := range list {
		if p["is_default"] == true {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("expected exactly one default: %s", raw)
	}

	// Setting default on a disabled provider must be rejected.
	code, raw = doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/config/ai/%d", idB), `{"enabled":false}`)
	if code != 200 {
		t.Fatalf("disable B: %d %s", code, raw)
	}
	code, raw = doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/config/ai/%d", idB), `{"is_default":true}`)
	if code != 400 {
		t.Fatalf("default on disabled status %d, want 400: %s", code, raw)
	}
}

func TestAITestFailureDisablesAndChangeResets(t *testing.T) {
	app := newTestApp(t)
	ok := openAIServer(t, false)
	bad := openAIServer(t, true)
	defer ok.Close()
	defer bad.Close()

	code, raw := doJSON(t, app, "POST", "/api/v1/config/ai",
		fmt.Sprintf(`{"name":"p","base_url":%q,"api_key":"sk","model":"m"}`, ok.URL))
	if code != 201 {
		t.Fatalf("create: %d %s", code, raw)
	}
	id := uint(aiViewFrom(t, raw)["id"].(float64))

	// Test OK -> enable -> point at a failing endpoint -> re-test.
	code, raw = doJSON(t, app, "POST", fmt.Sprintf("/api/v1/config/ai/%d/test", id), "")
	if code != 200 || aiViewFrom(t, raw)["last_test_ok"] != true {
		t.Fatalf("test ok: %d %s", code, raw)
	}
	code, raw = doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/config/ai/%d", id), `{"enabled":true,"is_default":true}`)
	if code != 200 {
		t.Fatalf("enable: %d %s", code, raw)
	}

	// Changing the endpoint clears the test result and disables the row.
	code, raw = doJSON(t, app, "PUT", fmt.Sprintf("/api/v1/config/ai/%d", id),
		fmt.Sprintf(`{"base_url":%q}`, bad.URL))
	if code != 200 {
		t.Fatalf("change endpoint: %d %s", code, raw)
	}
	v := aiViewFrom(t, raw)
	if v["enabled"] != false || v["is_default"] != false || v["last_test_ok"] != false {
		t.Fatalf("after change view: %v", v)
	}

	// A failing test records the error and keeps the provider disabled.
	code, raw = doJSON(t, app, "POST", fmt.Sprintf("/api/v1/config/ai/%d/test", id), "")
	if code != 200 {
		t.Fatalf("test fail status: %d %s", code, raw)
	}
	v = aiViewFrom(t, raw)
	if v["last_test_ok"] != false || v["enabled"] != false || v["last_test_error"] == "" {
		t.Fatalf("failed test view: %v", v)
	}
}

func TestAINameConflict(t *testing.T) {
	app := newTestApp(t)
	code, raw := doJSON(t, app, "POST", "/api/v1/config/ai", `{"name":"Same","base_url":"https://x","api_key":"k","model":"m"}`)
	if code != 201 {
		t.Fatalf("create: %d %s", code, raw)
	}
	code, raw = doJSON(t, app, "POST", "/api/v1/config/ai", `{"name":"same","base_url":"https://y","api_key":"k2","model":"m"}`)
	if code != 409 {
		t.Fatalf("duplicate name status %d, want 409: %s", code, raw)
	}
}
