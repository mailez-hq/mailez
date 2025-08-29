package stack

import (
	"github.com/gofiber/fiber/v2"
)

// registerNotify mounts the engine delivery-receipt endpoint.
//
// mailezine POSTs here the moment mail lands in a local mailbox so web push,
// webhooks and the webmail SSE stream fire immediately instead of at the
// poller's next tick. Receipts are coalesced per account on the push side;
// the endpoint itself is fire-and-forget and never blocks the delivery.
func (h *Handler) registerNotify(r fiber.Router) {
	r.Post("/notify/delivered", h.notifyDelivered)
}

// notifyDeliveredIn mirrors mailezine/internal/notify's receipt payload.
type notifyDeliveredIn struct {
	Account   string `json:"account"`
	Deliveries []struct {
		Mailbox string `json:"mailbox"`
		UID     uint32 `json:"uid"`
	} `json:"deliveries"`
}

// notifyDelivered kicks the push notifier and the SSE watcher for the
// account. The payload's delivery list is accepted for forward compatibility
// (per-folder routing) but the check itself is a cheap unseen/UIDNEXT diff.
//
// @Summary Engine delivery receipt
// @Tags notify
// @Accept json
// @Success 202
// @Router /notify/delivered [post]
func (h *Handler) notifyDelivered(c *fiber.Ctx) error {
	var in notifyDeliveredIn
	if err := c.BodyParser(&in); err != nil || in.Account == "" {
		return c.Status(400).JSON(fiber.Map{"error": "account is required"})
	}
	if h.Notifier != nil {
		h.Notifier.Kick(c.UserContext(), in.Account)
	}
	if h.EventWatcher != nil {
		h.EventWatcher.Kick(in.Account)
	}
	return c.SendStatus(fiber.StatusAccepted)
}
