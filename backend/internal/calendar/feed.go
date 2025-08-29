package calendar

// Calendar subscription feed: a stateless HMAC token authenticates external
// subscribers (Apple/Google Calendar) without a webmail session.
import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"

	ics "github.com/arran4/golang-ical"
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/caldav"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// feedToken is a stateless HMAC over the user email so the subscription URL
// needs no stored secret and can be revoked by rotating the server secret.
func feedToken(email, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte("mailez-calendar-feed:" + email))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func feedValid(email, token, secret string) bool {
	if token == "" {
		return false
	}
	want := feedToken(email, secret)
	return hmac.Equal([]byte(token), []byte(want))
}

// calendarFeed returns the caller's ICS subscription URL (token included).
// @Summary Calendar subscription URL
// @Tags calendar
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /calendar/feed [get]
func (h *Handler) calendarFeed(c *fiber.Ctx) error {
	user := currentUser(c)
	token := feedToken(user.Email, h.Cfg.SecretKey)
	return c.JSON(fiber.Map{
		"url": c.BaseURL() + "/api/v1/calendar/export.ics?email=" +
			user.Email + "&token=" + token,
	})
}

// exportICS streams the account's calendar as a subscribable VCALENDAR.
// @Summary Export calendar as ICS
// @Tags calendar
// @Produce text/calendar
// @Success 200 {string} string
// @Router /calendar/export.ics [get]
func (h *Handler) exportICS(c *fiber.Ctx) error {
	email := strings.ToLower(strings.TrimSpace(c.Query("email")))
	token := c.Query("token")
	if !feedValid(email, token, h.Cfg.SecretKey) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	var events []models.CalendarEvent
	if err := h.DB.Where("user_email = ?", email).Order("start").Find(&events).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	cal := ics.NewCalendar()
	cal.SetMethod(ics.MethodPublish)
	cal.SetProductId("-//Mailez//Mailez Calendar//CN")
	for _, ev := range events {
		d := &caldav.EventData{
			UID:         ev.UID,
			Summary:     ev.Summary,
			Location:    ev.Location,
			Description: ev.Description,
			RRule:       ev.RRule,
			AllDay:      ev.AllDay,
			Start:       ev.Start,
			End:         ev.End,
		}
		raw, err := caldav.BuildICS(d)
		if err != nil {
			continue
		}
		parsed, err := ics.ParseCalendar(strings.NewReader(raw))
		if err != nil || len(parsed.Events()) == 0 {
			continue
		}
		cal.AddVEvent(parsed.Events()[0])
	}
	c.Set("Content-Type", "text/calendar; charset=utf-8")
	c.Set("Content-Disposition", `attachment; filename="calendar.ics"`)
	return c.SendString(cal.Serialize())
}
