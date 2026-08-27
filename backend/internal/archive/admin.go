package archive

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// listSettings returns the global policy and every per-domain override.
func (s *Service) listSettings(c *fiber.Ctx) error {
	var global models.ArchiveSettings
	gErr := s.DB.WithContext(c.Context()).Where("domain = ?", "").First(&global).Error
	if gErr != nil && !isNotFound(gErr) {
		return core.Fail(c, 500, gErr, "load settings failed")
	}
	if isNotFound(gErr) {
		global = models.ArchiveSettings{
			Enabled:         true,
			CaptureInbound:  true,
			CaptureOutbound: true,
		}
	}
	var domains []models.ArchiveSettings
	if err := s.DB.WithContext(c.Context()).Where("domain <> ?", "").Order("domain").Find(&domains).Error; err != nil {
		return core.Fail(c, 500, err, "load settings failed")
	}
	return c.JSON(fiber.Map{"global": global, "domains": domains})
}

// updateSettings upserts one policy row (global when domain is empty).
func (s *Service) updateSettings(c *fiber.Ctx) error {
	var body struct {
		Domain          string `json:"domain"`
		Enabled         *bool  `json:"enabled"`
		CaptureInbound  *bool  `json:"capture_inbound"`
		CaptureOutbound *bool  `json:"capture_outbound"`
		RetentionDays   *int   `json:"retention_days"`
	}
	if err := c.BodyParser(&body); err != nil {
		return core.Fail(c, 400, err, "bad request")
	}
	domain := strings.ToLower(strings.TrimSpace(body.Domain))
	var row models.ArchiveSettings
	err := s.DB.WithContext(c.Context()).Where("domain = ?", domain).First(&row).Error
	if isNotFound(err) {
		row = models.ArchiveSettings{Domain: domain}
	} else if err != nil {
		return core.Fail(c, 500, err, "load settings failed")
	}
	if body.Enabled != nil {
		row.Enabled = *body.Enabled
	}
	if body.CaptureInbound != nil {
		row.CaptureInbound = *body.CaptureInbound
	}
	if body.CaptureOutbound != nil {
		row.CaptureOutbound = *body.CaptureOutbound
	}
	if body.RetentionDays != nil {
		if *body.RetentionDays < 0 {
			return core.Fail(c, 400, nil, "retention_days must be >= 0")
		}
		row.RetentionDays = *body.RetentionDays
	}
	// Select("*") forces zero-valued booleans into the statement; the
	// model's default:true tags would otherwise silently turn an explicit
	// false back into true on create.
	if err := s.DB.WithContext(c.Context()).Select("*").Save(&row).Error; err != nil {
		return core.Fail(c, 500, err, "save settings failed")
	}
	return c.JSON(row)
}

// listMessages searches the archive with metadata filters.
func (s *Service) listMessages(c *fiber.Ctx) error {
	page, limit := core.PageParams(c)
	q := s.DB.WithContext(c.Context()).Model(&models.ArchivedMessage{})
	if v := c.Query("direction"); v != "" {
		q = q.Where("direction = ?", v)
	}
	if v := c.Query("domain"); v != "" {
		q = q.Where("domain = ?", v)
	}
	if v := c.Query("from"); v != "" {
		q = q.Where("LOWER(`from`) LIKE ? OR LOWER(envelope_from) LIKE ?", "%"+strings.ToLower(v)+"%", "%"+strings.ToLower(v)+"%")
	}
	if v := c.Query("to"); v != "" {
		q = q.Where("LOWER(`to`) LIKE ? OR LOWER(envelope_to) LIKE ?", "%"+strings.ToLower(v)+"%", "%"+strings.ToLower(v)+"%")
	}
	if v := strings.TrimSpace(c.Query("q")); v != "" {
		like := "%" + strings.ToLower(v) + "%"
		q = q.Where("LOWER(subject) LIKE ? OR LOWER(`from`) LIKE ? OR LOWER(`to`) LIKE ? OR LOWER(message_id) LIKE ?",
			like, like, like, like)
	}
	if v := c.Query("date_from"); v != "" {
		if t, err := parseQueryTime(v); err == nil {
			q = q.Where("date >= ?", t)
		}
	}
	if v := c.Query("date_to"); v != "" {
		if t, err := parseQueryTime(v); err == nil {
			q = q.Where("date <= ?", t)
		}
	}
	if v := c.Query("reviewed"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			q = q.Where("reviewed = ?", b)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return core.Fail(c, 500, err, "search failed")
	}
	var rows []models.ArchivedMessage
	if err := q.Order("archived_at desc").Limit(limit).Offset((page - 1) * limit).Find(&rows).Error; err != nil {
		return core.Fail(c, 500, err, "search failed")
	}
	for i := range rows {
		rows[i].Raw = nil // never ship raw bytes in list responses
	}
	return core.Page(c, rows, int(total), page, limit)
}

// getMessage returns one archived message's metadata and a rendered body
// preview for review.
func (s *Service) getMessage(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return core.Fail(c, 400, err, "bad id")
	}
	var row models.ArchivedMessage
	if err := s.DB.WithContext(c.Context()).First(&row, id).Error; err != nil {
		return core.Fail(c, 404, err, "message not found")
	}
	body := renderPreview(row.Raw)
	row.Raw = nil
	return c.JSON(fiber.Map{"message": row, "preview": body})
}

// rawMessage streams the original RFC 5322 bytes as an .eml download. The
// audit middleware records the reviewer's identity for the 稽核 trail.
func (s *Service) rawMessage(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return core.Fail(c, 400, err, "bad id")
	}
	var row models.ArchivedMessage
	if err := s.DB.WithContext(c.Context()).First(&row, id).Error; err != nil {
		return core.Fail(c, 404, err, "message not found")
	}
	name := "message-" + strconv.Itoa(int(row.ID)) + ".eml"
	c.Set("Content-Type", "message/rfc822")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", mime.QEncoding.Encode("UTF-8", name)))
	return c.Send(row.Raw)
}

// reviewMessage marks a message as reviewed (or reopens it) with a note.
func (s *Service) reviewMessage(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return core.Fail(c, 400, err, "bad id")
	}
	var body struct {
		Reviewed *bool  `json:"reviewed"`
		Note     string `json:"note"`
	}
	if err := c.BodyParser(&body); err != nil {
		return core.Fail(c, 400, err, "bad request")
	}
	var row models.ArchivedMessage
	if err := s.DB.WithContext(c.Context()).First(&row, id).Error; err != nil {
		return core.Fail(c, 404, err, "message not found")
	}
	user := core.CurrentUser(c)
	now := time.Now()
	row.ReviewNote = strings.TrimSpace(body.Note)
	if body.Reviewed != nil && *body.Reviewed {
		row.Reviewed = true
		row.ReviewedBy = user.Email
		row.ReviewedAt = &now
	} else if body.Reviewed != nil {
		row.Reviewed = false
		row.ReviewedBy = ""
		row.ReviewedAt = nil
	}
	if err := s.DB.WithContext(c.Context()).Save(&row).Error; err != nil {
		return core.Fail(c, 500, err, "save review failed")
	}
	row.Raw = nil
	return c.JSON(row)
}

// deleteMessage removes one archived message. Audited; the row is hard
// deleted per compliance-erasure semantics (the audit entry is the proof).
func (s *Service) deleteMessage(c *fiber.Ctx) error {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil {
		return core.Fail(c, 400, err, "bad id")
	}
	res := s.DB.WithContext(c.Context()).Delete(&models.ArchivedMessage{}, id)
	if res.Error != nil {
		return core.Fail(c, 500, res.Error, "delete failed")
	}
	if res.RowsAffected == 0 {
		return core.Fail(c, 404, nil, "message not found")
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// exportMessages streams every match of the current filter as an mbox file
// (bounded to avoid an unbounded response), recording the export in the
// audit trail.
func (s *Service) exportMessages(c *fiber.Ctx) error {
	q := s.DB.WithContext(c.Context()).Model(&models.ArchivedMessage{})
	if v := c.Query("direction"); v != "" {
		q = q.Where("direction = ?", v)
	}
	if v := c.Query("domain"); v != "" {
		q = q.Where("domain = ?", v)
	}
	if v := c.Query("from"); v != "" {
		q = q.Where("LOWER(`from`) LIKE ? OR LOWER(envelope_from) LIKE ?", "%"+strings.ToLower(v)+"%", "%"+strings.ToLower(v)+"%")
	}
	if v := c.Query("to"); v != "" {
		q = q.Where("LOWER(`to`) LIKE ? OR LOWER(envelope_to) LIKE ?", "%"+strings.ToLower(v)+"%", "%"+strings.ToLower(v)+"%")
	}
	if v := strings.TrimSpace(c.Query("q")); v != "" {
		like := "%" + strings.ToLower(v) + "%"
		q = q.Where("LOWER(subject) LIKE ? OR LOWER(`from`) LIKE ? OR LOWER(`to`) LIKE ? OR LOWER(message_id) LIKE ?",
			like, like, like, like)
	}
	const exportLimit = 5000
	var rows []models.ArchivedMessage
	if err := q.Order("archived_at").Limit(exportLimit).Find(&rows).Error; err != nil {
		return core.Fail(c, 500, err, "export failed")
	}
	var buf bytes.Buffer
	for i := range rows {
		fmt.Fprintf(&buf, "From %s %s\n",
			mailFromAddr(rows[i].From, rows[i].EnvelopeFrom),
			rows[i].ArchivedAt.Format(time.ANSIC))
		buf.Write(rows[i].Raw)
		if len(rows[i].Raw) == 0 || rows[i].Raw[len(rows[i].Raw)-1] != '\n' {
			buf.WriteByte('\n')
		}
		buf.WriteByte('\n')
	}
	name := fmt.Sprintf("mailez-archive-%s.mbox", time.Now().Format("20060102-150405"))
	c.Set("Content-Type", "application/mbox")
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", mime.QEncoding.Encode("UTF-8", name)))
	return c.Send(buf.Bytes())
}

func parseQueryTime(v string) (time.Time, error) {
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04"} {
		if t, err := time.Parse(layout, v); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad time %q", v)
}

func mailFromAddr(fromHeader, envelope string) string {
	if a, err := mail.ParseAddress(fromHeader); err == nil {
		return a.Address
	}
	return envelope
}

// renderPreview extracts the first text/plain (or text/html) body for the
// review pane, capped at a few KB.
func renderPreview(raw []byte) string {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	mediaType, _, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if strings.HasPrefix(mediaType, "text/") {
		b, _ := io.ReadAll(io.LimitReader(msg.Body, 8<<10))
		return string(b)
	}
	if !strings.HasPrefix(mediaType, "multipart/") {
		return ""
	}
	_, params, _ := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	mr := multipart.NewReader(msg.Body, params["boundary"])
	if mr == nil {
		return ""
	}
	for {
		p, err := mr.NextPart()
		if err != nil {
			return ""
		}
		pt, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if pt == "text/plain" || pt == "text/html" {
			b, _ := io.ReadAll(io.LimitReader(p, 8<<10))
			p.Close()
			return string(b)
		}
		p.Close()
	}
}
