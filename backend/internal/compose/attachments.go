package compose

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/mail"
)

// attachmentBytes is the size a single attachment occupies once decoded.
// Attachment.Data is standard base64 (mail.writeAttachmentPart), so the
// encoded length is what the caller can actually inflate; a declared Size is
// taken into account as well so an under-reported field cannot slip past.
func attachmentBytes(a mail.Attachment) int {
	n := a.Size
	if n < 0 {
		n = 0
	}
	if trimmed := strings.TrimSpace(a.Data); trimmed != "" {
		if decoded := base64.StdEncoding.DecodedLen(len(trimmed)); decoded > n {
			n = decoded
		}
	}
	return n
}

// oversizedAttachment returns the first attachment above max (0 = no limit).
func oversizedAttachment(atts []mail.Attachment, max int) (mail.Attachment, int, bool) {
	if max <= 0 {
		return mail.Attachment{}, 0, false
	}
	for _, a := range atts {
		if n := attachmentBytes(a); n > max {
			return a, n, true
		}
	}
	return mail.Attachment{}, 0, false
}

// attachmentRefusal answers an oversized attachment. 422 rather than 413: the
// request was understood, the message is simply not acceptable as sent.
func attachmentRefusal(c *fiber.Ctx, a mail.Attachment, size, max int) error {
	name := a.Filename
	if name == "" {
		name = "attachment"
	}
	return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
		"error": fmt.Sprintf("%s is %s, over the %s attachment limit", name, humanBytes(size), humanBytes(max)),
		"code":  "attachment_too_large",
		"limit": max,
	})
}

// humanBytes formats a byte count the way the error message wants it (MB to
// one decimal, KB as whole numbers).
func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
