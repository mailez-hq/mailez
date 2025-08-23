package contacts

import (
	"strconv"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// registerContacts mounts self-service address book endpoints. A user only
// ever sees and edits their own entries.
func (h *Handler) registerContacts(r fiber.Router) {
	r.Get("/contacts", h.listContacts)
	r.Post("/contacts", h.createContact)
	r.Put("/contacts/:id", h.updateContact)
	r.Delete("/contacts/:id", h.deleteContact)
}

// listContacts returns the caller's address book.
// @Summary List contacts
// @Tags contacts
// @Produce json
// @Success 200 {array} models.Contact
// @Router /contacts [get]
func (h *Handler) listContacts(c *fiber.Ctx) error {
	var contacts []models.Contact
	if err := h.DB.Where("user_email = ?", currentUser(c).Email).Order("name").Find(&contacts).Error; err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	return c.JSON(contacts)
}

// createContact adds an address book entry.
// @Summary Create contact
// @Tags contacts
// @Accept json
// @Produce json
// @Success 201 {object} models.Contact
// @Failure 400 {object} models.APIError
// @Router /contacts [post]
func (h *Handler) createContact(c *fiber.Ctx) error {
	var in struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Comment string `json:"comment"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Name == "" || in.Email == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name and email are required"})
	}
	contact := models.Contact{
		UserEmail: currentUser(c).Email,
		Name:      in.Name,
		Email:     in.Email,
		Comment:   in.Comment,
	}
	if err := h.DB.Create(&contact).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(contact)
}

// updateContact updates an address book entry.
// @Summary Update contact
// @Tags contacts
// @Accept json
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /contacts/{id} [put]
func (h *Handler) updateContact(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	var contact models.Contact
	if err := h.DB.First(&contact, "id = ? AND user_email = ?", id, currentUser(c).Email).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "contact not found"})
	}
	var in struct {
		Name    *string `json:"name"`
		Email   *string `json:"email"`
		Comment *string `json:"comment"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Name != nil {
		contact.Name = *in.Name
	}
	if in.Email != nil {
		contact.Email = *in.Email
	}
	if in.Comment != nil {
		contact.Comment = *in.Comment
	}
	if err := h.DB.Save(&contact).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(contact)
}

// deleteContact removes an address book entry.
// @Summary Delete contact
// @Tags contacts
// @Param id path int true "contact id"
// @Success 204
// @Failure 400 {object} models.APIError
// @Router /contacts/{id} [delete]
func (h *Handler) deleteContact(c *fiber.Ctx) error {
	id, err := strconv.ParseUint(c.Params("id"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
	}
	if err := h.DB.Delete(&models.Contact{}, "id = ? AND user_email = ?", id, currentUser(c).Email).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
