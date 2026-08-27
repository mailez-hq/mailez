package domain

import (
	"net/url"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func (h *Handler) registerManagers(r fiber.Router, mw fiber.Handler) {
	r.Get("/domains/:name/managers", mw, h.listDomainManagers)
	r.Post("/domains/:name/managers", mw, h.addDomainManager)
	r.Delete("/domains/:name/managers/:email", mw, h.removeDomainManager)
}

// listDomainManagers returns the users that administrate a domain.
// listDomainManagers returns the managers of a domain.
// @Summary List domain managers
// @Tags domains
// @Produce json
// @Param name path string true "domain name"
// @Success 200 {array} models.User
// @Router /domains/{name}/managers [get]
func (h *Handler) listDomainManagers(c *fiber.Ctx) error {
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	var managers []models.User
	if err := h.DB.Model(&d).Association("Managers").Find(&managers); err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(managers)
}

// addDomainManager grants a user manager rights over a domain.
// @Summary Add domain manager
// @Tags domains
// @Accept json
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /domains/{name}/managers [post]
func (h *Handler) addDomainManager(c *fiber.Ctx) error {
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	var user models.User
	if err := h.DB.First(&user, "email = ?", in.Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if err := h.DB.Model(&d).Association("Managers").Append(&user); err != nil {
		return core.Fail(c, 400, err, "add failed")
	}
	return c.SendStatus(204)
}

// removeDomainManager revokes a manager grant.
// @Summary Remove domain manager
// @Tags domains
// @Param name path string true "domain name"
// @Param email path string true "manager email"
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /domains/{name}/managers/{email} [delete]
func (h *Handler) removeDomainManager(c *fiber.Ctx) error {
	var d models.Domain
	if err := h.DB.First(&d, "name = ?", c.Params("name")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "domain not found"})
	}
	email, _ := url.QueryUnescape(c.Params("email"))
	var user models.User
	if err := h.DB.First(&user, "email = ?", email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if err := h.DB.Model(&d).Association("Managers").Delete(&user); err != nil {
		return core.Fail(c, 400, err, "remove failed")
	}
	return c.SendStatus(204)
}
