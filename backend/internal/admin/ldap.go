package admin

import (
	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
)

func (h *Handler) registerLDAP(r fiber.Router, mw fiber.Handler) {
	r.Get("/ldap", mw, h.getLDAPConfig)
	r.Put("/ldap", mw, h.putLDAPConfig)
	r.Post("/ldap/test", mw, h.testLDAP)
	r.Post("/ldap/sync", mw, h.syncLDAPContacts)
}

// ldapConfigView is the API shape for the admin UI; the bind password is
// never returned.
type ldapConfigView struct {
	Enabled     bool   `json:"enabled"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Security    string `json:"security"`
	BaseDN      string `json:"base_dn"`
	BindDN      string `json:"bind_dn"`
	HasBindPw   bool   `json:"has_bind_pw,omitempty"`
	UserFilter  string `json:"user_filter"`
	MailAttr    string `json:"mail_attr"`
	UIDAttr     string `json:"uid_attr"`
	NameAttr    string `json:"name_attr"`
	DeptAttr    string `json:"dept_attr"`
	TitleAttr   string `json:"title_attr"`
	PhoneAttr   string `json:"phone_attr"`
	AutoCreate  bool   `json:"auto_create"`
	SyncMinutes int    `json:"sync_minutes"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

func viewLDAP(row *models.LdapConfig) ldapConfigView {
	out := ldapConfigView{
		Enabled:     row.Enabled,
		Host:        row.Host,
		Port:        row.Port,
		Security:    row.Security,
		BaseDN:      row.BaseDN,
		BindDN:      row.BindDN,
		HasBindPw:   row.BindPasswordEnc != "",
		UserFilter:  row.UserFilter,
		MailAttr:    row.MailAttr,
		UIDAttr:     row.UIDAttr,
		NameAttr:    row.NameAttr,
		DeptAttr:    row.DeptAttr,
		TitleAttr:   row.TitleAttr,
		PhoneAttr:   row.PhoneAttr,
		AutoCreate:  row.AutoCreate,
		SyncMinutes: row.SyncMinutes,
	}
	if !row.UpdatedAt.IsZero() {
		out.UpdatedAt = row.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return out
}

// getLDAPConfig returns the current directory settings.
// @Summary Get LDAP configuration
// @Tags admin
// @Success 200 {object} ldapConfigView
// @Router /ldap [get]
func (h *Handler) getLDAPConfig(c *fiber.Ctx) error {
	var row models.LdapConfig
	if err := h.DB.First(&row).Error; err != nil {
		return c.JSON(ldapConfigView{Port: 389, Security: "none", UserFilter: "(objectClass=person)", MailAttr: "mail", UIDAttr: "uid", NameAttr: "displayName", AutoCreate: true, SyncMinutes: 60})
	}
	return c.JSON(viewLDAP(&row))
}

// putLDAPConfig saves the directory settings. An empty bind_password keeps
// the stored one; a non-empty value rotates it.
// @Summary Update LDAP configuration
// @Tags admin
// @Accept json
// @Success 200 {object} ldapConfigView
// @Router /ldap [put]
func (h *Handler) putLDAPConfig(c *fiber.Ctx) error {
	var in struct {
		Enabled      *bool  `json:"enabled"`
		Host         string `json:"host"`
		Port         int    `json:"port"`
		Security     string `json:"security"`
		BaseDN       string `json:"base_dn"`
		BindDN       string `json:"bind_dn"`
		BindPassword string `json:"bind_password"`
		UserFilter   string `json:"user_filter"`
		MailAttr     string `json:"mail_attr"`
		UIDAttr      string `json:"uid_attr"`
		NameAttr     string `json:"name_attr"`
		DeptAttr     string `json:"dept_attr"`
		TitleAttr    string `json:"title_attr"`
		PhoneAttr    string `json:"phone_attr"`
		AutoCreate   *bool  `json:"auto_create"`
		SyncMinutes  int    `json:"sync_minutes"`
	}
	if err := c.BodyParser(&in); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request"})
	}
	if in.Host == "" || in.BaseDN == "" {
		return c.Status(400).JSON(fiber.Map{"error": "host and base_dn are required"})
	}
	if in.Port <= 0 || in.Port > 65535 {
		in.Port = 389
	}
	if in.Security != "none" && in.Security != "starttls" && in.Security != "tls" {
		in.Security = "none"
	}
	if in.SyncMinutes <= 0 {
		in.SyncMinutes = 60
	}
	def := func(v, d string) string {
		if v == "" {
			return d
		}
		return v
	}

	var row models.LdapConfig
	if err := h.DB.First(&row).Error; err != nil {
		row = models.LdapConfig{ID: 1}
	}
	if in.Enabled != nil {
		row.Enabled = *in.Enabled
	}
	row.Host = in.Host
	row.Port = in.Port
	row.Security = in.Security
	row.BaseDN = in.BaseDN
	row.BindDN = in.BindDN
	row.UserFilter = def(in.UserFilter, "(objectClass=person)")
	row.MailAttr = def(in.MailAttr, "mail")
	row.UIDAttr = def(in.UIDAttr, "uid")
	row.NameAttr = def(in.NameAttr, "displayName")
	row.DeptAttr = def(in.DeptAttr, "department")
	row.TitleAttr = def(in.TitleAttr, "title")
	row.PhoneAttr = def(in.PhoneAttr, "telephoneNumber")
	if in.AutoCreate != nil {
		row.AutoCreate = *in.AutoCreate
	}
	row.SyncMinutes = in.SyncMinutes
	if in.BindPassword != "" {
		enc, err := crypto.Encrypt(h.Cfg.SecretKey, in.BindPassword)
		if err != nil {
			return core.Fail(c, 500, err, "encryption failed")
		}
		row.BindPasswordEnc = enc
	}
	if err := h.DB.Save(&row).Error; err != nil {
		return core.Fail(c, 500, err, "save failed")
	}
	return c.JSON(viewLDAP(&row))
}

// testLDAP validates the connection with the submitted (or stored) settings.
// @Summary Test LDAP connection
// @Tags admin
// @Accept json
// @Success 200 {object} map[string]interface{}
// @Router /ldap/test [post]
func (h *Handler) testLDAP(c *fiber.Ctx) error {
	var in struct {
		Host         string `json:"host"`
		Port         int    `json:"port"`
		Security     string `json:"security"`
		BaseDN       string `json:"base_dn"`
		BindDN       string `json:"bind_dn"`
		BindPassword string `json:"bind_password"`
		UserFilter   string `json:"user_filter"`
	}
	if err := c.BodyParser(&in); err != nil || in.Host == "" || in.BaseDN == "" {
		return c.Status(400).JSON(fiber.Map{"error": "host and base_dn are required"})
	}
	if in.Port <= 0 {
		in.Port = 389
	}
	cfg := models.LdapConfig{
		Host:       in.Host,
		Port:       in.Port,
		Security:   in.Security,
		BaseDN:     in.BaseDN,
		BindDN:     in.BindDN,
		UserFilter: in.UserFilter,
	}
	if cfg.UserFilter == "" {
		cfg.UserFilter = "(objectClass=person)"
	}
	// Empty submitted password falls back to the stored bind password.
	pw := in.BindPassword
	if pw == "" {
		var row models.LdapConfig
		if err := h.DB.First(&row).Error; err == nil {
			pw, _ = crypto.Decrypt(h.Cfg.SecretKey, row.BindPasswordEnc)
		}
	}
	if err := h.App.LDAP.TestConnection(c.Context(), cfg, pw); err != nil {
		return c.Status(502).JSON(fiber.Map{"error": "connection failed: " + err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true})
}

// syncLDAPContacts runs the organization address book sync immediately.
// @Summary Sync LDAP contacts
// @Tags admin
// @Success 200 {object} map[string]interface{}
// @Router /ldap/sync [post]
func (h *Handler) syncLDAPContacts(c *fiber.Ctx) error {
	added, updated, err := h.App.LDAP.SyncContacts(c.Context())
	if err != nil {
		return core.Fail(c, 502, err, "sync failed")
	}
	return c.JSON(fiber.Map{"added": added, "updated": updated})
}
