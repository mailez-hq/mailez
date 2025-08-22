package api

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailess/backend/internal/models"
	"mailess/backend/internal/password"
)

// registerSignup mounts the public self-registration endpoints. These must be
// registered before requireAuth so unauthenticated users can reach them.
func (h *Handler) registerSignup(r fiber.Router) {
	r.Get("/signup/domains", h.signupDomains)
	r.Post("/signup", h.signup)
}

// signupDomains lists domains that accept self-registration, with the number
// of existing users so the form can disable full domains.
func (h *Handler) signupDomains(c *fiber.Ctx) error {
	var domains []models.Domain
	if err := h.DB.Where("signup_enabled = ?", true).Order("name").Find(&domains).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	type domainInfo struct {
		Name       string `json:"name"`
		MaxUsers   int    `json:"max_users"`
		UserCount  int64  `json:"user_count"`
		SignupFull bool   `json:"signup_full"`
	}
	out := make([]domainInfo, 0, len(domains))
	for _, d := range domains {
		var count int64
		if err := h.DB.Model(&models.User{}).Where("domain_name = ?", d.Name).Count(&count).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		full := d.MaxUsers >= 0 && count >= int64(d.MaxUsers)
		out = append(out, domainInfo{Name: d.Name, MaxUsers: d.MaxUsers, UserCount: count, SignupFull: full})
	}
	return c.JSON(out)
}

// signup creates a user account on a domain with self-registration enabled.
func (h *Handler) signup(c *fiber.Ctx) error {
	var in struct {
		Email         string `json:"email"`
		Password      string `json:"pw"`
		DisplayedName string `json:"displayed_name"`
		SpamEnabled   *bool  `json:"spam_enabled"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Email == "" || in.Password == "" {
		return c.Status(400).JSON(fiber.Map{"error": "email and password are required"})
	}
	localpart, domainName, ok := strings.Cut(in.Email, "@")
	if !ok || localpart == "" || domainName == "" {
		return c.Status(400).JSON(fiber.Map{"error": "invalid email"})
	}
	var domain models.Domain
	if err := h.DB.First(&domain, "name = ?", domainName).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "domain does not exist"})
	}
	if !domain.SignupEnabled {
		return c.Status(403).JSON(fiber.Map{"error": "self-signup is disabled for this domain"})
	}
	var existing int64
	if err := h.DB.Model(&models.User{}).Where("email = ?", in.Email).Count(&existing).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if existing > 0 {
		return c.Status(409).JSON(fiber.Map{"error": "this address is already taken"})
	}
	if domain.MaxUsers >= 0 {
		var users int64
		if err := h.DB.Model(&models.User{}).Where("domain_name = ?", domainName).Count(&users).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		if users >= int64(domain.MaxUsers) {
			return c.Status(403).JSON(fiber.Map{"error": "domain user limit reached"})
		}
	}
	hash, err := password.Hash(in.Password)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	u := models.User{
		Email:         in.Email,
		Localpart:     localpart,
		DomainName:    domainName,
		Password:      hash,
		DisplayedName: in.DisplayedName,
	}
	if in.SpamEnabled != nil {
		u.SpamEnabled = *in.SpamEnabled
	}
	if err := h.DB.Create(&u).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(201).JSON(fiber.Map{"email": u.Email})
}
