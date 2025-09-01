//go:build mailez_ee

// Command eas-e2e runs an ActiveSync smoke test against a running
// mailez backend: OPTIONS, autodiscover, Provision, FolderSync, incremental
// Sync, Ping and SendMail over the WBXML transport.
//
// Usage:
//
//	go run ./cmd/eas-e2e -email admin@example.com -password '...' -port 8080
package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"mailez/backend/internal/ee/activesync"
)

const (
	nsAirSync         = "AirSync"
	nsFolderHierarchy = "FolderHierarchy"
	nsProvision       = "Provision"
	nsPing            = "Ping"
	nsComposeMail     = "ComposeMail"
	nsSettings        = "Settings"
	nsCalendar        = "Calendar"
)

var (
	host     = flag.String("host", "127.0.0.1", "backend host")
	port     = flag.Int("port", 8080, "backend port")
	email    = flag.String("email", "admin@example.com", "account email")
	password = flag.String("password", "", "account password")
	device   = flag.String("device", "E2E-SMOKE-001", "device id")
)

type easClient struct {
	base   string
	auth   string
	device string
	http   *http.Client
}

func newClient() *easClient {
	return &easClient{
		base:   fmt.Sprintf("http://%s:%d", *host, *port),
		auth:   "Basic " + base64.StdEncoding.EncodeToString([]byte(*email+":"+*password)),
		device: *device,
		http:   &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *easClient) cmd(name string, body *activesync.Element) (int, *activesync.Element, []byte, error) {
	url := c.base + "/Microsoft-Server-ActiveSync?Cmd=" + name + "&DeviceId=" + c.device +
		"&DeviceType=E2E&User=" + *email
	var payload []byte
	if body != nil {
		enc, err := activesync.EncodeWBXML(body)
		if err != nil {
			return 0, nil, nil, err
		}
		payload = enc
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("MS-ASProtocolVersion", "14.1")
	req.Header.Set("Content-Type", "application/vnd.ms-sync.wbxml")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var root *activesync.Element
	if len(raw) > 0 {
		root, err = activesync.DecodeWBXML(raw, nil)
		if err != nil {
			return resp.StatusCode, nil, raw, fmt.Errorf("decode %s response: %w", name, err)
		}
	}
	return resp.StatusCode, root, raw, nil
}

func check(step string, err error) {
	if err != nil {
		log.Fatalf("EAS E2E FAILED at %s: %v", step, err)
	}
	fmt.Printf("ok   %s\n", step)
}

func main() {
	flag.Parse()
	if *password == "" {
		*password = os.Getenv("EAS_E2E_PASSWORD")
	}
	if *password == "" {
		log.Fatal("EAS E2E FAILED: -password is required")
	}
	c := newClient()

	// OPTIONS
	optReq, _ := http.NewRequest(http.MethodOptions, c.base+"/Microsoft-Server-ActiveSync", nil)
	resp, err := c.http.Do(optReq)
	check("OPTIONS", err)
	commands := resp.Header.Get("MS-ASProtocolCommands")
	versions := resp.Header.Get("MS-ASProtocolVersions")
	resp.Body.Close()
	if !strings.Contains(commands, "Sync") || !strings.Contains(versions, "14.1") {
		log.Fatalf("EAS E2E FAILED at OPTIONS: commands=%q versions=%q", commands, versions)
	}
	fmt.Printf("ok   OPTIONS (commands=%d, versions=%s)\n", len(strings.Split(commands, ",")), versions)

	// Autodiscover
	adBody := `<?xml version="1.0" encoding="utf-8"?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/requestschema/2006">
  <Request><EMailAddress>` + *email + `</EMailAddress>
  <AcceptableResponseSchema>http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a</AcceptableResponseSchema></Request>
</Autodiscover>`
	req, _ := http.NewRequest(http.MethodPost, c.base+"/autodiscover/autodiscover.xml", strings.NewReader(adBody))
	req.Header.Set("Authorization", c.auth)
	adResp, err := c.http.Do(req)
	check("autodiscover", err)
	adRaw, _ := io.ReadAll(adResp.Body)
	adResp.Body.Close()
	if adResp.StatusCode != http.StatusOK || !strings.Contains(string(adRaw), "<Type>activesync</Type>") {
		log.Fatalf("EAS E2E FAILED at autodiscover: %d %s", adResp.StatusCode, adRaw)
	}
	fmt.Printf("ok   autodiscover\n")

	// Provision
	provision := &activesync.Element{NS: nsProvision, Name: "Provision"}
	pols := provision.Add(nsProvision, "Policies", "")
	pol := pols.Add(nsProvision, "Policy", "")
	pol.Add(nsProvision, "PolicyType", "MS-EAS-Provisioning-WBXML")
	status, root, raw, err := c.cmd("Provision", provision)
	check("provision request", err)
	if status != http.StatusOK {
		log.Fatalf("EAS E2E FAILED at provision: status %d", status)
	}
	policyKey := root.Child(nsProvision, "Policies").Child(nsProvision, "Policy").ChildText(nsProvision, "PolicyKey")
	if policyKey == "" {
		log.Fatalf("EAS E2E FAILED at provision: no policy key: %s", string(raw))
	}
	ack := &activesync.Element{NS: nsProvision, Name: "Provision"}
	pols2 := ack.Add(nsProvision, "Policies", "")
	pol2 := pols2.Add(nsProvision, "Policy", "")
	pol2.Add(nsProvision, "PolicyType", "MS-EAS-Provisioning-WBXML")
	pol2.Add(nsProvision, "PolicyKey", policyKey)
	pol2.Add(nsProvision, "Status", "1")
	status, _, raw, err = c.cmd("Provision", ack)
	check("provision accept", err)
	if status != http.StatusOK || len(raw) != 0 {
		log.Fatalf("EAS E2E FAILED at provision accept: status %d body %q", status, raw)
	}
	fmt.Println("ok   provision")

	// FolderSync
	fsInit := &activesync.Element{NS: nsFolderHierarchy, Name: "FolderSync"}
	fsInit.Add(nsFolderHierarchy, "SyncKey", "0")
	status, root, _, err = c.cmd("FolderSync", fsInit)
	check("foldersync init", err)
	if status != http.StatusOK || root.ChildText(nsFolderHierarchy, "Status") != "1" {
		log.Fatalf("EAS E2E FAILED at foldersync init: %d", status)
	}
	fsKey := root.ChildText(nsFolderHierarchy, "SyncKey")
	fsFull := &activesync.Element{NS: nsFolderHierarchy, Name: "FolderSync"}
	fsFull.Add(nsFolderHierarchy, "SyncKey", fsKey)
	status, root, _, err = c.cmd("FolderSync", fsFull)
	check("foldersync full", err)
	if status != http.StatusOK || root.ChildText(nsFolderHierarchy, "Status") != "1" {
		log.Fatalf("EAS E2E FAILED at foldersync full: %d", status)
	}
	changes := root.Child(nsFolderHierarchy, "Changes")
	folderCount := 0
	if changes != nil {
		folderCount = len(changes.ChildrenNamed(nsFolderHierarchy, "Add"))
	}
	fmt.Printf("ok   foldersync (%d folders)\n", folderCount)

	// Settings / user information
	settings := &activesync.Element{NS: nsSettings, Name: "Settings"}
	settings.Add(nsSettings, "UserInformation", "")
	status, root, _, err = c.cmd("Settings", settings)
	check("settings", err)
	if status != http.StatusOK {
		log.Fatalf("EAS E2E FAILED at settings: %d", status)
	}
	fmt.Println("ok   settings")

	// Sync inbox
	syncInit := &activesync.Element{NS: nsAirSync, Name: "Sync"}
	cols := syncInit.Add(nsAirSync, "Collections", "")
	col := cols.Add(nsAirSync, "Collection", "")
	col.Add(nsAirSync, "Class", "Email")
	col.Add(nsAirSync, "SyncKey", "0")
	col.Add(nsAirSync, "CollectionId", "inbox")
	col.Add(nsAirSync, "GetChanges", "1")
	status, root, _, err = c.cmd("Sync", syncInit)
	check("sync init", err)
	if status != http.StatusOK {
		log.Fatalf("EAS E2E FAILED at sync init: %d", status)
	}
	syncKey := root.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection").ChildText(nsAirSync, "SyncKey")
	if syncKey == "" {
		log.Fatalf("EAS E2E FAILED at sync init: no sync key: %s", root.String())
	}
	syncFull := &activesync.Element{NS: nsAirSync, Name: "Sync"}
	cols2 := syncFull.Add(nsAirSync, "Collections", "")
	col2 := cols2.Add(nsAirSync, "Collection", "")
	col2.Add(nsAirSync, "Class", "Email")
	col2.Add(nsAirSync, "SyncKey", syncKey)
	col2.Add(nsAirSync, "CollectionId", "inbox")
	col2.Add(nsAirSync, "GetChanges", "1")
	status, root, _, err = c.cmd("Sync", syncFull)
	check("sync full", err)
	if status != http.StatusOK || root.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection").ChildText(nsAirSync, "Status") != "1" {
		log.Fatalf("EAS E2E FAILED at sync full: %d %s", status, root.String())
	}
	fmt.Println("ok   sync inbox")

	// Ping (short heartbeat)
	ping := &activesync.Element{NS: nsPing, Name: "Ping"}
	ping.Add(nsPing, "HeartbeatInterval", "5")
	folders := ping.Add(nsPing, "Folders", "")
	f := folders.Add(nsPing, "Folder", "")
	f.Add(nsPing, "Id", "inbox")
	f.Add(nsPing, "Class", "Email")
	status, root, _, err = c.cmd("Ping", ping)
	check("ping", err)
	if status != http.StatusOK {
		log.Fatalf("EAS E2E FAILED at ping: %d", status)
	}
	fmt.Printf("ok   ping (status %s)\n", root.ChildText(nsPing, "Status"))

	// SendMail
	mime := "From: " + *email + "\r\nTo: " + *email + "\r\nSubject: EAS E2E smoke\r\nDate: " +
		time.Now().Format(time.RFC1123Z) + "\r\n\r\nHello from the ActiveSync E2E test."
	send := &activesync.Element{NS: nsComposeMail, Name: "SendMail"}
	send.Add(nsComposeMail, "ClientId", "e2e-1")
	send.Add(nsComposeMail, "SaveInSentItems", "1")
	send.Add(nsComposeMail, "Mime", base64.StdEncoding.EncodeToString([]byte(mime)))
	status, root, _, err = c.cmd("SendMail", send)
	check("sendmail", err)
	if status != http.StatusOK || root.ChildText(nsComposeMail, "Status") != "1" {
		log.Fatalf("EAS E2E FAILED at sendmail: %d %s", status, root.String())
	}
	fmt.Println("ok   sendmail")

	// Verify the sent message is visible through the EAS sync of Sent Items.
	sentInit := &activesync.Element{NS: nsAirSync, Name: "Sync"}
	sCols := sentInit.Add(nsAirSync, "Collections", "")
	sCol := sCols.Add(nsAirSync, "Collection", "")
	sCol.Add(nsAirSync, "Class", "Email")
	sCol.Add(nsAirSync, "SyncKey", "0")
	sCol.Add(nsAirSync, "CollectionId", "sent")
	sCol.Add(nsAirSync, "GetChanges", "1")
	status, root, _, err = c.cmd("Sync", sentInit)
	check("sync sent init", err)
	sentKey := root.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection").ChildText(nsAirSync, "SyncKey")
	sentFull := &activesync.Element{NS: nsAirSync, Name: "Sync"}
	sCols2 := sentFull.Add(nsAirSync, "Collections", "")
	sCol2 := sCols2.Add(nsAirSync, "Collection", "")
	sCol2.Add(nsAirSync, "Class", "Email")
	sCol2.Add(nsAirSync, "SyncKey", sentKey)
	sCol2.Add(nsAirSync, "CollectionId", "sent")
	sCol2.Add(nsAirSync, "GetChanges", "1")
	status, root, _, err = c.cmd("Sync", sentFull)
	check("sync sent full", err)
	sentAdds := root.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection").
		Child(nsAirSync, "Commands").ChildrenNamed(nsAirSync, "Add")
	found := false
	for _, a := range sentAdds {
		app := a.Child(nsAirSync, "ApplicationData")
		if app != nil && app.ChildText("Email", "Subject") == "EAS E2E smoke" {
			found = true
			break
		}
	}
	if !found {
		log.Fatalf("EAS E2E FAILED: sent message not visible in Sent sync: %s", root.String())
	}
	fmt.Println("ok   sent delivery verified")

	// Calendar collection sync (EAS calendar over the built-in store).
	calInit := &activesync.Element{NS: nsAirSync, Name: "Sync"}
	cCols := calInit.Add(nsAirSync, "Collections", "")
	cCol := cCols.Add(nsAirSync, "Collection", "")
	cCol.Add(nsAirSync, "Class", "Calendar")
	cCol.Add(nsAirSync, "SyncKey", "0")
	cCol.Add(nsAirSync, "CollectionId", "calendar")
	cCol.Add(nsAirSync, "GetChanges", "1")
	status, root, _, err = c.cmd("Sync", calInit)
	check("sync calendar init", err)
	calKey := root.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection").ChildText(nsAirSync, "SyncKey")
	calFull := &activesync.Element{NS: nsAirSync, Name: "Sync"}
	cCols2 := calFull.Add(nsAirSync, "Collections", "")
	cCol2 := cCols2.Add(nsAirSync, "Collection", "")
	cCol2.Add(nsAirSync, "Class", "Calendar")
	cCol2.Add(nsAirSync, "SyncKey", calKey)
	cCol2.Add(nsAirSync, "CollectionId", "calendar")
	cCol2.Add(nsAirSync, "GetChanges", "1")
	status, root, _, err = c.cmd("Sync", calFull)
	check("sync calendar full", err)
	calOut := root.Child(nsAirSync, "Collections").Child(nsAirSync, "Collection")
	if calOut == nil || calOut.ChildText(nsAirSync, "Status") != "1" {
		log.Fatalf("EAS E2E FAILED at calendar sync: %s", root.String())
	}
	fmt.Println("ok   calendar sync")

	fmt.Println("EAS E2E PASSED")
}
