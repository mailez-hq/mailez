package api

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/models"
	"mailez/backend/internal/password"
)

func (h *Handler) registerUsers(r fiber.Router, mw fiber.Handler) {
	r.Get("/users", mw, h.listUsers)
	r.Post("/users", mw, h.createUser)
	r.Get("/users/:email", mw, h.getUser)
	r.Put("/users/:email", mw, h.updateUser)
	r.Delete("/users/:email", mw, h.deleteUser)
}

func (h *Handler) listUsers(c *fiber.Ctx) error {
	q := h.DB
	if u := currentUser(c); !u.GlobalAdmin {
		q = h.managedDomainScope(u, q)
	}
	var users []models.User
	if err := q.Find(&users).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(users)
}

func (h *Handler) getUser(c *fiber.Ctx) error {
	var u models.User
	if err := h.DB.First(&u, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if !h.canManageDomain(currentUser(c), u.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	return c.JSON(u)
}

func (h *Handler) createUser(c *fiber.Ctx) error {
	var in struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		QuotaBytes    int64  `json:"quota_bytes"`
		GlobalAdmin   bool   `json:"global_admin"`
		Enabled       *bool  `json:"enabled"`
		DisplayedName string `json:"displayed_name"`
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
	if !h.canManageDomain(currentUser(c), domainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
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
		QuotaBytes:    in.QuotaBytes,
		DisplayedName: in.DisplayedName,
	}
	if currentUser(c).GlobalAdmin {
		u.GlobalAdmin = in.GlobalAdmin
	}
	if in.Enabled != nil {
		u.Enabled = *in.Enabled
	}
	if err := h.DB.Create(&u).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	// GORM omits zero-value booleans carrying a `default` tag from INSERT, so
	// an explicitly disabled account would otherwise be stored as enabled.
	if in.Enabled != nil && !*in.Enabled {
		if err := h.DB.Model(&u).Update("enabled", false).Error; err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}
	return c.Status(201).JSON(u)
}

type updateUserIn struct {
	Password           string  `json:"password"`
	QuotaBytes         int64   `json:"quota_bytes"`
	GlobalAdmin        *bool   `json:"global_admin"`
	Enabled            *bool   `json:"enabled"`
	DisplayedName      string  `json:"displayed_name"`
	EnableImap         *bool   `json:"enable_imap"`
	EnablePop          *bool   `json:"enable_pop"`
	AllowSpoofing      *bool   `json:"allow_spoofing"`
	ForwardEnabled     *bool   `json:"forward_enabled"`
	ForwardDestination *string `json:"forward_destination"`
	ForwardKeep        *bool   `json:"forward_keep"`
	ReplyEnabled       *bool   `json:"reply_enabled"`
	ReplySubject       *string `json:"reply_subject"`
	ReplyBody          *string `json:"reply_body"`
	ReplyStartdate     *string `json:"reply_startdate"`
	ReplyEnddate       *string `json:"reply_enddate"`
	SpamEnabled        *bool   `json:"spam_enabled"`
	SpamMarkAsRead     *bool   `json:"spam_mark_as_read"`
	SpamThreshold      *int    `json:"spam_threshold"`
}

func (h *Handler) updateUser(c *fiber.Ctx) error {
	var u models.User
	if err := h.DB.First(&u, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if !h.canManageDomain(currentUser(c), u.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	var in updateUserIn
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Password != "" {
		hash, err := password.Hash(in.Password)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		u.Password = hash
	}
	if in.QuotaBytes != 0 {
		u.QuotaBytes = in.QuotaBytes
	}
	if in.GlobalAdmin != nil && currentUser(c).GlobalAdmin {
		u.GlobalAdmin = *in.GlobalAdmin
	}
	if in.Enabled != nil {
		u.Enabled = *in.Enabled
	}
	if in.DisplayedName != "" {
		u.DisplayedName = in.DisplayedName
	}
	if in.EnableImap != nil {
		u.EnableImap = *in.EnableImap
	}
	if in.EnablePop != nil {
		u.EnablePop = *in.EnablePop
	}
	if in.AllowSpoofing != nil {
		u.AllowSpoofing = *in.AllowSpoofing
	}
	if in.ForwardEnabled != nil {
		u.ForwardEnabled = *in.ForwardEnabled
	}
	if in.ForwardDestination != nil {
		u.ForwardDestination = *in.ForwardDestination
	}
	if in.ForwardKeep != nil {
		u.ForwardKeep = *in.ForwardKeep
	}
	if in.ReplyEnabled != nil {
		u.ReplyEnabled = *in.ReplyEnabled
	}
	if in.ReplySubject != nil {
		u.ReplySubject = *in.ReplySubject
	}
	if in.ReplyBody != nil {
		u.ReplyBody = *in.ReplyBody
	}
	if in.ReplyStartdate != nil {
		u.ReplyStartdate = parseUserDate(*in.ReplyStartdate)
	}
	if in.ReplyEnddate != nil {
		u.ReplyEnddate = parseUserDate(*in.ReplyEnddate)
	}
	if in.SpamEnabled != nil {
		u.SpamEnabled = *in.SpamEnabled
	}
	if in.SpamMarkAsRead != nil {
		u.SpamMarkAsRead = *in.SpamMarkAsRead
	}
	if in.SpamThreshold != nil {
		u.SpamThreshold = *in.SpamThreshold
	}
	if err := h.DB.Save(&u).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(u)
}

// parseUserDate accepts RFC3339 or plain "2006-01-02"; empty clears the date.
func parseUserDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (h *Handler) deleteUser(c *fiber.Ctx) error {
	var u models.User
	if err := h.DB.First(&u, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	if !h.canManageDomain(currentUser(c), u.DomainName) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "no access to this domain"})
	}
	if err := h.DB.Delete(&models.User{}, "email = ?", c.Params("email")).Error; err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}
