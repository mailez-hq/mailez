package mailbox

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"path"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
)

// mailAttachmentsZip packs every attachment of one message into a zip and
// streams it to the client (Gmail/SnappyMail "download all attachments").
// @Summary Download all attachments as zip
// @Tags mail
// @Produce application/zip
// @Param folder query string true "mailbox folder"
// @Param uid query int true "message uid"
// @Success 200 {file} binary
// @Router /mail/attachments/zip [get]
func (h *Handler) mailAttachmentsZip(c *fiber.Ctx) error {
	d, err := h.MailDial(c)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "token error"})
	}
	folder := c.Query("folder")
	uid64, err := strconv.ParseUint(c.Query("uid"), 10, 32)
	uid := uint32(uid64)
	if err != nil || folder == "" {
		return c.Status(400).JSON(fiber.Map{"error": "folder and uid are required"})
	}
	msg, err := h.Mail.With(d).GetMessage(d.Email, d.Token, folder, uid)
	if err != nil {
		return core.Fail(c, 502, err, "mail service error")
	}
	if len(msg.Attachments) == 0 {
		return c.Status(404).JSON(fiber.Map{"error": "no attachments"})
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	used := map[string]bool{}
	for _, att := range msg.Attachments {
		name := zipEntryName(att.Filename, used)
		w, err := zw.Create(name)
		if err != nil {
			return core.Fail(c, 500, err, "zip error")
		}
		data, err := base64.StdEncoding.DecodeString(att.Data)
		if err != nil {
			data = []byte(att.Data)
		}
		if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
			return core.Fail(c, 500, err, "zip error")
		}
	}
	if err := zw.Close(); err != nil {
		return core.Fail(c, 500, err, "zip error")
	}
	base := zipBaseName(msg.Subject)
	c.Set("Content-Type", "application/zip")
	c.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.zip"`, mime.QEncoding.Encode("utf-8", base)))
	return c.Send(buf.Bytes())
}

// zipEntryName dedupes attachment filenames inside the archive.
func zipEntryName(name string, used map[string]bool) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "attachment"
	}
	name = path.Base(name)
	key := name
	for i := 2; used[key]; i++ {
		ext := path.Ext(name)
		stem := strings.TrimSuffix(name, ext)
		key = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
	used[key] = true
	return key
}

// zipBaseName turns a message subject into a safe zip file base name.
func zipBaseName(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "attachments"
	}
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_",
		"<", "_", ">", "_", "|", "_", "\n", " ", "\r", " ",
	)
	subject = replacer.Replace(subject)
	if len(subject) > 60 {
		subject = subject[:60]
	}
	return strings.TrimSpace(subject)
}
