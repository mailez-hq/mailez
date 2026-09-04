package calendar

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

var currentUser = core.CurrentUser

// listEvents returns events overlapping the ?from=&to= window (RFC3339).
// @Summary List calendar events
// @Tags calendar
// @Produce json
// @Param from query string false "window start (RFC3339)"
// @Param to query string false "window end (RFC3339)"
// @Success 200 {array} map[string]interface{}
// @Router /calendar/events [get]
func (h *Handler) listEvents(c *fiber.Ctx) error {
	user := currentUser(c)
	// Own events plus events from calendars shared with this account.
	owners, shares := h.visibleCalendars(user.Email)
	q := h.DB.Where("user_email IN ?", owners)
	if from := c.Query("from"); from != "" {
		if t, err := time.Parse(time.RFC3339, from); err == nil {
			// "end" is a reserved word on PostgreSQL — qualify with the table
			// name (portable across MySQL/SQLite/Postgres).
			q = q.Where("(calendar_event.end IS NULL OR calendar_event.end >= ?)", t)
		}
	}
	if to := c.Query("to"); to != "" {
		if t, err := time.Parse(time.RFC3339, to); err == nil {
			q = q.Where("(calendar_event.start IS NULL OR calendar_event.start <= ?)", t)
		}
	}
	var events []models.CalendarEvent
	if err := q.Order("start").Find(&events).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	out := make([]fiber.Map, 0, len(events))
	for i := range events {
		v := view(&events[i])
		if events[i].UserEmail == user.Email {
			v["owner_email"] = ""
		} else {
			v["read_only"] = true
			for _, s := range shares {
				if s.OwnerEmail == events[i].UserEmail {
					v["read_only"] = s.ReadOnly
					break
				}
			}
		}
		out = append(out, v)
	}
	return c.JSON(out)
}

// createEvent stores a new event and returns it.
// @Summary Create calendar event
// @Tags calendar
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /calendar/events [post]
func (h *Handler) createEvent(c *fiber.Ctx) error {
	user := currentUser(c)
	var in eventInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	ev := models.CalendarEvent{UserEmail: user.Email}
	if err := in.apply(&ev); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.DB.Create(&ev).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(view(&ev))
}

// updateEvent edits an existing event.
// @Summary Update calendar event
// @Tags calendar
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /calendar/events/{id} [put]
func (h *Handler) updateEvent(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var ev models.CalendarEvent
	if err := h.DB.First(&ev, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "event not found"})
	}
	if !h.canModify(&ev, user.Email) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "read-only calendar"})
	}
	var in eventInput
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if err := in.apply(&ev); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if err := h.DB.Save(&ev).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(view(&ev))
}

// deleteEvent removes an event.
// @Summary Delete calendar event
// @Tags calendar
// @Success 204
// @Router /calendar/events/{id} [delete]
func (h *Handler) deleteEvent(c *fiber.Ctx) error {
	user := currentUser(c)
	id, err := strconv.ParseUint(c.Params("id"), 10, 64)
	if err != nil || id == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var ev models.CalendarEvent
	if err := h.DB.First(&ev, "id = ?", id).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "event not found"})
	}
	if !h.canModify(&ev, user.Email) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "read-only calendar"})
	}
	res := h.DB.Delete(&models.CalendarEvent{}, id)
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "db error")
	}
	if res.RowsAffected == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "event not found"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
