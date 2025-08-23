package mailbox

import (
	"strconv"
	"strings"

	"mailez/backend/internal/core"
	"mailez/backend/internal/mail"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) registerMail(r fiber.Router) {
	r.Get("/mail/folders", h.mailFolders)
	r.Get("/mail/unseen", h.mailUnseen)
	r.Get("/mail/messages", h.mailMessages)
	r.Get("/mail/message", h.mailMessage)
	r.Get("/mail/raw", h.mailRaw)
	r.Get("/mail/thread", h.mailThread)
	r.Get("/mail/search", h.mailSearch)
	r.Post("/mail/flag", h.mailFlag)
	r.Post("/mail/move", h.mailMove)
	r.Post("/mail/delete", h.mailDelete)
}

// mailFlag adds or removes an IMAP flag on a message.
// @Summary Set message flag
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/flag [post]
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
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailMove moves messages to another folder.
// @Summary Move messages
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/move [post]
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
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailDelete moves a message to Trash.
// @Summary Delete message
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/delete [post]
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
		return core.Fail(c, 502, err, "mail service error")
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

// mailFolders lists the user's IMAP folders.
// @Summary List folders
// @Tags mail
// @Produce json
// @Success 200 {array} string
// @Router /mail/folders [get]
func (h *Handler) mailFolders(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folders, err := h.Mail.ListFolders(user.Email, token)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(folders)
}

// mailUnseen returns the unseen count per mailbox for the sidebar badges.
// mailUnseen returns the unseen count per folder.
// @Summary Unseen counts
// @Tags mail
// @Produce json
// @Success 200 {object} map[string]int
// @Router /mail/unseen [get]
func (h *Handler) mailUnseen(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	counts, err := h.Mail.UnseenCounts(user.Email, token)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(counts)
}

// mailMessages returns a page of messages; the total is in X-Total-Messages.
// @Summary List messages
// @Tags mail
// @Produce json
// @Param folder query string false "mailbox name" default(INBOX)
// @Param page query int false "page (0-based)" default(0)
// @Success 200 {array} mail.Message
// @Router /mail/messages [get]
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
		return core.Fail(c, 502, err, "mail service error")
	}
	c.Set("X-Total-Messages", strconv.Itoa(total))
	return c.JSON(messages)
}

// mailSearch searches messages in a folder or all folders.
// @Summary Search messages
// @Tags mail
// @Produce json
// @Param folder query string false "mailbox or all" default(INBOX)
// @Param q query string true "search expression"
// @Success 200 {array} mail.Message
// @Router /mail/search [get]
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
	if strings.EqualFold(folder, "all") {
		messages, err := h.Mail.SearchAllMessages(user.Email, token, query)
		if err != nil {
			return core.Fail(c, 502, err, "mail service error")
		}
		return c.JSON(messages)
	}
	messages, err := h.Mail.SearchMessages(user.Email, token, folder, query)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(messages)
}

// mailThread returns every message in a conversation.
// @Summary Message thread
// @Tags mail
// @Produce json
// @Param folder query string true "mailbox"
// @Param thread_id query string true "thread id"
// @Success 200 {object} map[string]interface{}
// @Router /mail/thread [get]
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
		return core.Fail(c, 502, err, "mail service error")
	}
	subject := ""
	if len(messages) > 0 {
		subject = messages[0].Subject
	}
	return c.JSON(fiber.Map{"thread_id": tid, "subject": subject, "messages": messages})
}

// mailRaw returns the raw RFC 822 source of a message.
// mailRaw returns the raw RFC 822 source of a message.
// @Summary Raw message
// @Tags mail
// @Produce json
// @Param folder query string true "mailbox"
// @Param uid query int true "message uid"
// @Success 200 {object} map[string]interface{}
// @Router /mail/raw [get]
func (h *Handler) mailRaw(c *fiber.Ctx) error {
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
	raw, err := h.Mail.GetRaw(user.Email, token, folder, uint32(uid))
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(fiber.Map{"raw": raw})
}

// mailMessage returns one full message.
// @Summary Get message
// @Tags mail
// @Produce json
// @Param folder query string true "mailbox"
// @Param uid query int true "message uid"
// @Success 200 {object} mail.Message
// @Router /mail/message [get]
func (h *Handler) mailMessage(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := h.mailToken(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "INBOX")

	// A message may be addressed by its routable id (stable across mailboxes)
	// or directly by its IMAP uid. The id path resolves to the uid first; both
	// converge on the same fetch so callers always receive a message with uid.
	var msg *mail.Message
	if id := c.Query("id"); id != "" {
		uid, err := h.Mail.UIDByMessageID(user.Email, token, folder, id)
		if err != nil {
			return core.Fail(c, 404, err, "message not found")
		}
		msg, err = h.Mail.GetMessage(user.Email, token, folder, uid)
		if err != nil {
			return core.Fail(c, 502, err, "mail service error")
		}
	} else {
		uid, err := strconv.ParseUint(c.Query("uid"), 10, 32)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid uid"})
		}
		msg, err = h.Mail.GetMessage(user.Email, token, folder, uint32(uid))
		if err != nil {
			return core.Fail(c, 502, err, "mail service error")
		}
	}
	return c.JSON(msg)
}
