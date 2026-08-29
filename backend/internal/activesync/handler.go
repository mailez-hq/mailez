// Package activesync implements the server side of Exchange ActiveSync
// (EAS): WBXML transport, device registry, folder hierarchy and incremental
// mail sync over the existing IMAP gateway. Mobile clients can configure
// the account with the mailbox password or an app token.
package activesync

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/authcache"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
	"mailez/backend/internal/password"
)

// Service is the ActiveSync server.
type Service struct {
	DB   *gorm.DB
	Auth *auth.Manager
	Cfg  core.Config
	Mail mail.Gateway
	// AuthCache memoizes successful Basic-auth credential checks; ActiveSync
	// clients (and the Ping long-poll) re-authenticate on every request.
	AuthCache *authcache.Cache
}

// New assembles the ActiveSync service.
func New(db *gorm.DB, authMgr *auth.Manager, cfg core.Config, gw mail.Gateway, cache *authcache.Cache) *Service {
	if cache == nil {
		cache = authcache.New(0)
	}
	return &Service{DB: db, Auth: authMgr, Cfg: cfg, Mail: gw, AuthCache: cache}
}

// Register mounts the EAS endpoints. The command handler is registered for
// all HTTP verbs because EAS clients use GET (autodiscover-style blob
// requests) and POST interchangeably.
func (s *Service) Register(app *fiber.App) {
	app.All("/Microsoft-Server-ActiveSync", s.handle)
	app.All("/autodiscover/autodiscover.xml", s.handleAutodiscover)
}

// supportedVersions are the protocol versions we advertise and accept.
const supportedVersions = "12.1,14.0,14.1"

var supportedCommands = strings.Join([]string{
	"Sync", "SendMail", "SmartForward", "SmartReply",
	"FolderSync", "FolderCreate", "FolderDelete", "FolderUpdate",
	"MoveItems", "GetItemEstimate", "MeetingResponse", "Search",
	"Settings", "Ping", "ItemOperations", "Provision", "ResolveRecipients",
}, ",")

type easRequest struct {
	command     string
	protocolVer byte
	deviceID    string
	deviceType  string
	policyKey   string
	user        string
}

var commandByCode = map[byte]string{
	0: "Sync", 1: "SendMail", 2: "SmartForward", 3: "SmartReply",
	4: "GetAttachment", 9: "FolderSync", 10: "FolderCreate", 11: "FolderDelete",
	12: "FolderUpdate", 13: "MoveItems", 14: "GetItemEstimate",
	15: "MeetingResponse", 16: "Search", 17: "Settings", 18: "Ping",
	19: "ItemOperations", 20: "Provision", 21: "ResolveRecipients", 22: "ValidateCert",
}

// handle is the single entry point for /Microsoft-Server-ActiveSync.
func (s *Service) handle(c *fiber.Ctx) error {
	if c.Method() == http.MethodOptions {
		return s.handleOptions(c)
	}
	req, err := parseRequest(c)
	if err != nil {
		log.Printf("activesync: bad request: %v", err)
		return c.SendStatus(http.StatusBadRequest)
	}
	user, credential, err := s.authenticate(c, req)
	if err != nil {
		log.Printf("activesync: auth failed for %q: %v", req.user, err)
		c.Set("WWW-Authenticate", `Basic realm="Mailez ActiveSync"`)
		return c.SendStatus(http.StatusUnauthorized)
	}
	if req.deviceID != "" {
		if err := s.registerDevice(c.Context(), user.Email, req.deviceID, req.deviceType,
			versionString(req.protocolVer), c.Get("User-Agent"), req.policyKey); err != nil {
			log.Printf("activesync: device register: %v", err)
		}
	}
	return s.dispatch(c, req, user, credential)
}

func (s *Service) handleOptions(c *fiber.Ctx) error {
	c.Set("MS-Server-ActiveSync", "1")
	c.Set("MS-ASProtocolCommands", supportedCommands)
	c.Set("MS-ASProtocolVersions", supportedVersions)
	c.Set("Public", "OPTIONS,POST")
	c.Set("Allow", "OPTIONS,POST")
	c.Set("Content-Length", "0")
	return c.SendStatus(http.StatusOK)
}

// parseRequest extracts the command/device envelope from either the
// query-parameter form (Cmd=Sync&DeviceId=...) or the base64 blob form used
// by iOS.
func parseRequest(c *fiber.Ctx) (*easRequest, error) {
	queries := c.Queries()
	if cmd := queries["Cmd"]; cmd != "" {
		return &easRequest{
			command:     cmd,
			protocolVer: parseVersion(c.Get("MS-ASProtocolVersion")),
			deviceID:    queries["DeviceId"],
			deviceType:  queries["DeviceType"],
			policyKey:   queries["PolicyKey"],
			user:        queries["User"],
		}, nil
	}
	raw := c.OriginalURL()
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		raw = raw[i+1:]
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode request blob: %w", err)
	}
	r := bytes.NewReader(decoded)
	version, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	code, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	// Locale (2 bytes, little-endian) is not needed by the server.
	lo, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	hi, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	_ = lo
	_ = hi
	readLen := func() (string, error) {
		n, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf), nil
	}
	deviceID, err := readLen()
	if err != nil {
		return nil, err
	}
	policyKey, err := readLen()
	if err != nil {
		return nil, err
	}
	deviceType, err := readLen()
	if err != nil {
		return nil, err
	}
	name, ok := commandByCode[code]
	if !ok {
		return nil, fmt.Errorf("unknown command code %d", code)
	}
	return &easRequest{
		command: name, protocolVer: version, deviceID: deviceID,
		deviceType: deviceType, policyKey: policyKey,
	}, nil
}

func parseVersion(v string) byte {
	switch strings.TrimSpace(v) {
	case "12.1":
		return 121
	case "14.0":
		return 140
	case "14.1":
		return 141
	case "12.0":
		return 120
	}
	return 141
}

func versionString(v byte) string {
	switch v {
	case 120:
		return "12.0"
	case 121:
		return "12.1"
	case 140:
		return "14.0"
	default:
		return "14.1"
	}
}

// authenticate validates HTTP Basic credentials (mailbox password, app
// token, or AD/LDAP bind) and returns the user plus the credential to pass
// through to the IMAP/SMTP gateway.
func (s *Service) authenticate(c *fiber.Ctx, req *easRequest) (*models.User, string, error) {
	authz := c.Get("Authorization")
	if !strings.HasPrefix(authz, "Basic ") {
		return nil, "", errors.New("missing Basic auth")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authz, "Basic "))
	if err != nil {
		return nil, "", err
	}
	email, pw, ok := strings.Cut(string(raw), ":")
	email = strings.ToLower(strings.TrimSpace(email))
	// Strip domain qualifiers some clients send (DOMAIN\user).
	if i := strings.LastIndex(email, `\`); i >= 0 {
		email = email[i+1:]
	}
	if !ok || email == "" || pw == "" {
		return nil, "", errors.New("malformed Basic credentials")
	}
	if req.user != "" {
		req.user = strings.ToLower(strings.TrimSpace(req.user))
	}
	var user models.User
	err = s.DB.WithContext(c.Context()).First(&user, "email = ?", email).Error
	if err == nil && user.Enabled {
		// Cache the expensive credential verification (bcrypt / token scan)
		// while keeping the cheap indexed user lookup per request, so a
		// disabled account is rejected promptly.
		if s.AuthCache.Check(email, pw, func() bool { return s.validCredential(&user, pw) }) {
			return &user, pw, nil
		}
	}
	// Directory fallback: LDAP-bound users may not exist locally yet.
	if s.Auth != nil && s.Auth.LDAP != nil && !auth.IsAppToken(pw) {
		if ok, lerr := s.Auth.LDAP.Authenticate(c.Context(), email, pw); lerr == nil && ok {
			if user.Email == "" {
				_ = s.Auth.LDAP.EnsureLocalUser(c.Context(), email)
				if err := s.DB.WithContext(c.Context()).First(&user, "email = ?", email).Error; err != nil {
					return nil, "", err
				}
			}
			if user.Enabled {
				return &user, pw, nil
			}
		}
	}
	return nil, "", errors.New("invalid credentials")
}

// validCredential accepts the mailbox password or any app token of the user.
// App-token credentials are detected by shape and checked first, so clients
// configured with an app token never pay a pointless bcrypt comparison.
func (s *Service) validCredential(u *models.User, pw string) bool {
	if auth.IsAppToken(pw) {
		return s.matchesToken(u, pw)
	}
	if password.Verify(u.Password, pw) {
		return true
	}
	// Legacy/imported tokens may not carry the 32-hex shape; keep accepting
	// any stored token after the password check.
	if s.matchesToken(u, pw) {
		return true
	}
	if s.Auth != nil && s.Auth.LDAP != nil {
		if ok, err := s.Auth.LDAP.Authenticate(context.Background(), u.Email, pw); err == nil && ok {
			return true
		}
	}
	return false
}

// matchesToken reports whether pw matches one of the user's stored app
// tokens (PBKDF2-SHA256, so no bcrypt cost on this path).
func (s *Service) matchesToken(u *models.User, pw string) bool {
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

// dispatch routes a parsed command to its handler.
func (s *Service) dispatch(c *fiber.Ctx, req *easRequest, user *models.User, credential string) error {
	switch strings.ToLower(req.command) {
	case "provision":
		return s.cmdProvision(c, req, user)
	case "foldersync":
		return s.cmdFolderSync(c, req, user, credential)
	case "sync":
		return s.cmdSync(c, req, user, credential)
	case "ping":
		return s.cmdPing(c, req, user, credential)
	case "sendmail", "smartreply", "smartforward":
		return s.cmdSendMail(c, req, user, credential, strings.ToLower(req.command))
	case "search":
		return s.cmdSearch(c, req, user, credential)
	case "settings":
		return s.cmdSettings(c, req, user)
	case "itemoperations":
		return s.cmdItemOperations(c, req, user, credential)
	case "getitemestimate":
		return s.cmdGetItemEstimate(c, req, user, credential)
	case "meetingresponse":
		return s.cmdMeetingResponse(c, req, user, credential)
	case "resolverecipients":
		return s.cmdResolveRecipients(c, req, user)
	case "moveitems":
		return s.cmdMoveItems(c, req, user, credential)
	case "foldercreate", "folderupdate", "folderdelete":
		return s.cmdFolderMutation(c, req, user, credential, strings.ToLower(req.command))
	case "getattachment":
		return c.Status(http.StatusNotImplemented).SendString("GetAttachment is superseded by ItemOperations")
	default:
		log.Printf("activesync: unsupported command %q from %s", req.command, user.Email)
		return c.SendStatus(http.StatusNotImplemented)
	}
}

// readBody decodes the WBXML request body into a document tree.
func readBody(c *fiber.Ctx) (*Element, error) {
	body := c.Body()
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}
	root, err := DecodeWBXML(body, nil)
	if err != nil {
		return nil, fmt.Errorf("decode wbxml: %w", err)
	}
	return root, nil
}

// writeWBXML encodes the response tree and sends it with EAS headers.
func writeWBXML(c *fiber.Ctx, root *Element, version byte) error {
	data, err := EncodeWBXML(root)
	if err != nil {
		log.Printf("activesync: encode wbxml: %v", err)
		return c.SendStatus(http.StatusInternalServerError)
	}
	c.Set("Content-Type", "application/vnd.ms-sync.wbxml")
	c.Set("MS-ASProtocolVersion", versionString(version))
	return c.Status(http.StatusOK).Send(data)
}

// writeEmptyOK answers a command with an empty 200 (used by Provision when
// the policy is accepted without a document).
func writeEmptyOK(c *fiber.Ctx, version byte) error {
	c.Set("Content-Type", "application/vnd.ms-sync.wbxml")
	c.Set("MS-ASProtocolVersion", versionString(version))
	return c.Status(http.StatusOK).Send(nil)
}

// handleAutodiscover answers the XML autodiscover request so mobile clients
// can discover the ActiveSync endpoint from just the email address.
func (s *Service) handleAutodiscover(c *fiber.Ctx) error {
	if c.Method() == http.MethodOptions {
		return c.SendStatus(http.StatusOK)
	}
	host := s.Cfg.Hostname
	if host == "" || host == "localhost" {
		host = c.Hostname()
	}
	var (
		email string
	)
	if authz := c.Get("Authorization"); strings.HasPrefix(authz, "Basic ") {
		if raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(authz, "Basic ")); err == nil {
			email, _, _ = strings.Cut(string(raw), ":")
			email = strings.ToLower(strings.TrimSpace(email))
		}
	}
	if email == "" {
		// Fall back to parsing the request payload for the email address.
		body, _ := io.ReadAll(c.Request().BodyStream())
		var probe struct {
			Request struct {
				AcceptableResponseSchema string `xml:"AcceptableResponseSchema"`
				EMailAddress             string `xml:"EMailAddress"`
			} `xml:"Request"`
		}
		if err := xml.Unmarshal(body, &probe); err == nil && probe.Request.EMailAddress != "" {
			email = strings.ToLower(probe.Request.EMailAddress)
		}
	}
	resp := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/2010/Autodiscover">
  <Response xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a">
    <User>
      <DisplayName>%s</DisplayName>
      <EMailAddress>%s</EMailAddress>
    </User>
    <Account>
      <AccountType>email</AccountType>
      <Action>settings</Action>
      <Protocol>
        <Type>activesync</Type>
        <Server>%s</Server>
        <Port>443</Port>
        <SSL>on</SSL>
        <AuthPackage>Basic</AuthPackage>
        <DomainRequired>off</DomainRequired>
      </Protocol>
    </Account>
  </Response>
</Autodiscover>`, xmlEscape(email), xmlEscape(email), xmlEscape(host))
	c.Set("Content-Type", "text/xml; charset=utf-8")
	return c.Status(http.StatusOK).SendString(resp)
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
