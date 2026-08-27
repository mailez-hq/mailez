package activesync

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"mailez/backend/internal/auth"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/mail"
	"mailez/backend/internal/password"
)

// fakeEASGateway implements the mail surface ActiveSync uses, backed by
// in-memory folders.
type fakeEASGateway struct {
	mail.Gateway
	mu       sync.Mutex
	folders  []string
	byFolder map[string][]mail.Message
	nextUID  uint32

	sentRaw  []string
	appended []string
	sentTo   []string
	sentSubj string
	moved    []string
	cleared  []string
	created  []string
	renamed  map[string]string
	deleted  []string
}

func newFakeGateway() *fakeEASGateway {
	g := &fakeEASGateway{
		folders:  []string{"Inbox", "Sent", "Drafts", "Trash", "Junk", "Archive"},
		byFolder: map[string][]mail.Message{},
		nextUID:  100,
		renamed:  map[string]string{},
	}
	for _, f := range g.folders {
		g.byFolder[f] = []mail.Message{}
	}
	g.addMessage("Inbox", "Welcome to Mailez", "boss@example.com", "alice@example.com", false, false)
	g.addMessage("Inbox", "Re: Project plan", "carol@example.com", "alice@example.com", true, false)
	return g
}

func (g *fakeEASGateway) addMessage(folder, subject, from, to string, read, flagged bool) uint32 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.nextUID++
	uid := g.nextUID
	flags := []string{}
	if read {
		flags = append(flags, `\Seen`)
	}
	if flagged {
		flags = append(flags, `\Flagged`)
	}
	msg := mail.Message{
		UID: uid, ID: fmt.Sprintf("id%d", uid), Subject: subject,
		From:  []mail.Address{{Name: from, Email: from}},
		To:    []mail.Address{{Name: to, Email: to}},
		Date:  time.Now().Add(-time.Duration(uid) * time.Minute),
		Flags: flags, TextBody: "Body of " + subject,
		HasAttachment: false,
	}
	if strings.Contains(subject, "attach") {
		msg.HasAttachment = true
		msg.Attachments = []mail.Attachment{{
			Filename: "report.txt", ContentType: "text/plain", Size: 5,
			Data: base64.StdEncoding.EncodeToString([]byte("hello")),
		}}
	}
	g.byFolder[folder] = append(g.byFolder[folder], msg)
	return uid
}

func (g *fakeEASGateway) With(dial mail.Dial) mail.Gateway { return g }

func (g *fakeEASGateway) ListFolders(email, token string) ([]string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := append([]string(nil), g.folders...)
	sort.Strings(out)
	return out, nil
}

func (g *fakeEASGateway) ListAllMessages(email, token, folder string) ([]mail.Message, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := append([]mail.Message(nil), g.byFolder[folder]...)
	return out, nil
}

func (g *fakeEASGateway) FolderStat(email, token, folder string) (mail.FolderStat, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	msgs := g.byFolder[folder]
	var unseen uint32
	for _, m := range msgs {
		if !hasFlag(m.Flags, `\Seen`) {
			unseen++
		}
	}
	return mail.FolderStat{Messages: uint32(len(msgs)), Unseen: unseen, UidNext: g.nextUID + 1}, nil
}

func (g *fakeEASGateway) GetMessage(email, token, folder string, uid uint32) (*mail.Message, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.byFolder[folder] {
		m := g.byFolder[folder][i]
		if m.UID == uid {
			cp := m
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (g *fakeEASGateway) GetRaw(email, token, folder string, uid uint32) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, m := range g.byFolder[folder] {
		if m.UID == uid {
			return fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
				m.From[0].Email, m.To[0].Email, m.Subject, m.TextBody), nil
		}
	}
	return "", fmt.Errorf("not found")
}

func (g *fakeEASGateway) SetFlag(email, token, folder string, uid uint32, flag string, value bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	for i := range g.byFolder[folder] {
		m := &g.byFolder[folder][i]
		if m.UID != uid {
			continue
		}
		idx := -1
		for j, f := range m.Flags {
			if strings.EqualFold(f, flag) {
				idx = j
				break
			}
		}
		if value && idx < 0 {
			m.Flags = append(m.Flags, flag)
		}
		if !value && idx >= 0 {
			m.Flags = append(m.Flags[:idx], m.Flags[idx+1:]...)
		}
	}
	return nil
}

func (g *fakeEASGateway) Delete(email, token, folder string, uid uint32) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.deleted = append(g.deleted, fmt.Sprintf("%s:%d", folder, uid))
	for i, m := range g.byFolder[folder] {
		if m.UID == uid {
			g.byFolder[folder] = append(g.byFolder[folder][:i], g.byFolder[folder][i+1:]...)
			g.byFolder["Trash"] = append(g.byFolder["Trash"], m)
			break
		}
	}
	return nil
}

func (g *fakeEASGateway) MoveMany(email, token, folder string, uids []uint32, destination string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.moved = append(g.moved, fmt.Sprintf("%s->%s:%v", folder, destination, uids))
	want := map[uint32]bool{}
	for _, u := range uids {
		want[u] = true
	}
	var kept []mail.Message
	for _, m := range g.byFolder[folder] {
		if want[m.UID] {
			g.byFolder[destination] = append(g.byFolder[destination], m)
		} else {
			kept = append(kept, m)
		}
	}
	g.byFolder[folder] = kept
	return nil
}

func (g *fakeEASGateway) UIDByMessageID(email, token, folder, id string) (uint32, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, m := range g.byFolder[folder] {
		if m.ID == id {
			return m.UID, nil
		}
	}
	return 0, fmt.Errorf("not found")
}

func (g *fakeEASGateway) CreateFolder(email, token, name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.created = append(g.created, name)
	if _, ok := g.byFolder[name]; !ok {
		g.byFolder[name] = []mail.Message{}
		g.folders = append(g.folders, name)
	}
	return nil
}

func (g *fakeEASGateway) RenameFolder(email, token, oldName, newName string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.renamed[oldName] = newName
	if msgs, ok := g.byFolder[oldName]; ok {
		delete(g.byFolder, oldName)
		g.byFolder[newName] = msgs
	}
	for i, f := range g.folders {
		if f == oldName {
			g.folders[i] = newName
		}
	}
	return nil
}

func (g *fakeEASGateway) DeleteFolder(email, token, name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.byFolder, name)
	for i, f := range g.folders {
		if f == name {
			g.folders = append(g.folders[:i], g.folders[i+1:]...)
			break
		}
	}
	return nil
}

func (g *fakeEASGateway) ClearFolder(email, token, name string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.cleared = append(g.cleared, name)
	g.byFolder[name] = []mail.Message{}
	return nil
}

func (g *fakeEASGateway) SearchAllMessages(email, token, query string) ([]mail.Message, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []mail.Message
	q := strings.ToLower(query)
	for _, msgs := range g.byFolder {
		for _, m := range msgs {
			if strings.Contains(strings.ToLower(m.Subject), q) ||
				strings.Contains(strings.ToLower(m.TextBody), q) {
				out = append(out, m)
			}
		}
	}
	return out, nil
}

func (g *fakeEASGateway) Send(email, token, from string, to, cc, bcc []string, subject, text, html string, attachments []mail.Attachment, extra ...mail.Header) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sentTo = to
	g.sentSubj = subject
	return nil
}

func (g *fakeEASGateway) SubmitRawAs(email, token, from string, recipients []string, raw string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sentRaw = append(g.sentRaw, raw)
	return nil
}

func (g *fakeEASGateway) AppendRaw(email, token, folder, raw string, flags []string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.appended = append(g.appended, raw)
	// Parse the subject for the fake store so the sent copy syncs back.
	subject := "unknown"
	if i := strings.Index(raw, "Subject:"); i >= 0 {
		line := raw[i:]
		if j := strings.IndexAny(line, "\r\n"); j > 0 {
			subject = strings.TrimSpace(line[len("Subject:"):j])
		}
	}
	from := "sent@example.com"
	to := "alice@example.com"
	g.byFolder["Sent"] = append(g.byFolder["Sent"], mail.Message{
		UID: g.nextUID, ID: fmt.Sprintf("sent%d", g.nextUID), Subject: subject,
		From: []mail.Address{{Name: from, Email: from}}, To: []mail.Address{{Name: to, Email: to}},
		Date: time.Now(), Flags: []string{`\Seen`}, TextBody: "sent copy",
	})
	g.nextUID++
	return nil
}

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

const (
	testEmail    = "alice@example.com"
	testPassword = "secret-pass"
	testDevice   = "TESTDEVICE123"
)

func newTestService(t *testing.T) (*fiber.App, *fakeEASGateway, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "eas.db")), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
		Logger:         logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { sqlDB.Close() })
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.EasDevice{}, &models.EasSyncState{}, &models.EasPingState{},
		&models.CalendarEvent{}, &models.OrgContact{}, &models.Alias{}, &models.Contact{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	hash, _ := password.Hash(testPassword)
	user := models.User{
		Email: testEmail, Localpart: "alice", DomainName: "example.com",
		Password: hash, Enabled: true,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	mgr := auth.NewManager(db, auth.NewMemoryStore(), "mailez_session", time.Hour)
	cfg := core.Config{SecretKey: "test-secret", Domain: "example.com", Hostname: "mail.example.com"}
	gw := newFakeGateway()
	svc := New(db, mgr, cfg, gw)
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	svc.Register(app)
	return app, gw, db
}

func basicAuth(email, pw string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(email+":"+pw))
}

// doEAS runs a request against the ActiveSync endpoint.
func doEAS(t *testing.T, app *fiber.App, method, url, auth string, body *Element) (*http.Response, []byte) {
	t.Helper()
	var raw []byte
	if body != nil {
		enc, err := EncodeWBXML(body)
		if err != nil {
			t.Fatalf("encode request: %v", err)
		}
		raw = enc
	}
	req := httptest.NewRequest(method, url, bytes.NewReader(raw))
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	req.Header.Set("MS-ASProtocolVersion", "14.1")
	if raw != nil {
		req.Header.Set("Content-Type", "application/vnd.ms-sync.wbxml")
	}
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("request %s: %v", url, err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, b
}

func authURL(cmd string) string {
	return "/Microsoft-Server-ActiveSync?Cmd=" + cmd + "&DeviceId=" + testDevice +
		"&DeviceType=iPhone&User=" + testEmail
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestOptions(t *testing.T) {
	app, _, _ := newTestService(t)
	req := httptest.NewRequest(http.MethodOptions, "/Microsoft-Server-ActiveSync", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("MS-ASProtocolCommands"); !strings.Contains(got, "Sync") {
		t.Fatalf("commands = %q", got)
	}
	if got := resp.Header.Get("MS-ASProtocolVersions"); !strings.Contains(got, "14.1") {
		t.Fatalf("versions = %q", got)
	}
}

func TestBase64BlobRequestForm(t *testing.T) {
	app, _, _ := newTestService(t)
	// Encode the iOS-style request blob: version 14.1, command 9 (FolderSync),
	// locale 0x0409, device id, policy key, device type.
	blob := []byte{141, 9, 0x09, 0x04, 13}
	blob = append(blob, []byte("TESTDEVICE123")...)
	blob = append(blob, 0)
	blob = append(blob, 6)
	blob = append(blob, []byte("iPhone")...)
	url := "/Microsoft-Server-ActiveSync?" + base64.StdEncoding.EncodeToString(blob)

	body := &Element{NS: nsFolderHierarchy, Name: "FolderSync"}
	body.Add(nsFolderHierarchy, "SyncKey", "0")
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewReader(mustEncode(t, body)))
	req.Header.Set("Authorization", basicAuth(testEmail, testPassword))
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := root.ChildText(nsFolderHierarchy, "SyncKey"); got == "" {
		t.Fatalf("no sync key: %s", root.String())
	}
}

func mustEncode(t *testing.T, root *Element) []byte {
	t.Helper()
	enc, err := EncodeWBXML(root)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return enc
}

func TestAuthRejected(t *testing.T) {
	app, _, _ := newTestService(t)
	body := &Element{NS: nsFolderHierarchy, Name: "FolderSync"}
	body.Add(nsFolderHierarchy, "SyncKey", "0")
	resp, _ := doEAS(t, app, http.MethodPost, authURL("FolderSync"), basicAuth(testEmail, "wrong"), body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestProvision(t *testing.T) {
	app, _, _ := newTestService(t)
	reqBody := &Element{NS: nsProvision, Name: "Provision"}
	pols := reqBody.Add(nsProvision, "Policies", "")
	pol := pols.Add(nsProvision, "Policy", "")
	pol.Add(nsProvision, "PolicyType", "MS-EAS-Provisioning-WBXML")

	resp, raw := doEAS(t, app, http.MethodPost, authURL("Provision"), basicAuth(testEmail, testPassword), reqBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	key := root.Child(nsProvision, "Policies").Child(nsProvision, "Policy").ChildText(nsProvision, "PolicyKey")
	if key == "" {
		t.Fatal("no policy key returned")
	}
	if got := root.Child(nsProvision, "Policies").Child(nsProvision, "Policy").
		Child(nsProvision, "Data").Child(nsProvision, "EASProvisionDoc").ChildText(nsProvision, "DevicePasswordEnabled"); got != "1" {
		t.Fatalf("DevicePasswordEnabled = %q", got)
	}

	// Second round: accept the policy.
	ack := &Element{NS: nsProvision, Name: "Provision"}
	pols2 := ack.Add(nsProvision, "Policies", "")
	pol2 := pols2.Add(nsProvision, "Policy", "")
	pol2.Add(nsProvision, "PolicyType", "MS-EAS-Provisioning-WBXML")
	pol2.Add(nsProvision, "PolicyKey", key)
	pol2.Add(nsProvision, "Status", "1")
	resp2, raw2 := doEAS(t, app, http.MethodPost, authURL("Provision"), basicAuth(testEmail, testPassword), ack)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("ack status = %d", resp2.StatusCode)
	}
	if len(raw2) != 0 {
		t.Fatalf("expected empty ack body, got %d bytes: %q", len(raw2), raw2)
	}
}

func TestFolderSyncFlow(t *testing.T) {
	app, _, _ := newTestService(t)
	auth := basicAuth(testEmail, testPassword)
	init := &Element{NS: nsFolderHierarchy, Name: "FolderSync"}
	init.Add(nsFolderHierarchy, "SyncKey", "0")
	_, raw := doEAS(t, app, http.MethodPost, authURL("FolderSync"), auth, init)
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	key := root.ChildText(nsFolderHierarchy, "SyncKey")
	if key == "" || key == "0" {
		t.Fatalf("bad sync key %q", key)
	}
	if got := root.ChildText(nsFolderHierarchy, "Status"); got != "1" {
		t.Fatalf("status = %q", got)
	}

	full := &Element{NS: nsFolderHierarchy, Name: "FolderSync"}
	full.Add(nsFolderHierarchy, "SyncKey", key)
	_, raw2 := doEAS(t, app, http.MethodPost, authURL("FolderSync"), auth, full)
	root2, err := DecodeWBXML(raw2, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	changes := root2.Child(nsFolderHierarchy, "Changes")
	if changes == nil {
		t.Fatal("no Changes in folder sync")
	}
	adds := changes.ChildrenNamed(nsFolderHierarchy, "Add")
	if len(adds) < 6 {
		t.Fatalf("Add count = %d", len(adds))
	}
	var foundInbox bool
	for _, a := range adds {
		if a.ChildText(nsFolderHierarchy, "ServerId") == "inbox" {
			foundInbox = true
			if a.ChildText(nsFolderHierarchy, "Type") != "2" {
				t.Fatalf("inbox type = %s", a.ChildText(nsFolderHierarchy, "Type"))
			}
			if a.ChildText(nsFolderHierarchy, "DisplayName") != "Inbox" {
				t.Fatalf("inbox name = %s", a.ChildText(nsFolderHierarchy, "DisplayName"))
			}
		}
	}
	if !foundInbox {
		t.Fatal("inbox folder missing")
	}
}

func TestSyncIncremental(t *testing.T) {
	app, gw, _ := newTestService(t)
	auth := basicAuth(testEmail, testPassword)

	// Initial sync key.
	init := &Element{NS: nsAirSync, Name: "Sync"}
	cols := init.Add(nsAirSync, "Collections", "")
	col := cols.Add(nsAirSync, "Collection", "")
	col.Add(nsAirSync, "Class", "Email")
	col.Add(nsAirSync, "SyncKey", "0")
	col.Add(nsAirSync, "CollectionId", "inbox")
	col.Add(nsAirSync, "GetChanges", "1")
	_, raw := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, init)
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	colOut := root.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection")
	key1 := colOut.ChildText(nsAirSync, "SyncKey")
	if key1 == "" {
		t.Fatal("no sync key from initial sync")
	}

	// Full sync: expect the two inbox messages as Adds.
	full := &Element{NS: nsAirSync, Name: "Sync"}
	cols2 := full.Add(nsAirSync, "Collections", "")
	col2 := cols2.Add(nsAirSync, "Collection", "")
	col2.Add(nsAirSync, "Class", "Email")
	col2.Add(nsAirSync, "SyncKey", key1)
	col2.Add(nsAirSync, "CollectionId", "inbox")
	col2.Add(nsAirSync, "GetChanges", "1")
	opts := col2.Add(nsAirSync, "Options", "")
	bp := opts.Add(nsAirSyncBase, "BodyPreference", "")
	bp.Add(nsAirSyncBase, "Type", "0")
	bp.Add(nsAirSyncBase, "TruncationSize", "10240")
	_, raw2 := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, full)
	root2, err := DecodeWBXML(raw2, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	colOut2 := root2.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection")
	if got := colOut2.ChildText(nsAirSync, "Status"); got != "1" {
		t.Fatalf("sync status = %q", got)
	}
	commands := colOut2.Child(nsAirSync, "Commands")
	if commands == nil {
		t.Fatal("no commands in full sync")
	}
	adds := commands.ChildrenNamed(nsAirSync, "Add")
	if len(adds) != 2 {
		t.Fatalf("adds = %d, want 2", len(adds))
	}
	first := adds[0].Child(nsAirSync, "ApplicationData")
	if got := first.ChildText(nsEmail, "Subject"); got == "" {
		t.Fatal("missing Email:Subject")
	}
	if got := first.ChildText(nsEmail, "From"); got == "" {
		t.Fatal("missing Email:From")
	}
	body := first.Child(nsAirSyncBase, "Body")
	if body == nil || body.ChildText(nsAirSyncBase, "Type") != "1" {
		t.Fatalf("body preference not honoured: %v", body)
	}
	if body.ChildText(nsAirSyncBase, "Data") == "" {
		t.Fatal("missing body data")
	}
	key2 := colOut2.ChildText(nsAirSync, "SyncKey")

	// Third sync: nothing changed.
	full3 := &Element{NS: nsAirSync, Name: "Sync"}
	cols3 := full3.Add(nsAirSync, "Collections", "")
	col3 := cols3.Add(nsAirSync, "Collection", "")
	col3.Add(nsAirSync, "Class", "Email")
	col3.Add(nsAirSync, "SyncKey", key2)
	col3.Add(nsAirSync, "CollectionId", "inbox")
	col3.Add(nsAirSync, "GetChanges", "1")
	_, raw3 := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, full3)
	root3, err := DecodeWBXML(raw3, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	colOut3 := root3.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection")
	if got := colOut3.ChildText(nsAirSync, "Status"); got != "1" {
		t.Fatalf("third sync status = %q", got)
	}
	if colOut3.Child(nsAirSync, "Commands") != nil {
		t.Fatal("unexpected commands on unchanged sync")
	}
	key3 := colOut3.ChildText(nsAirSync, "SyncKey")

	// Mark a message read from another client and expect a Change.
	gw.addMessage("Inbox", "New message", "dave@example.com", "alice@example.com", false, false)
	full4 := &Element{NS: nsAirSync, Name: "Sync"}
	cols4 := full4.Add(nsAirSync, "Collections", "")
	col4 := cols4.Add(nsAirSync, "Collection", "")
	col4.Add(nsAirSync, "Class", "Email")
	col4.Add(nsAirSync, "SyncKey", key3)
	col4.Add(nsAirSync, "CollectionId", "inbox")
	col4.Add(nsAirSync, "GetChanges", "1")
	_, raw4 := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, full4)
	root4, err := DecodeWBXML(raw4, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	colOut4 := root4.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection")
	if got := colOut4.ChildText(nsAirSync, "Status"); got != "1" {
		t.Fatalf("fourth sync status = %q: %s", got, colOut4.String())
	}
	cmds4 := colOut4.Child(nsAirSync, "Commands")
	if cmds4 == nil || len(cmds4.ChildrenNamed(nsAirSync, "Add")) != 1 {
		t.Fatalf("expected 1 add after new message, got %v", colOut4.String())
	}
}

func TestSyncClientCommands(t *testing.T) {
	app, gw, _ := newTestService(t)
	auth := basicAuth(testEmail, testPassword)
	uid := gw.addMessage("Inbox", "Flag me", "eve@example.com", "alice@example.com", false, false)

	body := &Element{NS: nsAirSync, Name: "Sync"}
	cols := body.Add(nsAirSync, "Collections", "")
	col := cols.Add(nsAirSync, "Collection", "")
	col.Add(nsAirSync, "Class", "Email")
	col.Add(nsAirSync, "SyncKey", "0")
	col.Add(nsAirSync, "CollectionId", "inbox")
	cmds := col.Add(nsAirSync, "Commands", "")
	change := cmds.Add(nsAirSync, "Change", "")
	change.Add(nsAirSync, "ServerId", fmt.Sprintf("%d", uid))
	appData := change.Add(nsAirSync, "ApplicationData", "")
	appData.Add(nsEmail, "Read", "1")
	flag := appData.Add(nsEmail, "Flag", "")
	flag.Add(nsEmail, "Status", "2")

	_, raw := doEAS(t, app, http.MethodPost, authURL("Sync"), auth, body)
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := root.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection").
		ChildText(nsAirSync, "Status"); got != "1" {
		t.Fatalf("status = %q", got)
	}
	msgs, _ := gw.ListAllMessages(testEmail, "", "Inbox")
	for _, m := range msgs {
		if m.UID == uid {
			if !hasFlag(m.Flags, `\Seen`) || !hasFlag(m.Flags, `\Flagged`) {
				t.Fatalf("flags not applied: %v", m.Flags)
			}
			return
		}
	}
	t.Fatal("message not found after change")
}

func TestPing(t *testing.T) {
	app, gw, _ := newTestService(t)
	auth := basicAuth(testEmail, testPassword)
	pingBody := func(heartbeat int) *Element {
		p := &Element{NS: nsPing, Name: "Ping"}
		p.Add(nsPing, "HeartbeatInterval", fmt.Sprintf("%d", heartbeat))
		folders := p.Add(nsPing, "Folders", "")
		f := folders.Add(nsPing, "Folder", "")
		f.Add(nsPing, "Id", "inbox")
		f.Add(nsPing, "Class", "Email")
		return p
	}
	// First ping establishes the baseline.
	_, raw := doEAS(t, app, http.MethodPost, authURL("Ping"), auth, pingBody(1))
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := root.ChildText(nsPing, "Status"); got != "1" {
		t.Fatalf("first ping status = %q", got)
	}
	// New mail arrives.
	gw.addMessage("Inbox", "Push me", "frank@example.com", "alice@example.com", false, false)
	_, raw2 := doEAS(t, app, http.MethodPost, authURL("Ping"), auth, pingBody(1))
	root2, err := DecodeWBXML(raw2, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := root2.ChildText(nsPing, "Status"); got != "2" {
		t.Fatalf("second ping status = %q", got)
	}
	folders := root2.Child(nsPing, "Folders")
	if folders == nil || len(folders.ChildrenNamed(nsPing, "Folder")) != 1 {
		t.Fatalf("expected changed folder list: %s", root2.String())
	}
}

func TestSendMail(t *testing.T) {
	app, gw, _ := newTestService(t)
	auth := basicAuth(testEmail, testPassword)
	mime := "From: alice@example.com\r\nTo: bob@example.com\r\nSubject: EAS test\r\n\r\nHello from ActiveSync"
	body := &Element{NS: nsComposeMail, Name: "SendMail"}
	body.Add(nsComposeMail, "ClientId", "client-1")
	body.Add(nsComposeMail, "SaveInSentItems", "1")
	body.Add(nsComposeMail, "Mime", base64.StdEncoding.EncodeToString([]byte(mime)))

	_, raw := doEAS(t, app, http.MethodPost, authURL("SendMail"), auth, body)
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := root.ChildText(nsComposeMail, "Status"); got != "1" {
		t.Fatalf("status = %q", got)
	}
	if len(gw.sentRaw) != 1 || !strings.Contains(gw.sentRaw[0], "Hello from ActiveSync") {
		t.Fatalf("raw not submitted: %v", gw.sentRaw)
	}
	if len(gw.appended) != 1 || !strings.Contains(gw.appended[0], "Hello from ActiveSync") {
		t.Fatalf("sent copy not appended: %v", gw.appended)
	}
}

func TestSearchGALAndMailbox(t *testing.T) {
	app, _, db := newTestService(t)
	auth := basicAuth(testEmail, testPassword)
	if err := db.Create(&models.OrgContact{Email: "hr@example.com", Name: "人力资源", Department: "HR"}).Error; err != nil {
		t.Fatal(err)
	}
	body := &Element{NS: nsSearch, Name: "Search"}
	store := body.Add(nsSearch, "Store", "")
	store.Add(nsSearch, "Name", "GAL")
	store.Add(nsSearch, "Query", "hr")
	_, raw := doEAS(t, app, http.MethodPost, authURL("Search"), auth, body)
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := root.ChildText(nsSearch, "Status"); got != "1" {
		t.Fatalf("search status = %q", got)
	}
	respStore := root.Child(nsSearch, "Response").Child(nsSearch, "Store")
	if respStore == nil {
		t.Fatal("no store in response")
	}
	results := respStore.ChildrenNamed(nsSearch, "Result")
	if len(results) == 0 {
		t.Fatalf("no GAL results: %s", respStore.String())
	}
	props := results[0].Child(nsSearch, "Properties")
	if got := props.ChildText(nsGAL, "EmailAddress"); got != "hr@example.com" {
		t.Fatalf("GAL email = %q", got)
	}

	// Mailbox search.
	body2 := &Element{NS: nsSearch, Name: "Search"}
	store2 := body2.Add(nsSearch, "Store", "")
	store2.Add(nsSearch, "Name", "Mailbox")
	store2.Add(nsSearch, "Query", "Mailez")
	_, raw2 := doEAS(t, app, http.MethodPost, authURL("Search"), auth, body2)
	root2, err := DecodeWBXML(raw2, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	storeOut := root2.Child(nsSearch, "Response").Child(nsSearch, "Store")
	if storeOut == nil {
		t.Fatalf("no store in mailbox search: %s", root2.String())
	}
	props2 := storeOut.Child(nsSearch, "Result").Child(nsSearch, "Properties")
	if got := props2.ChildText(nsEmail, "Subject"); got != "Welcome to Mailez" {
		t.Fatalf("mailbox search subject = %q", got)
	}
}

func TestMoveItems(t *testing.T) {
	app, gw, _ := newTestService(t)
	auth := basicAuth(testEmail, testPassword)
	uid := gw.addMessage("Inbox", "Move me", "grace@example.com", "alice@example.com", false, false)
	body := &Element{NS: nsMove, Name: "MoveItems"}
	move := body.Add(nsMove, "Move", "")
	move.Add(nsMove, "SrcMsgId", fmt.Sprintf("%d", uid))
	move.Add(nsMove, "SrcFldId", "inbox")
	move.Add(nsMove, "DstFldId", "archive")
	_, raw := doEAS(t, app, http.MethodPost, authURL("MoveItems"), auth, body)
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp := root.Child(nsMove, "Response")
	if resp == nil || resp.ChildText(nsMove, "Status") != "1" {
		t.Fatalf("move response: %s", root.String())
	}
	msgs, _ := gw.ListAllMessages(testEmail, "", "Archive")
	if len(msgs) != 1 || msgs[0].UID != uid {
		t.Fatalf("message not moved: %v", msgs)
	}
}

func TestItemOperations(t *testing.T) {
	app, gw, _ := newTestService(t)
	auth := basicAuth(testEmail, testPassword)
	uid := gw.addMessage("Inbox", "Has attach", "henry@example.com", "alice@example.com", false, false)

	body := &Element{NS: nsItemOperations, Name: "ItemOperations"}
	fetch := body.Add(nsItemOperations, "Fetch", "")
	store := fetch.Add(nsItemOperations, "Store", "")
	store.Add(nsAirSync, "CollectionId", "inbox")
	store.Add(nsAirSync, "ServerId", fmt.Sprintf("%d", uid))
	_, raw := doEAS(t, app, http.MethodPost, authURL("ItemOperations"), auth, body)
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	fetchOut := root.Child(nsItemOperations, "Response").Child(nsItemOperations, "Fetch")
	if fetchOut == nil || fetchOut.ChildText(nsItemOperations, "Status") != "1" {
		t.Fatalf("fetch response: %s", root.String())
	}
	bodyData := fetchOut.Child(nsItemOperations, "Properties").Child(nsAirSyncBase, "Body")
	if bodyData.ChildText(nsAirSyncBase, "Type") != "4" {
		t.Fatalf("fetch body type = %q", bodyData.ChildText(nsAirSyncBase, "Type"))
	}
	if bodyData.ChildText(nsAirSyncBase, "Data") == "" {
		t.Fatal("fetch body data empty")
	}

	// Attachment fetch by FileReference.
	body2 := &Element{NS: nsItemOperations, Name: "ItemOperations"}
	fetch2 := body2.Add(nsItemOperations, "Fetch", "")
	store2 := fetch2.Add(nsItemOperations, "Store", "")
	store2.Add(nsAirSync, "CollectionId", "inbox")
	store2.Add(nsAirSyncBase, "FileReference", fmt.Sprintf("%d:0", uid))
	_, raw2 := doEAS(t, app, http.MethodPost, authURL("ItemOperations"), auth, body2)
	root2, err := DecodeWBXML(raw2, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	attBody := root2.Child(nsItemOperations, "Response").Child(nsItemOperations, "Fetch").
		Child(nsItemOperations, "Properties").Child(nsAirSyncBase, "Body")
	attData, _ := base64.StdEncoding.DecodeString(attBody.ChildText(nsAirSyncBase, "Data"))
	if string(attData) != "hello" {
		t.Fatalf("attachment data = %q", attData)
	}
}

func TestSettingsUserInformation(t *testing.T) {
	app, _, _ := newTestService(t)
	auth := basicAuth(testEmail, testPassword)
	body := &Element{NS: nsSettings, Name: "Settings"}
	body.Add(nsSettings, "UserInformation", "")
	_, raw := doEAS(t, app, http.MethodPost, authURL("Settings"), auth, body)
	root, err := DecodeWBXML(raw, nil)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	info := root.Child(nsSettings, "UserInformation")
	if info == nil {
		t.Fatalf("no user information: %s", root.String())
	}
	if got := info.Child(nsSettings, "EmailAddresses").ChildText(nsSettings, "SmtpAddress"); got != testEmail {
		t.Fatalf("smtp address = %q", got)
	}
}

func TestAutodiscover(t *testing.T) {
	app, _, _ := newTestService(t)
	reqBody := `<?xml version="1.0" encoding="utf-8"?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/requestschema/2006">
  <Request>
    <EMailAddress>alice@example.com</EMailAddress>
    <AcceptableResponseSchema>http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a</AcceptableResponseSchema>
  </Request>
</Autodiscover>`
	req := httptest.NewRequest(http.MethodPost, "/autodiscover/autodiscover.xml", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("Authorization", basicAuth(testEmail, testPassword))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "<Type>activesync</Type>") {
		t.Fatalf("no activesync protocol: %s", body)
	}
	if !strings.Contains(body, "mail.example.com") {
		t.Fatalf("no server host: %s", body)
	}
	var parsed struct {
		XMLName xml.Name `xml:"Autodiscover"`
	}
	if err := xml.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("not valid XML: %v", err)
	}
}
