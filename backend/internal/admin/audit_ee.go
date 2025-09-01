//go:build mailez_ee

package admin

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
)

// auditExportLimit caps a single CSV export so a huge trail cannot exhaust
// memory; callers narrow the window with from/to instead.
const auditExportLimit = 50000

// registerAuditExtra mounts the enterprise-only audit CSV export.
func (h *Handler) registerAuditExtra(r fiber.Router, mw fiber.Handler) {
	r.Get("/audit/export", mw, h.exportAudit)
}

// exportAudit streams the audit trail as CSV. Optional filters: from/to
// (RFC3339 or YYYY-MM-DD) and user.
//
// @Summary Export audit log as CSV
// @Tags admin
// @Produce text/csv
// @Param from query string false "start time (RFC3339 or YYYY-MM-DD)"
// @Param to query string false "end time (RFC3339 or YYYY-MM-DD)"
// @Param user query string false "filter by user email"
// @Success 200 {string} string
// @Failure 400 {object} models.APIError
// @Router /audit/export [get]
func (h *Handler) exportAudit(c *fiber.Ctx) error {
	from, err := parseAuditTime(c.Query("from"), false)
	if err != nil {
		return core.Fail(c, fiber.StatusBadRequest, err, "invalid from")
	}
	to, err := parseAuditTime(c.Query("to"), true)
	if err != nil {
		return core.Fail(c, fiber.StatusBadRequest, err, "invalid to")
	}
	q := h.DB.Model(&models.AuditLog{}).Order("id asc").Limit(auditExportLimit)
	if from != nil {
		q = q.Where("created_at >= ?", *from)
	}
	if to != nil {
		q = q.Where("created_at <= ?", *to)
	}
	if u := c.Query("user"); u != "" {
		q = q.Where("user = ?", u)
	}
	var entries []models.AuditLog
	if err := q.Find(&entries).Error; err != nil {
		return core.Fail(c, fiber.StatusInternalServerError, err, "internal error")
	}

	c.Set(fiber.HeaderContentType, "text/csv; charset=utf-8")
	c.Set(fiber.HeaderContentDisposition,
		fmt.Sprintf(`attachment; filename="audit-%s.csv"`, time.Now().UTC().Format("20060102-150405")))
	w := csv.NewWriter(c.Response().BodyWriter())
	_ = w.Write([]string{"id", "created_at", "user", "ip", "method", "path", "status", "action", "target", "detail"})
	for _, e := range entries {
		_ = w.Write([]string{
			strconv.FormatUint(uint64(e.ID), 10),
			e.CreatedAt.UTC().Format(time.RFC3339),
			csvSafe(e.User), csvSafe(e.IP), e.Method, csvSafe(e.Path),
			strconv.Itoa(e.Status), e.Action, csvSafe(e.Target), csvSafe(e.Detail),
		})
	}
	w.Flush()
	return w.Error()
}

// parseAuditTime accepts RFC3339 timestamps or bare dates (interpreted as
// UTC start-of-day, or end-of-day when end is set). Empty input is nil.
func parseAuditTime(v string, end bool) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t, nil
	}
	t, err := time.ParseInLocation("2006-01-02", v, time.UTC)
	if err != nil {
		return nil, fmt.Errorf("cannot parse %q as RFC3339 or YYYY-MM-DD", v)
	}
	if end {
		t = t.Add(24*time.Hour - time.Second)
	}
	return &t, nil
}

// csvSafe neutralizes spreadsheet formula injection: values starting with
// = + - @ or a tab would otherwise execute as formulas when opened in Excel.
func csvSafe(s string) string {
	if s == "" {
		return ""
	}
	switch s[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + s
	}
	return strings.ReplaceAll(s, "\r", "")
}
