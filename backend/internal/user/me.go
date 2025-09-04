package user

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
)

func (h *Handler) registerMe(r fiber.Router) {
	r.Get("/me", h.meProfile)
	r.Get("/me/logins", h.meRecentLogins)
	r.Put("/me/settings", h.meUpdateSettings)
	r.Put("/me/password", h.meChangePassword)
}

// meProfile returns the current user's full profile (password hash excluded).
// meProfile returns the full profile/settings of the current user.
// @Summary Current user settings
// @Tags me
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /me [get]
func (h *Handler) meProfile(c *fiber.Ctx) error {
	return c.JSON(currentUser(c))
}

// meSettingsIn are the fields a user may manage for themselves.
type meSettingsIn struct {
	DisplayedName      *string `json:"displayed_name"`
	ForwardEnabled     *bool   `json:"forward_enabled"`
	ForwardDestination *string `json:"forward_destination"`
	ForwardKeep        *bool   `json:"forward_keep"`
	ReplyEnabled       *bool   `json:"reply_enabled"`
	ReplySubject       *string `json:"reply_subject"`
	ReplyBody          *string `json:"reply_body"`
	ReplyStartdate     *string `json:"reply_startdate"`
	ReplyEnddate       *string `json:"reply_enddate"`
	SpamEnabled        *bool   `json:"spam_enabled"`
	SpamMarkAsRead     *bool   `json:"spam_mark_as_read"`
	SpamThreshold      *int    `json:"spam_threshold"`
	Whitelist          *string `json:"whitelist"`
	Blacklist          *string `json:"blacklist"`
	Signature          *string `json:"signature"`
}

// meUpdateSettings applies self-service settings to the current user.
// meUpdateSettings updates the current user's self-service settings.
// @Summary Update settings
// @Tags me
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /me/settings [put]
func (h *Handler) meUpdateSettings(c *fiber.Ctx) error {
	u := currentUser(c)
	var in meSettingsIn
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.DisplayedName != nil {
		u.DisplayedName = *in.DisplayedName
	}
	if in.ForwardEnabled != nil {
		u.ForwardEnabled = *in.ForwardEnabled
	}
	if in.ForwardDestination != nil {
		u.ForwardDestination = *in.ForwardDestination
	}
	if in.ForwardKeep != nil {
		u.ForwardKeep = *in.ForwardKeep
	}
	if in.ReplyEnabled != nil {
		u.ReplyEnabled = *in.ReplyEnabled
	}
	if in.ReplySubject != nil {
		u.ReplySubject = *in.ReplySubject
	}
	if in.ReplyBody != nil {
		u.ReplyBody = *in.ReplyBody
	}
	if in.ReplyStartdate != nil {
		u.ReplyStartdate = parseUserDate(*in.ReplyStartdate)
	}
	if in.ReplyEnddate != nil {
		u.ReplyEnddate = parseUserDate(*in.ReplyEnddate)
	}
	if in.SpamEnabled != nil {
		u.SpamEnabled = *in.SpamEnabled
	}
	if in.SpamMarkAsRead != nil {
		u.SpamMarkAsRead = *in.SpamMarkAsRead
	}
	if in.SpamThreshold != nil {
		u.SpamThreshold = *in.SpamThreshold
	}
	if in.Signature != nil {
		u.Signature = *in.Signature
	}
	if in.Whitelist != nil {
		u.Whitelist = *in.Whitelist
	}
	if in.Blacklist != nil {
		u.Blacklist = *in.Blacklist
	}
	if err := h.DB.Save(u).Error; err != nil {
		return core.Fail(c, 400, err, "update failed")
	}
	return c.JSON(u)
}

// meChangePassword updates the current user's password after verifying the old
// one, so a stolen session cannot silently change credentials.
// meChangePassword changes the current user's password.
// @Summary Change password
// @Tags me
// @Accept json
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Router /me/password [put]
func (h *Handler) meChangePassword(c *fiber.Ctx) error {
	u := currentUser(c)
	var in struct {
		OldPassword string `json:"old_pw"`
		NewPassword string `json:"new_pw"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.NewPassword == "" {
		return c.Status(400).JSON(fiber.Map{"error": "new password is required"})
	}
	if !password.Verify(u.Password, in.OldPassword) {
		return c.Status(400).JSON(fiber.Map{"error": "old password is incorrect"})
	}
	hash, err := password.Hash(in.NewPassword)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	if err := h.DB.Model(u).Updates(map[string]any{
		"password":            hash,
		"password_changed_at": time.Now(),
	}).Error; err != nil {
		return core.Fail(c, 400, err, "update failed")
	}
	return c.SendStatus(204)
}

// meRecentLogins returns the latest successful password sign-ins of the
// current user (time + IP), read from the audit trail written by the SSO
// login endpoints. The workspace home shows them as "recent logins".
func (h *Handler) meRecentLogins(c *fiber.Ctx) error {
	u := currentUser(c)
	rows := make([]meLoginRow, 0, 8)
	err := h.DB.Model(&models.AuditLog{}).
		Select("created_at AS time, ip").
		// "user" is a reserved word on PostgreSQL — a bare reference parses
		// as the current_user special and silently matches nothing. Qualify
		// with the table name (portable across MySQL/SQLite/Postgres).
		Where("audit_log.user = ? AND path IN ? AND status = ?", u.Email,
			[]string{"/sso/login", "/sso/login/totp"}, fiber.StatusOK).
		Order("created_at DESC").
		Limit(8).
		Scan(&rows).Error
	if err != nil {
		return core.Fail(c, 500, err, "query failed")
	}
	return c.JSON(rows)
}

// meLoginRow is one recent sign-in: when and from which IP.
type meLoginRow struct {
	Time time.Time `json:"time"`
	IP   string    `json:"ip"`
}
