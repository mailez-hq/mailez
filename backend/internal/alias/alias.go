package alias

import (
	"fmt"
	"net/url"
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

// listAliases returns aliases (manager/admin), paginated.
// @Summary List aliases
// @Tags aliases
// @Produce json
// @Param page query int false "page number, 1-based"
// @Param limit query int false "page size"
// @Success 200 {object} models.Page
// @Failure 403 {object} models.APIError
// @Router /aliases [get]
func (h *Handler) listAliases(c *fiber.Ctx) error {
	q := h.DB
	if u := currentUser(c); !u.GlobalAdmin {
		q = h.ManagedDomainScope(u, q)
	}
	if c.Query("group") == "true" {
		q = q.Where("members <> ''")
	}
	page, limit := core.PageParams(c)
	var total int64
	if err := q.Model(&models.Alias{}).Count(&total).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	var aliases []models.Alias
	offset := (page - 1) * limit
	if err := q.Order("email").Limit(limit).Offset(offset).Find(&aliases).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return core.Page(c, aliases, int(total), page, limit)
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
	email, _ := url.PathUnescape(c.Params("email"))
	if err := h.DB.First(&a, "email = ?", email).Error; err != nil {
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
		Email       string               `json:"email"`
		Destination string               `json:"destination"`
		Name        string               `json:"name"`
		Members     []models.AliasMember `json:"members"`
		Wildcard    bool                 `json:"wildcard"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	localpart, domainName, ok := strings.Cut(in.Email, "@")
	if !ok || localpart == "" || domainName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email is required"})
	}
	if in.Destination == "" && len(in.Members) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "destination or members are required"})
	}
	members, err := normalizeMembers(in.Members)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	var domain models.Domain
	if err := h.DB.First(&domain, "name = ?", domainName).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "domain does not exist"})
	}
	if !h.CanManageDomain(currentUser(c), domainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	var exists int64
	if err := h.DB.Model(&models.Alias{}).Where("email = ?", in.Email).Count(&exists).Error; err == nil && exists > 0 {
		return c.Status(fiber.StatusConflict).JSON(fiber.Map{"error": "alias already exists"})
	}
	a := models.Alias{
		Email:       in.Email,
		Localpart:   localpart,
		DomainName:  domainName,
		Destination: in.Destination,
		Name:        in.Name,
		Wildcard:    in.Wildcard,
	}
	a.SetMembers(members)
	if err := h.DB.Create(&a).Error; err != nil {
		return core.Fail(c, 400, err, "save failed")
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
	email, _ := url.PathUnescape(c.Params("email"))
	if err := h.DB.First(&a, "email = ?", email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "alias not found"})
	}
	if !h.CanManageDomain(currentUser(c), a.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	var in struct {
		Destination *string               `json:"destination"`
		Name        *string               `json:"name"`
		Members     *[]models.AliasMember `json:"members"`
		Wildcard    *bool                 `json:"wildcard"`
		Disabled    *bool                 `json:"disabled"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Destination != nil {
		a.Destination = strings.TrimSpace(*in.Destination)
	}
	if in.Name != nil {
		a.Name = strings.TrimSpace(*in.Name)
	}
	if in.Members != nil {
		members, err := normalizeMembers(*in.Members)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		a.SetMembers(members)
	}
	if in.Wildcard != nil {
		a.Wildcard = *in.Wildcard
	}
	if in.Disabled != nil {
		a.Disabled = *in.Disabled
	}
	if err := h.DB.Save(&a).Error; err != nil {
		return core.Fail(c, 400, err, "update failed")
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
	email, _ := url.PathUnescape(c.Params("email"))
	if err := h.DB.First(&a, "email = ?", email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "alias not found"})
	}
	if !h.CanManageDomain(currentUser(c), a.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	if err := h.DB.Delete(&a).Error; err != nil {
		return core.Fail(c, 400, err, "delete failed")
	}
	return c.SendStatus(204)
}

// normalizeMembers trims, validates and deduplicates a distribution-group
// member list. Every member must be a plausible email address.
func normalizeMembers(members []models.AliasMember) ([]models.AliasMember, error) {
	if len(members) == 0 {
		return nil, nil
	}
	out := make([]models.AliasMember, 0, len(members))
	seen := map[string]bool{}
	for _, m := range members {
		m.Email = strings.ToLower(strings.TrimSpace(m.Email))
		m.Name = strings.TrimSpace(m.Name)
		if m.Email == "" {
			return nil, fmt.Errorf("member email is required")
		}
		if !strings.Contains(m.Email, "@") || strings.ContainsAny(m.Email, " \t\n") {
			return nil, fmt.Errorf("invalid member email: %s", m.Email)
		}
		if seen[m.Email] {
			continue
		}
		seen[m.Email] = true
		out = append(out, m)
	}
	return out, nil
}
