package core

import (
	"log"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// FailWithCode logs the underlying error server-side and returns a safe,
// generic message to the client so internals are never leaked, optionally
// carrying a machine-readable code.
func FailWithCode(c *fiber.Ctx, status int, err error, msg, code string) error {
	if err != nil {
		log.Printf("api error (%d %s): %v", status, c.Path(), err)
	}
	return c.Status(status).JSON(models.APIError{Error: msg, Code: code})
}

// Fail is FailWithCode without a machine-readable code.
func Fail(c *fiber.Ctx, status int, err error, msg string) error {
	return FailWithCode(c, status, err, msg, "")
}
