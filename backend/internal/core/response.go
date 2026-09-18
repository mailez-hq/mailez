package core

import (
	"errors"
	"io"
	"log"
	"net"
	"syscall"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
)

// FailWithCode logs the underlying error server-side and returns a safe,
// generic message to the client so internals are never leaked, optionally
// carrying a machine-readable code.
//
// A 502 whose cause is an unreachable upstream is answered as 503 with a
// retryable message instead of the generic gateway error.
func FailWithCode(c *fiber.Ctx, status int, err error, msg, code string) error {
	if err != nil {
		log.Printf("api error (%d %s): %v", status, c.Path(), err)
	}
	if status == fiber.StatusBadGateway && transportFailure(err) {
		return c.Status(fiber.StatusServiceUnavailable).JSON(models.APIError{
			Error: "the mail service is temporarily unavailable; try again in a moment",
			Code:  "mail_service_unavailable",
		})
	}
	return c.Status(status).JSON(models.APIError{Error: msg, Code: code})
}

// transportFailure reports that a request never reached a mail service: the
// engine is down, refused the connection, or the dial timed out.
func transportFailure(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	return errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

// Fail is FailWithCode without a machine-readable code.
func Fail(c *fiber.Ctx, status int, err error, msg string) error {
	return FailWithCode(c, status, err, msg, "")
}
