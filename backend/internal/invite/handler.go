package invite

import (
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
)

// Service owns the invitation endpoints.
type Service struct {
	*core.App
}

// New builds the invitation service.
func New(app *core.App) *Service {
	return &Service{app}
}

// Register mounts the invitation routes.
func (s *Service) Register(r fiber.Router) {
	r.Post("/invites/respond", s.respond)
	r.Post("/invites/send", s.sendInvite)
}

// respond answers a received meeting invitation: builds an iTIP REPLY, sends
// it to the organizer, and syncs the event into the user's calendar.
// @Summary Respond to a meeting invitation
// @Tags invite
// @Accept json
// @Produce json
// @Router /invites/respond [post]
func (s *Service) respond(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	var body struct {
		ICS    string `json:"ics"`
		Action string `json:"action"` // accept | decline | tentative
	}
	if err := c.BodyParser(&body); err != nil {
		return core.Fail(c, 400, err, "bad request")
	}
	kind := RespondKind(strings.ToLower(strings.TrimSpace(body.Action)))
	if kind != Accept && kind != Decline && kind != Tentative {
		return core.Fail(c, 400, nil, "action must be accept, decline or tentative")
	}
	inv := mail.ParseInvitation([]byte(body.ICS))
	if inv == nil || inv.Organizer == "" {
		return core.Fail(c, 400, nil, "not a valid meeting invitation")
	}
	replyICS, err := BuildReplyICS(inv, user.Email, kind)
	if err != nil {
		return core.Fail(c, 500, err, "build reply failed")
	}
	token, err := s.MailToken(c)
	if err != nil {
		return core.Fail(c, 500, err, "mail token failed")
	}
	subject := inv.Summary
	if subject == "" {
		subject = "会议邀请"
	}
	prefix := map[RespondKind]string{Accept: "接受：", Decline: "拒绝：", Tentative: "暂定："}[kind]
	text := "您已" + strings.TrimSuffix(prefix, "：") + "会议「" + subject + "」。\n"
	if err := s.Mail.Send(user.Email, token, user.Email, []string{inv.Organizer}, nil, nil,
		prefix+subject, text, "", []mail.Attachment{asAttachment(replyICS)}); err != nil {
		return core.Fail(c, 502, err, "send reply failed")
	}
	if kind == Decline {
		// Declined meetings are removed from the calendar.
		s.DB.Where("user_email = ? AND uid = ?", user.Email, inv.UID).Delete(&models.CalendarEvent{})
	} else {
		if err := s.upsertCalendarEvent(user.Email, inv); err != nil {
			// Calendar sync failure must not fail the RSVP itself.
			logInvite("calendar upsert", user.Email, inv.UID, err)
		}
	}
	return c.JSON(fiber.Map{"ok": true})
}

// sendInvite mails a new meeting REQUEST to the attendees.
// @Summary Send a meeting invitation
// @Tags invite
// @Accept json
// @Produce json
// @Router /invites/send [post]
func (s *Service) sendInvite(c *fiber.Ctx) error {
	user := core.CurrentUser(c)
	var body struct {
		To          []string `json:"to"`
		Summary     string   `json:"summary"`
		Location    string   `json:"location"`
		Description string   `json:"description"`
		Start       string   `json:"start"` // RFC3339
		End         string   `json:"end"`
	}
	if err := c.BodyParser(&body); err != nil {
		return core.Fail(c, 400, err, "bad request")
	}
	attendees := cleanAddresses(body.To)
	if len(attendees) == 0 {
		return core.Fail(c, 400, nil, "at least one attendee required")
	}
	summary := strings.TrimSpace(body.Summary)
	if summary == "" {
		return core.Fail(c, 400, nil, "summary required")
	}
	start, err := time.Parse(time.RFC3339, body.Start)
	if err != nil {
		return core.Fail(c, 400, err, "bad start time")
	}
	end := start.Add(time.Hour)
	if body.End != "" {
		if t, err := time.Parse(time.RFC3339, body.End); err == nil {
			end = t
		}
	}
	icsText, err := BuildRequestICS(newUID(summary), summary, strings.TrimSpace(body.Location),
		strings.TrimSpace(body.Description), start, end, user.Email, attendees)
	if err != nil {
		return core.Fail(c, 500, err, "build invitation failed")
	}
	token, err := s.MailToken(c)
	if err != nil {
		return core.Fail(c, 500, err, "mail token failed")
	}
	text := "邀请您参加会议：「" + summary + "」\n时间：" + start.Local().Format("2006-01-02 15:04") +
		" - " + end.Local().Format("2006-01-02 15:04") + "\n"
	if body.Location != "" {
		text += "地点：" + body.Location + "\n"
	}
	if body.Description != "" {
		text += "\n" + body.Description + "\n"
	}
	if err := s.Mail.Send(user.Email, token, user.Email, attendees, nil, nil,
		"邀请：「"+summary+"」", text, "", []mail.Attachment{asAttachment(icsText)}); err != nil {
		return core.Fail(c, 502, err, "send invitation failed")
	}
	return c.JSON(fiber.Map{"ok": true})
}

// upsertCalendarEvent stores (or updates) an accepted meeting in the user's
// calendar so the invitation also shows up in the calendar drawer.
func (s *Service) upsertCalendarEvent(userEmail string, inv *mail.Invitation) error {
	var ev models.CalendarEvent
	err := s.DB.Where("user_email = ? AND uid = ?", userEmail, inv.UID).First(&ev).Error
	ev.UserEmail = userEmail
	ev.UID = inv.UID
	ev.Summary = inv.Summary
	ev.Location = inv.Location
	ev.Description = inv.Description
	ev.AllDay = inv.AllDay
	if inv.Start != "" {
		if t, err := time.Parse(time.RFC3339, inv.Start); err == nil {
			ev.Start = &t
		}
	}
	if inv.End != "" {
		if t, err := time.Parse(time.RFC3339, inv.End); err == nil {
			ev.End = &t
		}
	}
	ev.ICS = inv.ICS
	if err == nil {
		return s.DB.Save(&ev).Error
	}
	return s.DB.Create(&ev).Error
}

func cleanAddresses(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range in {
		for _, part := range strings.Split(v, ",") {
			a := strings.TrimSpace(part)
			if a == "" || seen[strings.ToLower(a)] {
				continue
			}
			seen[strings.ToLower(a)] = true
			out = append(out, a)
		}
	}
	return out
}

func logInvite(kind, user, uid string, err error) {
	log.Printf("invite %s for %s (%s) failed: %v", kind, user, uid, err)
}
