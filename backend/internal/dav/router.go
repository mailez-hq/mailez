// Package dav mounts the built-in CardDAV/CalDAV servers behind Basic Auth
// (mailbox password or an app token), answers discovery on the principal
// URLs and dispatches address-book/calendar paths to the protocol backends.
package dav

import (
	"encoding/base64"
	"encoding/xml"
	"log"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/authcache"
	"mailez/backend/internal/caldav"
	"mailez/backend/internal/carddav"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/password"
	"mailez/backend/internal/webdav"
)

// Server wires the DAV endpoints.
type Server struct {
	DB   *gorm.DB
	Card *carddav.Server
	Cal  *caldav.Server
	// AuthCache memoizes successful Basic-auth credential checks so DAV
	// clients that poll every few seconds do not burn a bcrypt verification
	// on every request.
	AuthCache *authcache.Cache
}

// New assembles the DAV server stack.
func New(db *gorm.DB, cache *authcache.Cache) *Server {
	card := carddav.New(db)
	cal := caldav.New(db)
	if cache == nil {
		// Never degrade to skipping credential checks: a missing cache must
		// still verify every request, just without memoization.
		cache = authcache.New(0)
	}
	return &Server{
		DB:        db,
		Card:      card,
		Cal:       cal,
		AuthCache: cache,
	}
}

// Register mounts the DAV endpoints under the given group (typically /dav).
func (s *Server) Register(r fiber.Router) {
	r.All("/*", s.requireAuth, s.dispatch)
}

// requireAuth validates HTTP Basic credentials (mailbox password or app
// token) and stores the authenticated email for the request.
func (s *Server) requireAuth(c *fiber.Ctx) error {
	authz := c.Get("Authorization")
	if !strings.HasPrefix(authz, "Basic ") {
		c.Set("WWW-Authenticate", `Basic realm="Mailez DAV"`)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authz, "Basic "))
	if err != nil {
		c.Set("WWW-Authenticate", `Basic realm="Mailez DAV"`)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	email, pw, ok := strings.Cut(string(raw), ":")
	email = strings.ToLower(strings.TrimSpace(email))
	if !ok || email == "" {
		c.Set("WWW-Authenticate", `Basic realm="Mailez DAV"`)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	// The DB lookup stays on every request (one indexed PK read); the
	// credential verification — the expensive bcrypt or token scan — is
	// memoized per credential pair.
	var user models.User
	dbErr := s.DB.WithContext(c.Context()).First(&user, "email = ?", email).Error
	if dbErr != nil || !user.Enabled {
		log.Printf("dav auth: user lookup email=%q err=%v enabled=%v", email, dbErr, user.Enabled)
		c.Set("WWW-Authenticate", `Basic realm="Mailez DAV"`)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	if !s.AuthCache.Check(email, pw, func() bool { return s.validCredential(&user, pw) }) {
		log.Printf("dav auth: rejected email=%q", email)
		c.Set("WWW-Authenticate", `Basic realm="Mailez DAV"`)
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	c.Locals("davUser", user.Email)
	c.SetUserContext(webdav.WithUser(c.UserContext(), user.Email))
	return c.Next()
}

// validCredential accepts the mailbox password or any app token of the user.
// App-token credentials are detected by shape and checked first, so clients
// configured with an app token never pay a pointless bcrypt comparison.
func (s *Server) validCredential(u *models.User, pw string) bool {
	if auth.IsAppToken(pw) {
		return s.matchesToken(u, pw)
	}
	if password.Verify(u.Password, pw) {
		return true
	}
	// Legacy/imported tokens may not carry the 32-hex shape; keep accepting
	// any stored token after the password check.
	return s.matchesToken(u, pw)
}

// matchesToken reports whether pw matches one of the user's stored app
// tokens (PBKDF2-SHA256, so no bcrypt cost on this path).
func (s *Server) matchesToken(u *models.User, pw string) bool {
	var tokens []models.Token
	if err := s.DB.Where("user_email = ?", u.Email).Find(&tokens).Error; err != nil {
		return false
	}
	for _, t := range tokens {
		if password.VerifyPBKDF2SHA256(t.Password, pw) {
			return true
		}
	}
	return false
}

// dispatch routes the request to the principal handler or the matching
// protocol backend.
func (s *Server) dispatch(c *fiber.Ctx) error {
	path := c.Path()
	switch {
	case path == "/dav" || path == "/dav/":
		return s.serveRoot(c)
	case strings.HasPrefix(path, "/dav/principals/"):
		return s.servePrincipal(c, path)
	case strings.HasPrefix(path, "/dav/addressbooks"):
		if !s.pathUserAllowed(c, path, "/dav/addressbooks/") {
			return c.SendStatus(fiber.StatusForbidden)
		}
		// A per-request server instance: swapping Backend on a shared one
		// raced between concurrent addressbook/calendar requests.
		handler := &webdav.Server{Capabilities: []string{"addressbook"}, Backend: s.Card}
		return handler.Handle(c)
	case strings.HasPrefix(path, "/dav/calendars"):
		if !s.pathUserAllowed(c, path, "/dav/calendars/") {
			return c.SendStatus(fiber.StatusForbidden)
		}
		handler := &webdav.Server{Capabilities: []string{"calendar-access"}, Backend: s.Cal}
		return handler.Handle(c)
	}
	return c.SendStatus(fiber.StatusNotFound)
}

// pathUserAllowed enforces that the {user} segment of a DAV collection path
// names the authenticated user. The protocol backends scope every query by
// that segment, so without this check any valid DAV credential (mailbox
// password or app token) could read, overwrite or delete another account's
// address books and calendars (cross-account IDOR).
func (s *Server) pathUserAllowed(c *fiber.Ctx, path, prefix string) bool {
	rest := strings.TrimPrefix(path, prefix)
	seg := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		seg = rest[:i]
	}
	seg = strings.TrimSuffix(seg, "/")
	if decoded, err := url.PathUnescape(seg); err == nil {
		seg = decoded
	}
	return seg != "" && strings.EqualFold(seg, webdav.UserFrom(c.UserContext()))
}

// serveRoot answers discovery PROPFIND on /dav/ with the current user's
// principal and both home sets.
func (s *Server) serveRoot(c *fiber.Ctx) error {
	switch c.Method() {
	case fiber.MethodOptions:
		c.Set("DAV", "1, 3, addressbook, calendar-access")
		c.Set("Allow", "OPTIONS, PROPFIND")
		return c.SendStatus(fiber.StatusNoContent)
	case "PROPFIND":
		user := webdav.UserFrom(c.UserContext())
		principal := principalURL(user)
		props := []webdav.Prop{
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "resourcetype"}, Value: webdav.Empty(xml.Name{Space: webdav.NSDAV, Local: "collection"})},
			{XMLName: xml.Name{Space: webdav.NSDAV, Local: "current-user-principal"}, Value: webdav.Href(principal)},
			{XMLName: xml.Name{Space: webdav.NSCardDAV, Local: "addressbook-home-set"}, Value: webdav.Href("/dav/addressbooks/" + url.PathEscape(user) + "/")},
			{XMLName: xml.Name{Space: webdav.NSCalDAV, Local: "calendar-home-set"}, Value: webdav.Href("/dav/calendars/" + url.PathEscape(user) + "/")},
		}
		return c.Status(fiber.StatusMultiStatus).Type("application/xml; charset=utf-8").
			Send(webdav.RenderMultiStatus([]webdav.Response{{Href: "/dav/", Props: props}}))
	default:
		return c.SendStatus(fiber.StatusMethodNotAllowed)
	}
}

// servePrincipal answers PROPFIND on a principal URL, enforcing that the URL
// names the authenticated user.
func (s *Server) servePrincipal(c *fiber.Ctx, path string) error {
	user := strings.TrimPrefix(path, "/dav/principals/")
	user = strings.TrimSuffix(user, "/")
	decoded, err := url.PathUnescape(user)
	if err == nil {
		user = decoded
	}
	authUser := webdav.UserFrom(c.UserContext())
	if !strings.EqualFold(user, authUser) {
		return c.SendStatus(fiber.StatusForbidden)
	}
	return webdav.ServePrincipal(c, principalURL(authUser),
		"/dav/addressbooks/"+url.PathEscape(authUser)+"/",
		"/dav/calendars/"+url.PathEscape(authUser)+"/")
}

func principalURL(user string) string {
	return "/dav/principals/" + url.PathEscape(user) + "/"
}
