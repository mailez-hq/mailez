package core

import (
	"log"

	"github.com/gofiber/fiber/v2"
)

// Fail logs the underlying error server-side and returns a safe, generic
// message to the client so internals (DB/SMTP details) are never leaked.
func Fail(c *fiber.Ctx, status int, err error, msg string) error {
	if err != nil {
		log.Printf("api error (%d %s): %v", status, c.Path(), err)
	}
	return c.Status(status).JSON(fiber.Map{"error": msg})
}
