package mailbox

import (
	"strconv"
	"strings"
	"time"

	"mailez/backend/internal/core"
	"mailez/backend/internal/mail"

	"github.com/gofiber/fiber/v2"
)

func (h *Handler) registerMail(r fiber.Router) {
	r.Get("/mail/folders", h.mailFolders)
	r.Post("/mail/folders", h.mailFolderCreate)
	r.Put("/mail/folders", h.mailFolderRename)
	r.Delete("/mail/folders", h.mailFolderDelete)
	r.Post("/mail/folders/clear", h.mailFolderClear)
	r.Get("/mail/unseen", h.mailUnseen)
	r.Get("/mail/messages", h.mailMessages)
	r.Get("/mail/message", h.mailMessage)
	r.Get("/mail/raw", h.mailRaw)
	r.Get("/mail/thread", h.mailThread)
	r.Get("/mail/search", h.mailSearch)
	r.Post("/mail/search", h.mailSearchSpec)
	r.Post("/mail/flag", h.mailFlag)
	r.Post("/mail/snooze", h.mailSnooze)
	r.Get("/mail/snoozed", h.mailSnoozed)
	r.Post("/mail/move", h.mailMove)
	r.Post("/mail/delete", h.mailDelete)
	r.Post("/mail/unsubscribe", h.mailUnsubscribe)
	r.Get("/mail/labels", h.mailLabels)
	r.Post("/mail/labels", h.mailLabelSave)
	r.Post("/mail/labels/rename", h.mailLabelRename)
	r.Delete("/mail/labels", h.mailLabelDelete)
	r.Get("/mail/acl", h.mailACL)
	r.Put("/mail/acl", h.mailACLSet)
	r.Delete("/mail/acl", h.mailACLDelete)
}

// mailFlag adds or removes an IMAP flag on a message.
// @Summary Set message flag
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/flag [post]
func (h *Handler) mailFlag(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
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
	// Labels arrive as display names (possibly Chinese); map to the ASCII
	// keyword stored on the wire. System flags pass through unchanged.
	in.Flag = h.labelKeywordByName(mailboxIdentity(c), in.Flag)
	if err := h.Mail.With(d).SetFlag(d.Email, d.Token, in.Folder, in.UID, in.Flag, in.Value); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailSnooze snoozes a message until the given unix time, or wakes it back up
// when until is absent/zero.
// @Summary Snooze a message
// @Tags mail
// @Accept json
// @Success 204
// @Router /mail/snooze [post]
func (h *Handler) mailSnooze(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Folder string `json:"folder"`
		UID    uint32 `json:"uid"`
		Until  *int64 `json:"until"` // unix seconds; absent or 0 = wake up
	}
	if err := c.BodyParser(&in); err != nil || in.Folder == "" || in.UID == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "folder and uid are required"})
	}
	var until *time.Time
	if in.Until != nil && *in.Until > 0 {
		t := time.Unix(*in.Until, 0)
		until = &t
	}
	if err := h.Mail.With(d).Snooze(d.Email, d.Token, in.Folder, in.UID, until); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailSnoozed lists the caller's snoozed messages, resurfacing any that are due.
// @Summary List snoozed messages
// @Tags mail
// @Produce json
// @Success 200 {array} mail.SnoozedMessage
// @Router /mail/snoozed [get]
func (h *Handler) mailSnoozed(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	msgs, err := h.Mail.With(d).SnoozedMessages(d.Email, d.Token)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	names := h.labelNamesByKeyword(mailboxIdentity(c))
	for i := range msgs {
		msgs[i].Flags = decodeLabelFlags(msgs[i].Flags, names)
	}
	return c.JSON(msgs)
}

// mailMove moves messages to another folder.
// @Summary Move messages
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/move [post]
func (h *Handler) mailMove(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
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
	if err := h.Mail.With(d).MoveMany(d.Email, d.Token, in.Folder, uids, in.Destination); err != nil {
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
	d, err := h.MailDial(c)
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
	if err := h.Mail.With(d).Delete(d.Email, d.Token, in.Folder, in.UID); err != nil {
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
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folders, err := h.Mail.With(d).ListFolders(d.Email, d.Token)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(folders)
}

// validFolderName enforces mailbox-name rules: non-empty, no control
// characters, a sane length, and no "." / ".." or empty hierarchy parts.
// Non-ASCII (Unicode) names are allowed — the IMAP engine handles them
// (CREATE/LIST/RENAME/DELETE) and the move path was already creating them,
// so rejecting them here only made folders undeletable.
func validFolderName(name string) bool {
	if name == "" || len(name) > 200 {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	if name == "." || name == ".." {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" {
			return false
		}
	}
	return true
}

// mailFolderCreate creates a new (possibly nested) mailbox.
// @Summary Create folder
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/folders [post]
func (h *Handler) mailFolderCreate(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&in); err != nil || !validFolderName(in.Name) {
		return c.Status(400).JSON(fiber.Map{"error": "valid folder name is required"})
	}
	if err := h.Mail.With(d).CreateFolder(d.Email, d.Token, in.Name); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailFolderRename renames a mailbox. System folders are protected.
// @Summary Rename folder
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/folders [put]
func (h *Handler) mailFolderRename(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Name    string `json:"name"`
		NewName string `json:"new_name"`
	}
	if err := c.BodyParser(&in); err != nil || !validFolderName(in.Name) || !validFolderName(in.NewName) {
		return c.Status(400).JSON(fiber.Map{"error": "valid name and new_name are required"})
	}
	if mail.SystemFolders[strings.ToLower(in.Name)] {
		return c.Status(400).JSON(fiber.Map{"error": "cannot rename a system folder"})
	}
	if err := h.Mail.With(d).RenameFolder(d.Email, d.Token, in.Name, in.NewName); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailFolderDelete deletes a mailbox. System folders are protected.
// @Summary Delete folder
// @Tags mail
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/folders [delete]
func (h *Handler) mailFolderDelete(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	name := c.Query("name")
	if !validFolderName(name) {
		return c.Status(400).JSON(fiber.Map{"error": "valid folder name is required"})
	}
	if mail.SystemFolders[strings.ToLower(name)] {
		return c.Status(400).JSON(fiber.Map{"error": "cannot delete a system folder"})
	}
	if err := h.Mail.With(d).DeleteFolder(d.Email, d.Token, name); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailFolderClear empties a mailbox.
// @Summary Clear folder
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/folders/clear [post]
func (h *Handler) mailFolderClear(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&in); err != nil || !validFolderName(in.Name) {
		return c.Status(400).JSON(fiber.Map{"error": "valid folder name is required"})
	}
	if err := h.Mail.With(d).ClearFolder(d.Email, d.Token, in.Name); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailUnseen returns the unseen count per folder for the sidebar badges.
// @Summary Unseen counts
// @Tags mail
// @Produce json
// @Success 200 {object} map[string]int
// @Router /mail/unseen [get]
func (h *Handler) mailUnseen(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	counts, err := h.Mail.With(d).UnseenCounts(d.Email, d.Token)
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
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "INBOX")
	page, err := strconv.Atoi(c.Query("page", "0"))
	if err != nil || page < 0 {
		page = 0
	}
	sortBy := c.Query("sort", "date")
	dir := c.Query("dir", "")
	if sortBy != "date" && sortBy != "from" && sortBy != "subject" && sortBy != "size" {
		sortBy = "date"
	}
	messages, total, err := h.Mail.With(d).ListMessagesSorted(d.Email, d.Token, folder, page, sortBy, dir)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	c.Set("X-Total-Messages", strconv.Itoa(total))
	return c.JSON(h.decodeMessages(mailboxIdentity(c), messages))
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
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "INBOX")
	query := c.Query("q")
	if query == "" {
		return c.Status(400).JSON(fiber.Map{"error": "q is required"})
	}
	// label:<name> tokens carry display names; rewrite them to wire keywords.
	query = h.rewriteLabelTokens(mailboxIdentity(c), query)
	if strings.EqualFold(folder, "all") {
		messages, err := h.Mail.With(d).SearchAllMessages(d.Email, d.Token, query)
		if err != nil {
			return core.Fail(c, 502, err, "mail service error")
		}
		return c.JSON(h.decodeMessages(mailboxIdentity(c), messages))
	}
	messages, err := h.Mail.With(d).SearchMessages(d.Email, d.Token, folder, query)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(h.decodeMessages(mailboxIdentity(c), messages))
}

// mailSearchSpec runs a structured search query (no syntax parsing). The body
// is a JSON-encoded mail.SearchQuery; dates use RFC 3339.
// @Summary Search messages (structured)
// @Tags mail
// @Accept json
// @Produce json
// @Param body body object true "folder + structured query"
// @Success 200 {array} mail.Message
// @Router /mail/search [post]
func (h *Handler) mailSearchSpec(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Folder string           `json:"folder"`
		Query  mail.SearchQuery `json:"query"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid search body"})
	}
	// Structured label filters use display names; map to wire keywords.
	for i := range in.Query.Labels {
		in.Query.Labels[i] = h.labelKeywordByName(mailboxIdentity(c), in.Query.Labels[i])
	}
	if in.Folder == "" {
		in.Folder = "INBOX"
	}
	if strings.EqualFold(in.Folder, "all") {
		messages, err := h.Mail.With(d).SearchAllMessagesSpec(d.Email, d.Token, in.Query)
		if err != nil {
			return core.Fail(c, 502, err, "mail service error")
		}
		return c.JSON(h.decodeMessages(mailboxIdentity(c), messages))
	}
	messages, err := h.Mail.With(d).SearchMessagesSpec(d.Email, d.Token, in.Folder, in.Query)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(h.decodeMessages(mailboxIdentity(c), messages))
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
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "INBOX")
	tid := c.Query("thread_id")
	if tid == "" {
		return c.Status(400).JSON(fiber.Map{"error": "thread_id is required"})
	}
	messages, err := h.Mail.With(d).Thread(d.Email, d.Token, folder, tid)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	messages = h.decodeMessages(mailboxIdentity(c), messages)
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
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "INBOX")
	uid, err := strconv.ParseUint(c.Query("uid"), 10, 32)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid uid"})
	}
	raw, err := h.Mail.With(d).GetRaw(d.Email, d.Token, folder, uint32(uid))
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
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "INBOX")

	// A message may be addressed by its routable id (stable across mailboxes)
	// or directly by its IMAP uid. The id path resolves to the uid first; both
	// converge on the same fetch so callers always receive a message with uid.
	var msg *mail.Message
	if id := c.Query("id"); id != "" {
		uid, err := h.Mail.With(d).UIDByMessageID(d.Email, d.Token, folder, id)
		if err != nil {
			return core.Fail(c, 404, err, "message not found")
		}
		msg, err = h.Mail.With(d).GetMessage(d.Email, d.Token, folder, uid)
		if err != nil {
			return core.Fail(c, 502, err, "mail service error")
		}
	} else {
		uid, err := strconv.ParseUint(c.Query("uid"), 10, 32)
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid uid"})
		}
		msg, err = h.Mail.With(d).GetMessage(d.Email, d.Token, folder, uint32(uid))
		if err != nil {
			return core.Fail(c, 502, err, "mail service error")
		}
	}
	if msg != nil {
		msg.Flags = decodeLabelFlags(msg.Flags, h.labelNamesByKeyword(mailboxIdentity(c)))
	}
	return c.JSON(msg)
}

// mailACL returns the ACL entries and the caller's own rights for a folder.
// @Summary Get folder ACL
// @Tags mail
// @Produce json
// @Param folder query string true "mailbox"
// @Success 200 {object} map[string]interface{}
// @Router /mail/acl [get]
func (h *Handler) mailACL(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "")
	if folder == "" {
		return c.Status(400).JSON(fiber.Map{"error": "folder is required"})
	}
	entries, err := h.Mail.With(d).FolderACL(d.Email, d.Token, folder)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	rights, err := h.Mail.With(d).MyRights(d.Email, d.Token, folder)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.JSON(fiber.Map{"folder": folder, "entries": entries, "my_rights": rights})
}

// mailACLSet grants or replaces the rights of an identifier on a folder.
// @Summary Set folder ACL
// @Tags mail
// @Accept json
// @Success 204
// @Router /mail/acl [put]
func (h *Handler) mailACLSet(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	var in struct {
		Folder     string `json:"folder"`
		Identifier string `json:"identifier"`
		Rights     string `json:"rights"`
	}
	if err := c.BodyParser(&in); err != nil || in.Folder == "" || in.Identifier == "" {
		return c.Status(400).JSON(fiber.Map{"error": "folder and identifier are required"})
	}
	if err := h.Mail.With(d).SetFolderACL(d.Email, d.Token, in.Folder, in.Identifier, in.Rights); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailACLDelete removes every right of an identifier on a folder.
// @Summary Delete folder ACL entry
// @Tags mail
// @Param folder query string true "mailbox"
// @Param identifier query string true "user or group"
// @Success 204
// @Router /mail/acl [delete]
func (h *Handler) mailACLDelete(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder", "")
	identifier := c.Query("identifier", "")
	if folder == "" || identifier == "" {
		return c.Status(400).JSON(fiber.Map{"error": "folder and identifier are required"})
	}
	if err := h.Mail.With(d).DeleteFolderACL(d.Email, d.Token, folder, identifier); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}
