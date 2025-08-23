package mailbox

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
)

// unsubscribeClient is the shared HTTP client for one-click unsubscribes.
var unsubscribeClient = &http.Client{Timeout: 10 * time.Second}

// mailUnsubscribe triggers a sender's List-Unsubscribe URL server-side, so
// the user's browser never has to fight CORS or leak its IP to the sender.
// RFC 8058 one-click entries are POSTed; plain https entries are GET.
// @Summary One-click unsubscribe
// @Tags mail
// @Accept json
// @Success 200 {object} map[string]interface{} "status"
// @Failure 400 {object} map[string]interface{}
// @Router /mail/unsubscribe [post]
func (h *Handler) mailUnsubscribe(c *fiber.Ctx) error {
	var in struct {
		URL  string `json:"url"`
		Post bool   `json:"post"`
	}
	if err := c.BodyParser(&in); err != nil || in.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "url is required"})
	}
	if err := checkUnsubscribeURL(in.URL); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	var resp *http.Response
	var err error
	if in.Post {
		resp, err = unsubscribeClient.Post(in.URL, "application/x-www-form-urlencoded",
			strings.NewReader("List-Unsubscribe=One-Click"))
	} else {
		resp, err = unsubscribeClient.Get(in.URL)
	}
	if err != nil {
		return core.Fail(c, 502, err, "unsubscribe request failed")
	}
	defer resp.Body.Close()
	// The sender has acted on the request once any 2xx comes back; the body
	// itself (usually a confirmation page) is irrelevant.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.Status(502).JSON(fiber.Map{"error": fmt.Sprintf("unsubscribe endpoint returned %d", resp.StatusCode)})
	}
	return c.JSON(fiber.Map{"status": "ok"})
}

// checkUnsubscribeURL keeps the proxy from being aimed at internal services:
// https only, and no loopback/private/link-local hosts.
func checkUnsubscribeURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("only https unsubscribe links are supported")
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("local addresses are not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("private addresses are not allowed")
		}
	}
	return nil
}
