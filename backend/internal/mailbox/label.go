package mailbox

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/labelutil"
	"mailez/backend/internal/mail"
)

// validLabelName enforces user-facing label rules: non-empty, trimmed, no
// control characters, never a system flag (\...), and a name whose encoded
// IMAP keyword fits the 64-octet keyword atom limit. Non-ASCII display names
// (Chinese etc.) are allowed; the wire keyword is derived by EncodeKeyword.
func validLabelName(name string) bool {
	if name == "" || name != strings.TrimSpace(name) ||
		strings.HasPrefix(name, "\\") || utf8.RuneCountInString(name) > 64 {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	kw := labelutil.EncodeKeyword(name)
	return kw != "" && len(kw) <= labelutil.MaxKeywordBytes
}

// labelKeywordByName returns the wire keyword for a display name. System
// flags (\Seen, \Flagged, ...) and reserved keywords pass through untouched;
// unknown names fall back to the deterministic encoding so a label that was
// just saved races safely with the flag operation that follows it.
func (h *Handler) labelKeywordByName(userEmail, name string) string {
	if strings.HasPrefix(name, "\\") {
		return name
	}
	var label models.Label
	if err := h.DB.Where("user_email = ? AND name = ?", userEmail, name).
		First(&label).Error; err == nil && label.Keyword != "" {
		return label.Keyword
	}
	return labelutil.EncodeKeyword(name)
}

// labelNamesByKeyword loads the user's keyword→display-name map so message
// flags can be decoded back to the names the UI speaks. Keys are
// lowercased: go-imap canonicalizes unknown flags to lowercase on read and
// IMAP keywords are case-insensitive anyway.
func (h *Handler) labelNamesByKeyword(userEmail string) map[string]string {
	var labels []models.Label
	if err := h.DB.Where("user_email = ?", userEmail).Find(&labels).Error; err != nil {
		return nil
	}
	out := make(map[string]string, len(labels))
	for _, l := range labels {
		if l.Keyword != "" {
			out[strings.ToLower(l.Keyword)] = l.Name
		}
	}
	return out
}

// decodeLabelFlags rewrites encoded label keywords back to their display
// names; system flags and keywords without a label definition pass through.
func decodeLabelFlags(flags []string, names map[string]string) []string {
	if len(names) == 0 {
		return flags
	}
	out := make([]string, 0, len(flags))
	for _, f := range flags {
		if name, ok := names[strings.ToLower(f)]; ok {
			out = append(out, name)
		} else {
			out = append(out, f)
		}
	}
	return out
}

// decodeMessages decodes label keywords on every returned message so the API
// contract speaks display names end to end.
func (h *Handler) decodeMessages(userEmail string, msgs []mail.Message) []mail.Message {
	names := h.labelNamesByKeyword(userEmail)
	for i := range msgs {
		msgs[i].Flags = decodeLabelFlags(msgs[i].Flags, names)
	}
	return msgs
}

// labelTokenRe matches label:"..." and label:... tokens in a raw search
// expression, mirroring the tokenizer the mail package uses for parsing.
var labelTokenRe = regexp.MustCompile(`(?i)(label:"([^"]*)"|label:(\S+))`)

// rewriteLabelTokens replaces every label:<name> token in a search query with
// its wire keyword so the IMAP engine searches the encoded atom.
func (h *Handler) rewriteLabelTokens(userEmail, query string) string {
	return labelTokenRe.ReplaceAllStringFunc(query, func(m string) string {
		sub := labelTokenRe.FindStringSubmatch(m)
		name := sub[2]
		if name == "" {
			name = sub[3]
		}
		return "label:" + h.labelKeywordByName(userEmail, name)
	})
}

// mailLabels lists the user's label definitions (name + color).
// @Summary List labels
// @Tags mail
// @Produce json
// @Success 200 {array} models.Label
// @Router /mail/labels [get]
func (h *Handler) mailLabels(c *fiber.Ctx) error {
	var labels []models.Label
	if err := h.DB.Where("user_email = ?", mailboxIdentity(c)).Order("name").Find(&labels).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(labels)
}

// mailLabelSave creates or updates a label definition (color).
// @Summary Save label
// @Tags mail
// @Accept json
// @Success 200 {object} models.Label
// @Failure 400 {object} map[string]interface{}
// @Router /mail/labels [post]
func (h *Handler) mailLabelSave(c *fiber.Ctx) error {
	identity := mailboxIdentity(c)
	var in struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := c.BodyParser(&in); err != nil || !validLabelName(in.Name) {
		return c.Status(400).JSON(fiber.Map{"error": "valid label name is required"})
	}
	if len(in.Color) > 16 {
		return c.Status(400).JSON(fiber.Map{"error": "invalid color"})
	}
	kw := labelutil.EncodeKeyword(in.Name)
	var clash int64
	if err := h.DB.Model(&models.Label{}).
		Where("user_email = ? AND keyword = ? AND name <> ?", identity, kw, in.Name).
		Count(&clash).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	if clash > 0 {
		return c.Status(409).JSON(fiber.Map{"error": "label name maps to an existing label's keyword"})
	}

	res := h.DB.Model(&models.Label{}).
		Where("user_email = ? AND name = ?", identity, in.Name).
		Updates(map[string]interface{}{"color": in.Color, "keyword": kw})
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "db error")
	}
	label := models.Label{UserEmail: identity, Name: in.Name, Keyword: kw, Color: in.Color}
	if res.RowsAffected == 0 {
		if err := h.DB.Create(&label).Error; err != nil {
			return core.Fail(c, 500, err, "db error")
		}
	} else if err := h.DB.Where("user_email = ? AND name = ?", identity, in.Name).First(&label).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.JSON(label)
}

// mailLabelRename renames a label everywhere: the definition row plus the
// IMAP keyword on every message carrying it.
// @Summary Rename label
// @Tags mail
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/labels/rename [post]
func (h *Handler) mailLabelRename(c *fiber.Ctx) error {
	identity := mailboxIdentity(c)
	var in struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := c.BodyParser(&in); err != nil || !validLabelName(in.From) || !validLabelName(in.To) {
		return c.Status(400).JSON(fiber.Map{"error": "valid from and to names are required"})
	}
	var old models.Label
	if err := h.DB.Where("user_email = ? AND name = ?", identity, in.From).
		First(&old).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(404).JSON(fiber.Map{"error": "label not found"})
		}
		return core.Fail(c, 500, err, "db error")
	}
	oldKw := old.Keyword
	if oldKw == "" {
		oldKw = labelutil.EncodeKeyword(in.From)
	}
	newKw := labelutil.EncodeKeyword(in.To)
	var clash int64
	if err := h.DB.Model(&models.Label{}).
		Where("user_email = ? AND keyword = ? AND name <> ?", identity, newKw, in.To).
		Count(&clash).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	if clash > 0 {
		return c.Status(409).JSON(fiber.Map{"error": "new name maps to an existing label's keyword"})
	}
	d, err := h.MailDial(c)
	if err != nil {
		return core.DialFailure(c, err)
	}
	if err := h.Mail.With(d).ReplaceKeyword(d.Email, d.Token, oldKw, newKw); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	if err := h.DB.Model(&models.Label{}).
		Where("user_email = ? AND name = ?", identity, in.From).
		Updates(map[string]interface{}{"name": in.To, "keyword": newKw}).Error; err != nil {
		return core.Fail(c, 500, err, "db error")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// mailLabelDelete removes a label definition and strips the keyword from
// every message.
// @Summary Delete label
// @Tags mail
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /mail/labels [delete]
func (h *Handler) mailLabelDelete(c *fiber.Ctx) error {
	identity := mailboxIdentity(c)
	name := c.Query("name")
	if !validLabelName(name) {
		return c.Status(400).JSON(fiber.Map{"error": "valid label name is required"})
	}
	var label models.Label
	err := h.DB.Where("user_email = ? AND name = ?", identity, name).First(&label).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return core.Fail(c, 500, err, "db error")
	}
	kw := labelutil.EncodeKeyword(name)
	if err == nil && label.Keyword != "" {
		kw = label.Keyword
	}
	d, err := h.MailDial(c)
	if err != nil {
		return core.DialFailure(c, err)
	}
	if err := h.Mail.With(d).ReplaceKeyword(d.Email, d.Token, kw, ""); err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	if err == nil {
		if err := h.DB.Delete(&models.Label{}, label.ID).Error; err != nil {
			return core.Fail(c, 500, err, "db error")
		}
	}
	return c.SendStatus(fiber.StatusNoContent)
}
