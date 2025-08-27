package calendar

// Calendar sharing, ICS export/subscription feed and permission helpers.
import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"

	ics "github.com/arran4/golang-ical"
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/caldav"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// canModify reports whether the account may edit/delete the event: the owner,
// or a sharee holding a read-write grant.
func (h *Handler) canModify(ev *models.CalendarEvent, email string) bool {
	if ev.UserEmail == email {
		return true
	}
	var share models.CalendarShare
	err := h.DB.Where("owner_email = ? AND sharee_email = ? AND read_only = ?",
		ev.UserEmail, email, false).First(&share).Error
	return err == nil
}

// listShares returns the calendars this account shares (owned) and the
// calendars shared with it (granted).
// @Summary List calendar shares
// @Tags calendar
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /calendar/shares [get]
func (h *Handler) listShares(c *fiber.Ctx) error {
	user := currentUser(c)
	var owned, granted []models.CalendarShare
	if err := h.DB.Where("owner_email = ?", user.Email).Find(&owned).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	if err := h.DB.Where("sharee_email = ?", user.Email).Find(&granted).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(fiber.Map{
		"owned":   sharesView(owned),
		"granted": sharesView(granted),
	})
}

func sharesView(shares []models.CalendarShare) []fiber.Map {
	out := make([]fiber.Map, 0, len(shares))
	for _, s := range shares {
		out = append(out, fiber.Map{
			"id":           s.ID,
			"owner_email":  s.OwnerEmail,
			"sharee_email": s.ShareeEmail,
			"read_only":    s.ReadOnly,
		})
	}
	return out
}

// createShare grants another account access to the caller's calendar.
// @Summary Share calendar
// @Tags calendar
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /calendar/shares [post]
func (h *Handler) createShare(c *fiber.Ctx) error {
	user := currentUser(c)
	var in struct {
		ShareeEmail string `json:"sharee_email"`
		ReadOnly    bool   `json:"read_only"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	sharee := strings.ToLower(strings.TrimSpace(in.ShareeEmail))
	if sharee == "" {
		return c.Status(400).JSON(fiber.Map{"error": "sharee_email is required"})
	}
	if strings.EqualFold(sharee, user.Email) {
		return c.Status(400).JSON(fiber.Map{"error": "cannot share with yourself"})
	}
	var target models.User
	if err := h.DB.First(&target, "LOWER(email) = ?", sharee).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "sharee not found"})
	}
	var existing models.CalendarShare
	if err := h.DB.Where("owner_email = ? AND sharee_email = ?", user.Email, target.Email).
		First(&existing).Error; err == nil {
		existing.ReadOnly = in.ReadOnly
		if err := h.DB.Save(&existing).Error; err != nil {
			return core.Fail(c, 500, err, "db error")
		}
		return c.JSON(sharesView([]models.CalendarShare{existing})[0])
	}
	share := models.CalendarShare{OwnerEmail: user.Email, ShareeEmail: target.Email, ReadOnly: in.ReadOnly}
	if err := h.DB.Create(&share).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(sharesView([]models.CalendarShare{share})[0])
}

// deleteShare revokes a calendar grant.
// @Summary Revoke calendar share
// @Tags calendar
// @Success 204
// @Router /calendar/shares/{id} [delete]
func (h *Handler) deleteShare(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	res := h.DB.Where("id = ? AND owner_email = ?", id, user.Email).Delete(&models.CalendarShare{})
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "db error")
	}
	if res.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "share not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

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
