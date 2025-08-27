package contacts

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// orgContacts returns the read-only organization address book synced from
// AD/LDAP, shared by every mailbox user.
// @Summary Organization address book
// @Tags contacts
// @Produce json
// @Success 200 {array} models.OrgContact
// @Router /contacts/org [get]
func (h *Handler) orgContacts(c *fiber.Ctx) error {
	var rows []models.OrgContact
	if err := h.DB.Order("name, email").Find(&rows).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(rows)
}
