package auth

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/totp"
)

// RegisterSSO mounts the user-facing session endpoints.
func (m *Manager) RegisterSSO(r fiber.Router) {
	r.Post("/sso/login", m.ssoLogin)
	r.Post("/sso/login/totp", m.ssoLoginTotp)
	r.Post("/sso/logout", m.ssoLogout)
	r.Get("/sso/me", m.ssoMe)
	m.RegisterPasskey(r)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"pw"`
}

// ssoLogin authenticates with email/password and returns a session cookie.
// @Summary Log in
// @Tags auth
// @Accept json
// @Produce json
// @Param credentials body loginRequest true "email + password"
// @Success 200 {object} map[string]string "email"
// @Failure 400 {object} models.APIError
// @Failure 401 {object} models.APIError
// @Failure 429 {object} models.APIError "rate_limited"
// @Router /sso/login [post]
func (m *Manager) ssoLogin(c *fiber.Ctx) error {
	var req loginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request"})
	}
	// Normalize the identifier so case/whitespace never breaks an exact-match
	// lookup (the directory fallback below already lowercases).
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if !m.checkLoginAttempt(c.Context(), c.IP()) {
		return c.Status(fiber.StatusTooManyRequests).JSON(models.APIError{Error: "too many login attempts, try again later", Code: "rate_limited"})
	}
	sid, user, err := m.Login(c.Context(), req.Email, req.Password)
	// Directory fallback: when local credentials fail and an LDAP
	// integration is configured, bind against the directory and provision
	// the local account on first successful login.
	if (err != nil || user == nil) && m.LDAP != nil && !strings.HasPrefix(req.Password, "token-") {
		if ok, lerr := m.LDAP.Authenticate(c.Context(), req.Email, req.Password); lerr == nil && ok {
			if uerr := m.LDAP.EnsureLocalUser(c.Context(), req.Email); uerr == nil {
				var lu models.User
				if derr := m.DB.WithContext(c.Context()).First(&lu, "email = ?", strings.ToLower(strings.TrimSpace(req.Email))).Error; derr == nil && lu.Enabled {
					user = &lu
					sid, err = m.CreateSession(c.Context(), lu.Email)
				}
			}
		}
	}
	if err != nil {
		if m.loginFailed(c.Context(), req.Email) {
			// Per-email lockout: the failure limit is already exceeded, so
			// burn no bcrypt on further attempts from any IP.
			return c.Status(fiber.StatusTooManyRequests).JSON(models.APIError{Error: "too many failed logins for this account, try again later", Code: "rate_limited"})
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "wrong e-mail or password"})
	}
	if user == nil {
		if m.loginFailed(c.Context(), req.Email) {
			return c.Status(fiber.StatusTooManyRequests).JSON(models.APIError{Error: "too many failed logins for this account, try again later", Code: "rate_limited"})
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "wrong e-mail or password"})
	}
	m.loginSucceeded(c.Context(), user.Email)
	// Security alert: a login from an unrecognized IP raises an email so the
	// account owner can spot unauthorized access early.
	if m.NotifyLogin != nil && m.rememberLoginIP(c.Context(), user.Email, c.IP()) {
		m.NotifyLogin(user.Email, c.IP(), c.Get("User-Agent"))
	}
	// A 2FA user must complete the second factor before a session is issued.
	if user.TOTPEnabled {
		pending, err := m.CreatePending2FA(c.Context(), user.Email)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "2fa setup failed"})
		}
		return c.JSON(fiber.Map{"totp_required": true, "pending_token": pending})
	}
	setSessionCookie(c, m.SessionName, sid, m.SessionTTL, m.secureCookie)
	// record the login in the audit trail
	m.DB.Create(&models.AuditLog{User: user.Email, IP: c.IP(), Method: "POST", Path: "/sso/login", Status: fiber.StatusOK})
	return c.JSON(fiber.Map{"email": user.Email})
}

// ssoLoginTotp completes a login after the password step by verifying the
// authenticator code and only then creating the real session.
// ssoLoginTotp completes a 2FA login with the pending token and TOTP code.
// @Summary Complete 2FA login
// @Tags auth
// @Accept json
// @Produce json
// @Param body body object true "pending_token + code"
// @Success 200 {object} map[string]string "email"
// @Failure 401 {object} models.APIError
// @Failure 429 {object} models.APIError "rate_limited"
// @Router /sso/login/totp [post]
func (m *Manager) ssoLoginTotp(c *fiber.Ctx) error {
	var in struct {
		PendingToken string `json:"pending_token"`
		Code         string `json:"code"`
	}
	if err := c.BodyParser(&in); err != nil || in.PendingToken == "" || in.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "pending_token and code are required"})
	}
	if !m.checkLoginAttempt(c.Context(), c.IP()) {
		return c.Status(fiber.StatusTooManyRequests).JSON(models.APIError{Error: "too many login attempts, try again later", Code: "rate_limited"})
	}
	email, ok := m.PeekPending2FA(c.Context(), in.PendingToken)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "2fa session expired"})
	}
	var user models.User
	if err := m.DB.WithContext(c.Context()).First(&user, "email = ?", email).Error; err != nil || !user.TOTPEnabled {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "2fa not configured"})
	}
	if !totp.Valid(user.TOTPSecret, in.Code, time.Now()) {
		burned := m.FailTotpAttempt(c.Context(), in.PendingToken)
		if burned {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "2fa session expired"})
		}
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid verification code"})
	}
	if _, ok := m.ConsumePending2FA(c.Context(), in.PendingToken); !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "2fa session expired"})
	}
	_ = m.Store.Delete(c.Context(), totpFailPrefix+in.PendingToken)
	sid, err := m.CreateSession(c.Context(), user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "session failed"})
	}
	setSessionCookie(c, m.SessionName, sid, m.SessionTTL, m.secureCookie)
	m.DB.Create(&models.AuditLog{User: user.Email, IP: c.IP(), Method: "POST", Path: "/sso/login/totp", Status: fiber.StatusOK, Action: "login.totp"})
	return c.JSON(fiber.Map{"email": user.Email})
}

// ssoLogout invalidates the session cookie.
// @Summary Log out
// @Tags auth
// @Success 204
// @Router /sso/logout [post]
func (m *Manager) ssoLogout(c *fiber.Ctx) error {
	sid := c.Cookies(m.SessionName)
	if sid != "" {
		_ = m.Logout(c.Context(), sid)
	}
	clearSessionCookie(c, m.SessionName)
	return c.SendStatus(fiber.StatusNoContent)
}

// MeResponse is the authenticated user profile returned by /sso/me.
type MeResponse struct {
	Email         string `json:"email"`
	DisplayedName string `json:"displayed_name"`
	GlobalAdmin   bool   `json:"global_admin"`
	Manager       bool   `json:"manager"`
	Enabled       bool   `json:"enabled"`
}

// ssoMe returns the current session's profile.
// @Summary Current profile
// @Tags auth
// @Produce json
// @Success 200 {object} MeResponse
// @Failure 401 {object} models.APIError
// @Router /sso/me [get]
func (m *Manager) ssoMe(c *fiber.Ctx) error {
	sid := c.Cookies(m.SessionName)
	user, err := m.UserFromSession(c.Context(), sid)
	if err != nil || user == nil {
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	manager := user.GlobalAdmin
	if !manager {
		var count int64
		if err := m.DB.WithContext(c.Context()).Table("manager").Where("user_email = ?", user.Email).Count(&count).Error; err == nil {
			manager = count > 0
		}
	}
	return c.JSON(MeResponse{
		Email:         user.Email,
		DisplayedName: user.DisplayedName,
		GlobalAdmin:   user.GlobalAdmin,
		Manager:       manager,
		Enabled:       user.Enabled,
	})
}

func setSessionCookie(c *fiber.Ctx, name, sid string, ttl time.Duration, secure bool) {
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    sid,
		Path:     "/",
		HTTPOnly: true,
		Secure:   secure,
		SameSite: "Lax",
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearSessionCookie(c *fiber.Ctx, name string) {
	c.Cookie(&fiber.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HTTPOnly: true,
		MaxAge:   -1,
	})
}
