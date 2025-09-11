package mailbox

import (
	"strconv"
	"strings"
	"time"

	"mailez/backend/internal/core"
	"mailez/backend/internal/mail"

	"github.com/gofiber/fiber/v2"
)

// Burn-after-read (阅后即焚) staging keywords. The engine stores flags as IMAP
// keywords, so the reveal window travels with the message like the snooze
// keywords do ($SnoozedUntil-<unix>).
const (
	burnReadFlag     = "$BurnRead"
	burnUntilPrefix  = "$BurnReadUntil-"
	burnLockedCode   = "burn_locked"
	burnConsumedCode = "burn_consumed"
)

// burnState reads the burn keywords off a message's flag list.
func burnState(flags []string) (revealed bool, until int64) {
	for _, f := range flags {
		if strings.EqualFold(f, burnReadFlag) {
			revealed = true
			continue
		}
		if len(f) > len(burnUntilPrefix) && strings.EqualFold(f[:len(burnUntilPrefix)], burnUntilPrefix) {
			if n, err := strconv.ParseInt(f[len(burnUntilPrefix):], 10, 64); err == nil {
				until = n
			}
			revealed = true
		}
	}
	return revealed, until
}

// stripBurnBody removes everything that carries message content. Used for a
// burn message that has not been revealed yet, or whose window has closed.
func stripBurnBody(msg *mail.Message) {
	if msg == nil {
		return
	}
	msg.TextBody = ""
	msg.HTMLBody = ""
	msg.Attachments = nil
	msg.Preview = ""
	msg.Invitation = nil
}

// gateBurn applies the burn-after-read contract to a detail response: the
// body only exists on the wire while the message is inside its reveal window.
func gateBurn(msg *mail.Message) {
	if msg == nil || msg.BurnAfterMinutes <= 0 {
		return
	}
	revealed, until := burnState(msg.Flags)
	switch {
	case !revealed:
		msg.BurnLocked = true
		stripBurnBody(msg)
	case until > 0 && time.Now().Unix() >= until:
		msg.BurnConsumed = true
		stripBurnBody(msg)
	}
}

// mailBurnReveal is the explicit "open this burn-after-read message" action:
// the server starts the reveal window and only then hands over the body, so a
// client cannot read it by simply fetching the message.
func (h *Handler) mailBurnReveal(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return core.DialFailure(c, err)
	}
	var in struct {
		Folder string `json:"folder"`
		UID    uint32 `json:"uid"`
	}
	if err := c.BodyParser(&in); err != nil || in.Folder == "" || in.UID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "folder and uid are required"})
	}
	msg, err := h.Mail.With(d).GetMessage(d.Email, d.Token, in.Folder, in.UID)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	if msg.BurnAfterMinutes <= 0 {
		return c.Status(400).JSON(fiber.Map{"error": "not a burn-after-read message"})
	}
	revealed, until := burnState(msg.Flags)
	if revealed && until > 0 && time.Now().Unix() >= until {
		return c.Status(410).JSON(fiber.Map{"error": "this message has already been burned", "code": burnConsumedCode})
	}
	if !revealed {
		until = time.Now().Add(time.Duration(msg.BurnAfterMinutes) * time.Minute).Unix()
		if err := h.Mail.With(d).SetFlag(d.Email, d.Token, in.Folder, in.UID, burnReadFlag, true); err != nil {
			return core.Fail(c, 502, err, "mail service error")
		}
		if err := h.Mail.With(d).SetFlag(d.Email, d.Token, in.Folder, in.UID,
			burnUntilPrefix+strconv.FormatInt(until, 10), true); err != nil {
			return core.Fail(c, 502, err, "mail service error")
		}
	}
	// Re-read so the response carries the body plus the updated flags.
	fresh, err := h.Mail.With(d).GetMessage(d.Email, d.Token, in.Folder, in.UID)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	fresh.Flags = decodeLabelFlags(fresh.Flags, h.labelNamesByKeyword(mailboxIdentity(c)))
	// Inside the window the body is served; the gate is a no-op here.
	return c.JSON(fresh)
}
