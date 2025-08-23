package alias

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

func (h *Handler) registerAliases(r fiber.Router, mw fiber.Handler) {
	r.Get("/aliases", mw, h.listAliases)
	r.Post("/aliases", mw, h.createAlias)
	r.Get("/aliases/:email", mw, h.getAlias)
	r.Put("/aliases/:email", mw, h.updateAlias)
	r.Delete("/aliases/:email", mw, h.deleteAlias)
}

// listAliases returns aliases (manager/admin).
// @Summary List aliases
// @Tags aliases
// @Produce json
// @Success 200 {array} models.Alias
// @Failure 403 {object} models.APIError
// @Router /aliases [get]
func (h *Handler) listAliases(c *fiber.Ctx) error {
	q := h.DB
	if u := currentUser(c); !u.GlobalAdmin {
		q = h.ManagedDomainScope(u, q)
	}
	var aliases []models.Alias
	if err := q.Find(&aliases).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(aliases)
}

// getAlias returns one alias.
// @Summary Get alias
// @Tags aliases
// @Produce json
// @Param email path string true "alias address"
// @Success 200 {object} models.Alias
// @Router /aliases/{email} [get]
func (h *Handler) getAlias(c *fiber.Ctx) error {
	var a models.Alias
	if err := h.DB.First(&a, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "alias not found"})
	}
	if !h.CanManageDomain(currentUser(c), a.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	return c.JSON(a)
}

// createAlias adds an alias.
// @Summary Create alias
// @Tags aliases
// @Accept json
// @Produce json
// @Success 201 {object} models.Alias
// @Failure 400 {object} models.APIError
// @Router /aliases [post]
func (h *Handler) createAlias(c *fiber.Ctx) error {
	var in struct {
		Email       string `json:"email"`
		Destination string `json:"destination"`
		Wildcard    bool   `json:"wildcard"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	localpart, domainName, ok := strings.Cut(in.Email, "@")
	if !ok || localpart == "" || domainName == "" || in.Destination == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email and destination are required"})
	}
	var domain models.Domain
	if err := h.DB.First(&domain, "name = ?", domainName).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "domain does not exist"})
	}
	if !h.CanManageDomain(currentUser(c), domainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	a := models.Alias{
		Email:       in.Email,
		Localpart:   localpart,
		DomainName:  domainName,
		Destination: in.Destination,
		Wildcard:    in.Wildcard,
	}
	if err := h.DB.Create(&a).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(a)
}

// updateAlias updates an alias.
// @Summary Update alias
// @Tags aliases
// @Accept json
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /aliases/{email} [put]
func (h *Handler) updateAlias(c *fiber.Ctx) error {
	var a models.Alias
	if err := h.DB.First(&a, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "alias not found"})
	}
	if !h.CanManageDomain(currentUser(c), a.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	var in struct {
		Destination string `json:"destination"`
		Wildcard    *bool  `json:"wildcard"`
		Disabled    *bool  `json:"disabled"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Destination != "" {
		a.Destination = in.Destination
	}
	if in.Wildcard != nil {
		a.Wildcard = *in.Wildcard
	}
	if in.Disabled != nil {
		a.Disabled = *in.Disabled
	}
	if err := h.DB.Save(&a).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(a)
}

// deleteAlias removes an alias.
// @Summary Delete alias
// @Tags aliases
// @Param email path string true "alias address"
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /aliases/{email} [delete]
func (h *Handler) deleteAlias(c *fiber.Ctx) error {
	var a models.Alias
	if err := h.DB.First(&a, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "alias not found"})
	}
	if !h.CanManageDomain(currentUser(c), a.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	if err := h.DB.Delete(&a).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
