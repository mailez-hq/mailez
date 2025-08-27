package archive

import (
	"encoding/json"
	"io"
	"net/mail"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// ingestEvent is the JSON metadata part of the multipart ingest request.
type ingestEvent struct {
	Direction    string    `json:"direction"`
	EnvelopeFrom string    `json:"envelope_from"`
	EnvelopeTo   []string  `json:"envelope_to"`
	ReceivedAt   time.Time `json:"received_at"`
}

// ingest stores one captured message when the effective policy captures its
// direction. It always returns 2xx so the engine's spool can drop the copy:
// a policy that does not capture is not an error.
func (s *Service) ingest(c *fiber.Ctx) error {
	var ev ingestEvent
	if err := json.Unmarshal([]byte(c.FormValue("meta")), &ev); err != nil {
		return core.Fail(c, 400, err, "bad meta json")
	}

	rawFile, err := c.FormFile("raw")
	if err != nil {
		return core.Fail(c, 400, err, "missing raw part")
	}
	rf, err := rawFile.Open()
	if err != nil {
		return core.Fail(c, 400, err, "bad raw part")
	}
	raw, err := io.ReadAll(io.LimitReader(rf, 64<<20))
	rf.Close()
	if err != nil {
		return core.Fail(c, 500, err, "read raw")
	}
	if len(raw) == 0 {
		return core.Fail(c, 400, err, "empty message")
	}

	dir := models.ArchiveDirection(ev.Direction)
	if dir != models.ArchiveInbound && dir != models.ArchiveOutbound {
		return core.Fail(c, 400, nil, "invalid direction")
	}
	domain := domainOfMessage(dir, ev.EnvelopeFrom, ev.EnvelopeTo)
	if domain == "" {
		// No local domain can be derived; still store under a blank domain
		// when the global policy is enabled.
		domain = ""
	}
	pol, err := s.effectivePolicy(s.DB.WithContext(c.Context()), domain)
	if err != nil {
		return core.Fail(c, 500, err, "policy lookup failed")
	}
	if pol == nil || !pol.Captures(dir) {
		return c.SendStatus(fiber.StatusNoContent)
	}

	ph := parseMessage(raw)
	now := time.Now()
	row := models.ArchivedMessage{
		Direction:    dir,
		Domain:       domain,
		EnvelopeFrom: ev.EnvelopeFrom,
		EnvelopeTo:   strings.Join(ev.EnvelopeTo, ", "),
		MessageID:    ph.MessageID,
		From:         ph.From,
		To:           ph.To,
		Cc:           ph.Cc,
		Subject:      ph.Subject,
		Size:         int64(len(raw)),
		Raw:          raw,
		ArchivedAt:   now,
	}
	if !ph.Date.IsZero() {
		row.Date = ph.Date
	} else if !ev.ReceivedAt.IsZero() {
		row.Date = ev.ReceivedAt
	} else {
		row.Date = now
	}
	row.ExpiresAt = pol.EffectiveRetention(now)
	if err := s.DB.WithContext(c.Context()).Create(&row).Error; err != nil {
		return core.Fail(c, 500, err, "archive store failed")
	}
	return c.JSON(fiber.Map{"id": row.ID})
}

// domainOfMessage picks the domain the archive policy is scoped to: the
// sender's domain for outbound, the first recipient's domain for inbound.
func domainOfMessage(dir models.ArchiveDirection, from string, to []string) string {
	addr := func(v string) string {
		a, err := mail.ParseAddress(v)
		if err != nil {
			return ""
		}
		_, d, ok := strings.Cut(a.Address, "@")
		if !ok {
			return ""
		}
		return strings.ToLower(strings.TrimSpace(d))
	}
	if dir == models.ArchiveOutbound {
		return addr(from)
	}
	for _, rcpt := range to {
		if d := addr(rcpt); d != "" {
			return d
		}
	}
	return ""
}
