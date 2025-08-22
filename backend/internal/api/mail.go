package api

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) registerMail(r fiber.Router) {
	r.Get("/mail/folders", h.mailFolders)
	r.Get("/mail/messages", h.mailMessages)
	r.Get("/mail/message", h.mailMessage)
	r.Get("/mail/thread", h.mailThread)
	r.Get("/mail/search", h.mailSearch)
	r.Post("/mail/send", h.mailSend)
	r.Post("/mail/flag", h.mailFlag)
	r.Post("/mail/move", h.mailMove)
	r.Post("/mail/delete", h.mailDelete)
	r.Get("/mail/identities", h.mailIdentities)
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
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) mailMove(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Folder      string   `json:"folder"`
		UID         uint32   `json:"uid"`
		Uids        []uint32 `json:"uids"`
		Destination string   `json:"destination"`
	}
	if err := c.BodyParser(&in); err != nil || in.Folder == "" || in.Destination == "" {
		return c.Status(400).JSON(fiber.Map{"error": "folder and destination are required"})
	}
	uids := in.Uids
	if len(uids) == 0 {
		if in.UID == 0 {
			return c.Status(400).JSON(fiber.Map{"error": "uid or uids are required"})
		}
		uids = []uint32{in.UID}
	}
	if err := h.Mail.MoveMany(user.Email, token, in.Folder, uids, in.Destination); err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
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
	return c.SendStatus(fiber.StatusNoContent)
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
	page, err := strconv.Atoi(c.Query("page", "0"))
	if err != nil || page < 0 {
		page = 0
	}
	messages, total, err := h.Mail.ListMessages(user.Email, token, folder, page)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	c.Set("X-Total-Messages", strconv.Itoa(total))
	return c.JSON(messages)
}

func (h *Handler) mailSearch(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "INBOX")
	query := c.Query("q")
	if query == "" {
		return c.Status(400).JSON(fiber.Map{"error": "q is required"})
	}
	messages, err := h.Mail.SearchMessages(user.Email, token, folder, query)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(messages)
}

func (h *Handler) mailThread(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "INBOX")
	tid := c.Query("thread_id")
	if tid == "" {
		return c.Status(400).JSON(fiber.Map{"error": "thread_id is required"})
	}
	messages, err := h.Mail.Thread(user.Email, token, folder, tid)
	if err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	subject := ""
	if len(messages) > 0 {
		subject = messages[0].Subject
	}
	return c.JSON(fiber.Map{"thread_id": tid, "subject": subject, "messages": messages})
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
		From    string `json:"from"`
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
		HTML    string `json:"html"`
	}
	if err := c.BodyParser(&in); err != nil || in.To == "" {
		return c.Status(400).JSON(fiber.Map{"error": "to is required"})
	}
	from := in.From
	if from == "" {
		from = user.Email
	}
	if !h.userMaySendAs(user, from) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "cannot send as this identity"})
	}
	if err := h.Mail.Send(user.Email, token, from, in.To, in.Subject, in.Body, in.HTML); err != nil {
		return c.Status(502).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}
