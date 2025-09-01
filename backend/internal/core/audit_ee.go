//go:build mailez_ee

package core

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// auditEnrich is the enterprise half of the audit seam: it derives a semantic
// action/target/detail triple from the request so the audit trail can be
// filtered and reviewed without reverse-engineering raw paths.
func auditEnrich(c *fiber.Ctx, entry *models.AuditLog) {
	action, target, detail := classifyAudit(c.Method(), c.Path(), string(c.Request().URI().QueryString()))
	entry.Action, entry.Target, entry.Detail = action, target, detail
}

// classifyAudit maps a management write (method + path + query) onto
// "<noun>.<verb|sub-action>" plus the acted-on target. It is a deterministic
// heuristic — the raw method/path columns always keep ground truth.
//
//	POST /api/v1/users                -> users.create
//	PUT  /api/v1/users/bob@x.com      -> users.update   bob@x.com
//	DELETE /api/v1/tokens/42          -> tokens.delete  42
//	POST /api/v1/domains/d.io/dkim    -> domains.dkim   d.io
func classifyAudit(method, rawPath, query string) (action, target, detail string) {
	if !strings.HasPrefix(rawPath, "/api/v1/") {
		return "", "", ""
	}
	p := strings.Trim(strings.TrimPrefix(rawPath, "/api/v1/"), "/")
	if p == "" {
		return "", "", ""
	}
	segs := strings.Split(p, "/")
	for i, s := range segs {
		if dec, err := url.PathUnescape(s); err == nil {
			segs[i] = dec
		}
	}
	noun, rest := segs[0], segs[1:]
	last := len(rest) - 1
	switch {
	case last < 0: // collection write: POST /users
		return noun + "." + auditVerb(method), "", bound(query, 512)
	case isNumericID(rest[last]): // row-id write: DELETE /tokens/42
		return noun + "." + auditVerb(method), rest[last], bound(joinDetail(subPath(rest[last+1:]), query), 512)
	case last == 0: // item write: PUT /users/bob@x.com
		return noun + "." + auditVerb(method), rest[0], bound(query, 512)
	default: // sub-resource action: POST /domains/d.io/dkim
		return noun + "." + rest[last], rest[0], bound(joinDetail(subPath(rest[last+1:]), query), 512)
	}
}

// auditVerb translates the HTTP verb into an audit-friendly action suffix.
func auditVerb(method string) string {
	switch method {
	case fiber.MethodPost:
		return "create"
	case fiber.MethodPut, fiber.MethodPatch:
		return "update"
	case fiber.MethodDelete:
		return "delete"
	default:
		return strings.ToLower(method)
	}
}

// isNumericID reports whether the segment is a bare numeric row id.
func isNumericID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// subPath renders path segments deeper than the acted-on sub-action so no
// routing information is silently dropped (e.g. POST /domains/d.io/dkim/x ->
// "x").
func subPath(rest []string) string {
	return strings.Join(rest, "/")
}

// joinDetail appends the query string to the derived detail, bounded.
func joinDetail(detail, query string) string {
	if detail == "" {
		return query
	}
	return detail + "?" + query
}

// bound truncates s to at most n bytes so oversized queries cannot overflow
// the audit column.
func bound(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
