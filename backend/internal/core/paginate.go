package core

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// PageParams reads the page/limit query params with sane defaults and clamps
// them to valid ranges. page is 1-based.
func PageParams(c *fiber.Ctx) (page, limit int) {
	page, _ = strconv.Atoi(c.Query("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ = strconv.Atoi(c.Query("limit", strconv.Itoa(defaultPageSize)))
	if limit < 1 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	return
}

// Page wraps a result set in a uniform pagination envelope so list endpoints
// share a single wire shape.
func Page(c *fiber.Ctx, data any, total, page, limit int) error {
	return c.JSON(fiber.Map{
		"data":  data,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}
