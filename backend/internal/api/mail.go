package api

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) registerMail(r fiber.Router) {
	r.Get("/mail/folders", h.mailFolders)
	r.Get("/mail/messages", h.mailMessages)
	r.Get("/mail/message", h.mailMessage)
	r.Post("/mail/send", h.mailSend)
	r.Post("/mail/flag", h.mailFlag)
	r.Post("/mail/move", h.mailMove)
	r.Post("/mail/delete", h.mailDelete)
}

func (h *Handler) mailFlag(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Folder string `json:"folder"`
		UID    uint32 `json:"uid"`
		Flag   string `json:"flag"`
		Value  bool   `json:"value"`
	}
	if err := c.BodyParser(&in); err != nil || in.Folder == "" || in.UID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "folder and uid are required"})
	}
	if err := h.Mail.SetFlag(user.Email, token, in.Folder, in.UID, in.Flag, in.Value); err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *Handler) mailMove(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Folder      string `json:"folder"`
		UID         uint32 `json:"uid"`
		Destination string `json:"destination"`
	}
	if err := c.BodyParser(&in); err != nil || in.Folder == "" || in.UID == 0 || in.Destination == "" {
		return c.Status(400).JSON(fiber.Map{"error": "folder, uid and destination are required"})
	}
	if err := h.Mail.Move(user.Email, token, in.Folder, in.UID, in.Destination); err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}

func (h *Handler) mailDelete(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Folder string `json:"folder"`
		UID    uint32 `json:"uid"`
	}
	if err := c.BodyParser(&in); err != nil || in.Folder == "" || in.UID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "folder and uid are required"})
	}
	if err := h.Mail.Delete(user.Email, token, in.Folder, in.UID); err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}

// mailToken issues a temp token for the current session so the IMAP gateway can
// authenticate without ever touching the user's password.
func (h *Handler) mailToken(c *fiber.Ctx) (string, error) {
	user := currentUser(c)
	sid := c.Cookies(h.Auth.SessionName)
	return h.Auth.CreateTempToken(c.Context(), user.Email, sid)
}

func (h *Handler) mailFolders(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folders, err := h.Mail.ListFolders(user.Email, token)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(folders)
}

func (h *Handler) mailMessages(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "INBOX")
	messages, err := h.Mail.ListMessages(user.Email, token, folder)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(messages)
}

func (h *Handler) mailMessage(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "INBOX")
	uid, err := strconv.ParseUint(c.Query("uid"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid uid"})
	}
	msg, err := h.Mail.GetMessage(user.Email, token, folder, uint32(uid))
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(msg)
}

func (h *Handler) mailSend(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := c.BodyParser(&in); err != nil || in.To == "" {
		return c.Status(400).JSON(fiber.Map{"error": "to is required"})
	}
	if err := h.Mail.Send(user.Email, token, in.To, in.Subject, in.Body); err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusOK)
}
