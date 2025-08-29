// Command e2e-enterprise runs a full functional + performance regression pass
// against a running mailez enterprise stack (mailez control plane + mailezine
// engine). It exercises the webmail API surface, direct IMAP, WebDAV and the
// mail round trip, and asserts that repeated requests do not re-pay the
// expensive control-plane auth round trip (connection pool + auth cache).
//
// Usage:
//
//	go run ./cmd/e2e-enterprise -user u00001@example.com -password benchpass
package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
)

type options struct {
	host          string
	apiPort       int
	imapPort      int
	user          string
	password      string
	adminEmail    string
	adminPassword string
	perfThreshold time.Duration
	coldThreshold time.Duration
	lenient       bool
}

func main() {
	opts := options{}
	flag.StringVar(&opts.host, "host", "127.0.0.1", "stack host")
	flag.IntVar(&opts.apiPort, "api-port", 8080, "backend API port")
	flag.IntVar(&opts.imapPort, "imap-port", 3143, "engine IMAP port")
	flag.StringVar(&opts.user, "user", "u00001@example.com", "test mailbox")
	flag.StringVar(&opts.password, "password", "benchpass", "test mailbox password")
	flag.StringVar(&opts.adminEmail, "admin-email", "admin@example.com", "global admin email (optional)")
	flag.StringVar(&opts.adminPassword, "admin-password", "MailezDemo2026!", "global admin password (optional)")
	flag.DurationVar(&opts.perfThreshold, "perf-threshold", time.Second, "max latency for repeated (warm) requests")
	flag.DurationVar(&opts.coldThreshold, "cold-threshold", 15*time.Second, "max latency for the first request")
	flag.BoolVar(&opts.lenient, "lenient", false, "report non-critical failures without failing the run")
	flag.Parse()

	api := newAPIClient(fmt.Sprintf("http://%s:%d/api/v1", opts.host, opts.apiPort))
	baseURL := api.base
	var checks, passed int
	check := func(name string, ok bool, detail string) {
		checks++
		if ok {
			passed++
		}
		mark := "PASS"
		if !ok {
			mark = "FAIL"
		}
		if detail != "" {
			fmt.Printf("[%s] %s - %s\n", mark, name, detail)
		} else {
			fmt.Printf("[%s] %s\n", mark, name)
		}
	}
	failHard := func(name string, err error) {
		check(name, false, err.Error())
		os.Exit(1)
	}
	elapsed := func(fn func() error) (time.Duration, error) {
		t0 := time.Now()
		err := fn()
		return time.Since(t0), err
	}

	// 1. Health + login.
	if code, err := api.json("GET", "/health", nil, nil); err != nil || code != 200 {
		failHard("backend health", fmt.Errorf("status %d: %w", code, err))
	}
	check("backend health", true, baseURL)
	code, err := api.json("POST", "/sso/login", map[string]string{"email": opts.user, "pw": opts.password}, nil)
	if err != nil || code != 200 {
		failHard("mailbox login", fmt.Errorf("status %d: %w", code, err))
	}
	check("mailbox login", true, opts.user)

	// 2. Perf regression: repeated mailbox reads must not re-pay auth.
	var unseen map[string]int
	code, err = api.json("GET", "/mail/unseen", nil, &unseen)
	if err != nil || code != 200 {
		failHard("unseen counts", fmt.Errorf("status %d: %w", code, err))
	}
	_, inboxKnown := unseen["Inbox"]
	check("unseen counts", inboxKnown, fmt.Sprintf("%v", unseen))
	for i := 2; i <= 3; i++ {
		d, err := elapsed(func() error {
			c, e := api.json("GET", "/mail/unseen", nil, &unseen)
			if e != nil || c != 200 {
				return fmt.Errorf("status %d: %w", c, e)
			}
			return nil
		})
		if err != nil {
			failHard(fmt.Sprintf("unseen repeat #%d", i), err)
		}
		threshold := opts.perfThreshold
		if d > threshold {
			check(fmt.Sprintf("unseen repeat #%d warm", i), false, d.String())
		} else {
			check(fmt.Sprintf("unseen repeat #%d warm", i), true, d.String())
		}
	}

	// 3. Mailbox functional: folders, list, detail, star, label, move, search.
	var folders []string
	if code, err := api.json("GET", "/mail/folders", nil, &folders); err != nil || code != 200 {
		failHard("folders", fmt.Errorf("status %d: %w", code, err))
	}
	hasInbox := false
	for _, f := range folders {
		if strings.EqualFold(f, "Inbox") {
			hasInbox = true
		}
	}
	check("folders contain Inbox", hasInbox, strings.Join(folders, ","))

	var msgs []mailRow
	if code, err := api.json("GET", "/mail/messages?folder=Inbox&page=0", nil, &msgs); err != nil || code != 200 {
		failHard("list Inbox", fmt.Errorf("status %d: %w", code, err))
	}
	check("list Inbox has messages", len(msgs) > 0, fmt.Sprintf("%d messages", len(msgs)))
	var msgID string
	var uid float64
	if len(msgs) > 0 {
		msgID = msgs[0].ID
		uid = msgs[0].UID
	}
	var detail mailRow
	if code, err := api.json("GET", "/mail/message?folder=Inbox&id="+msgID, nil, &detail); err != nil || code != 200 {
		failHard("message detail", fmt.Errorf("status %d: %w", code, err))
	}
	check("message detail", detail.Subject != "", detail.Subject)

	// Sort regression: 最旧优先 (date asc) must actually reverse the list
	// instead of silently returning newest-first like 最新优先.
	var newestFirst, oldestFirst []mailRow
	if code, err := api.json("GET", "/mail/messages?folder=Inbox&page=0&sort=date&dir=desc", nil, &newestFirst); err != nil || code != 200 {
		failHard("sort date desc", fmt.Errorf("status %d: %w", code, err))
	}
	if code, err := api.json("GET", "/mail/messages?folder=Inbox&page=0&sort=date&dir=asc", nil, &oldestFirst); err != nil || code != 200 {
		failHard("sort date asc", fmt.Errorf("status %d: %w", code, err))
	}
	sortDiffers := len(newestFirst) > 0 && len(oldestFirst) > 0 && newestFirst[0].UID != oldestFirst[0].UID
	check("date asc reverses order", sortDiffers, fmt.Sprintf("desc=%v asc=%v", firstUIDs(newestFirst), firstUIDs(oldestFirst)))

	// Star the message and verify it sticks.
	if code, err := api.json("POST", "/mail/flag", map[string]any{
		"folder": "Inbox", "uid": uid, "flag": "\\Flagged", "value": true,
	}, nil); err != nil || code != 204 {
		failHard("star message", fmt.Errorf("status %d: %w", code, err))
	}
	if code, err := api.json("GET", "/mail/messages?folder=Inbox&page=0", nil, &msgs); err != nil || code != 200 {
		failHard("relist after star", fmt.Errorf("status %d: %w", code, err))
	}
	starred := false
	for _, m := range msgs {
		if m.UID == uid {
			for _, f := range m.Flags {
				if strings.EqualFold(f, "\\Flagged") {
					starred = true
				}
			}
		}
	}
	check("star persists", starred, "")
	if code, err := api.json("POST", "/mail/flag", map[string]any{
		"folder": "Inbox", "uid": uid, "flag": "\\Flagged", "value": false,
	}, nil); err != nil || code != 204 {
		failHard("unstar message", fmt.Errorf("status %d: %w", code, err))
	}

	// Label: create, attach, verify, delete.
	labelName := fmt.Sprintf("e2e-tag-%d", time.Now().Unix())
	if code, err := api.json("POST", "/mail/labels", map[string]any{"name": labelName, "color": "#ff0000"}, nil); err != nil || (code != 200 && code != 201) {
		failHard("create label", fmt.Errorf("status %d: %w", code, err))
	}
	if code, err := api.json("POST", "/mail/flag", map[string]any{
		"folder": "Inbox", "uid": uid, "flag": labelName, "value": true,
	}, nil); err != nil || code != 204 {
		failHard("attach label", fmt.Errorf("status %d: %w", code, err))
	}
	if code, err := api.json("GET", "/mail/messages?folder=Inbox&page=0", nil, &msgs); err != nil || code != 200 {
		failHard("relist after label", fmt.Errorf("status %d: %w", code, err))
	}
	labelSeen := false
	for _, m := range msgs {
		if m.UID == uid {
			for _, f := range m.Flags {
				if f == labelName {
					labelSeen = true
				}
			}
		}
	}
	check("label attached", labelSeen, "")
	var labels []struct {
		Name string `json:"name"`
	}
	if code, err := api.json("GET", "/mail/labels", nil, &labels); err != nil || code != 200 {
		failHard("list labels", fmt.Errorf("status %d: %w", code, err))
	}
	foundLabel := false
	for _, l := range labels {
		if l.Name == labelName {
			foundLabel = true
		}
	}
	check("label listed", foundLabel, "")
	if code, err := api.json("DELETE", "/mail/labels?name="+labelName, nil, nil); err != nil || code != 204 {
		failHard("delete label", fmt.Errorf("status %d: %w", code, err))
	}
	if code, err := api.json("GET", "/mail/labels", nil, &labels); err != nil || code != 200 {
		failHard("relist labels", fmt.Errorf("status %d: %w", code, err))
	}
	labelGone := true
	for _, l := range labels {
		if l.Name == labelName {
			labelGone = false
		}
	}
	check("label deleted", labelGone, "")

	// Search by subject.
	var found []mailRow
	if code, err := api.json("GET", "/mail/search?folder=Inbox&q="+urlQuery(detail.Subject), nil, &found); err != nil || code != 200 {
		failHard("search", fmt.Errorf("status %d: %w", code, err))
	}
	check("search finds message", len(found) > 0, fmt.Sprintf("%d hits", len(found)))

	// 4. Compose round trip: send to self, confirm delivery + Sent copy.
	subject := fmt.Sprintf("e2e-enterprise-%d", time.Now().Unix())
	if code, err := api.json("POST", "/mail/send", map[string]any{
		"to": []string{opts.user}, "subject": subject, "body": "e2e enterprise body",
	}, nil); err != nil || (code != 200 && code != 201 && code != 202 && code != 204) {
		failHard("compose send", fmt.Errorf("status %d: %w", code, err))
	}
	check("compose send accepted", true, subject)
	delivered := false
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := imapFindSubject(opts.host, opts.imapPort, opts.user, opts.password, "INBOX", subject); err == nil && raw {
			delivered = true
			break
		}
		time.Sleep(3 * time.Second)
	}
	check("message delivered to INBOX", delivered, "")
	var sentMsgs []mailRow
	if code, err := api.json("GET", "/mail/messages?folder=Sent&page=0", nil, &sentMsgs); err != nil || code != 200 {
		failHard("list Sent", fmt.Errorf("status %d: %w", code, err))
	}
	inSent := false
	for _, m := range sentMsgs {
		if strings.Contains(m.Subject, subject) {
			inSent = true
		}
	}
	check("sent copy in Sent", inSent, "")

	// Move the delivered message to Trash and back.
	var inboxAfter []mailRow
	if code, err := api.json("GET", "/mail/messages?folder=Inbox&page=0", nil, &inboxAfter); err != nil || code != 200 {
		failHard("relist Inbox", fmt.Errorf("status %d: %w", code, err))
	}
	var moveUID float64
	var sentThreadID string
	for _, m := range inboxAfter {
		if strings.Contains(m.Subject, subject) {
			moveUID = m.UID
			sentThreadID = m.ThreadID
		}
	}
	if moveUID > 0 {
		if code, err := api.json("POST", "/mail/move", map[string]any{
			"folder": "Inbox", "uid": moveUID, "destination": "Trash",
		}, nil); err != nil || code != 204 {
			failHard("move to Trash", fmt.Errorf("status %d: %w", code, err))
		}
		var trash []mailRow
		if code, err := api.json("GET", "/mail/messages?folder=Trash&page=0", nil, &trash); err != nil || code != 200 {
			failHard("list Trash", fmt.Errorf("status %d: %w", code, err))
		}
		inTrash := false
		var trashUID float64
		for _, m := range trash {
			if strings.Contains(m.Subject, subject) {
				inTrash = true
				trashUID = m.UID
			}
		}
		check("moved to Trash", inTrash, "")
		if trashUID > 0 {
			if code, err := api.json("POST", "/mail/move", map[string]any{
				"folder": "Trash", "uid": trashUID, "destination": "Inbox",
			}, nil); err != nil || code != 204 {
				failHard("restore to Inbox", fmt.Errorf("status %d: %w", code, err))
			}
		}
	} else {
		check("moved to Trash", false, "delivered message not found for move test")
	}

	// Conversation view: a reply to the same subject must collapse into one
	// list row (thread grouping).
	replySubject := "Re: " + subject
	if code, err := api.json("POST", "/mail/send", map[string]any{
		"to": []string{opts.user}, "subject": replySubject, "body": "conversation reply",
	}, nil); err != nil || (code != 200 && code != 201 && code != 202 && code != 204) {
		failHard("send reply for conversation", fmt.Errorf("status %d: %w", code, err))
	}
	replyDelivered := false
	deadline = time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := imapFindSubject(opts.host, opts.imapPort, opts.user, opts.password, "INBOX", replySubject); err == nil && raw {
			replyDelivered = true
			break
		}
		time.Sleep(3 * time.Second)
	}
	check("conversation reply delivered", replyDelivered, "")
	var convRows []mailRow
	if code, err := api.json("GET", "/mail/messages?folder=Inbox&page=0&conversation=1", nil, &convRows); err != nil || code != 200 {
		failHard("conversation list", fmt.Errorf("status %d: %w", code, err))
	}
	convFound := false
	for _, m := range convRows {
		if strings.Contains(m.Subject, "e2e-enterprise-") && m.ThreadCount >= 2 {
			convFound = true
		}
	}
	check("conversation grouping", convFound, "")

	// 5. Contacts CRUD.
	var contacts []struct {
		ID    uint   `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if code, err := api.json("GET", "/contacts", nil, &contacts); err != nil || code != 200 {
		failHard("list contacts", fmt.Errorf("status %d: %w", code, err))
	}
	contactEmail := "e2e+" + time.Now().Format("150405") + "@example.com"
	var created struct {
		ID uint `json:"id"`
	}
	if code, err := api.json("POST", "/contacts", map[string]any{
		"name": "E2E Contact", "email": contactEmail,
	}, &created); err != nil || (code != 200 && code != 201) {
		failHard("create contact", fmt.Errorf("status %d: %w", code, err))
	}
	if code, err := api.json("GET", "/contacts", nil, &contacts); err != nil || code != 200 {
		failHard("relist contacts", fmt.Errorf("status %d: %w", code, err))
	}
	contactSeen := false
	for _, c := range contacts {
		if c.Email == contactEmail {
			contactSeen = true
		}
	}
	check("contact created", contactSeen, "")
	if code, err := api.json("DELETE", fmt.Sprintf("/contacts/%d", created.ID), nil, nil); err != nil || (code != 200 && code != 204) {
		failHard("delete contact", fmt.Errorf("status %d: %w", code, err))
	}

	// 6. Calendar CRUD.
	summary := "e2e event " + time.Now().Format("150405")
	var ev struct {
		ID uint `json:"id"`
	}
	start := time.Now().Add(time.Hour).Format(time.RFC3339)
	if code, err := api.json("POST", "/calendar/events", map[string]any{
		"summary": summary, "start": start, "end": time.Now().Add(2 * time.Hour).Format(time.RFC3339),
	}, &ev); err != nil || (code != 200 && code != 201) {
		failHard("create calendar event", fmt.Errorf("status %d: %w", code, err))
	}
	var events []struct {
		ID      uint   `json:"id"`
		Summary string `json:"summary"`
	}
	if code, err := api.json("GET", "/calendar/events", nil, &events); err != nil || code != 200 {
		failHard("list calendar events", fmt.Errorf("status %d: %w", code, err))
	}
	evSeen := false
	for _, e := range events {
		if e.ID == ev.ID {
			evSeen = true
		}
	}
	check("calendar event created", evSeen, "")
	if ev.ID > 0 {
		if code, err := api.json("DELETE", fmt.Sprintf("/calendar/events/%d", ev.ID), nil, nil); err != nil || (code != 200 && code != 204) {
			failHard("delete calendar event", fmt.Errorf("status %d: %w", code, err))
		}
	}

	// 7. Sieve + drive surface.
	var scripts []string
	if code, err := api.json("GET", "/sieve", nil, &scripts); err != nil || code != 200 {
		failHard("sieve scripts", fmt.Errorf("status %d: %w", code, err))
	}
	check("sieve list", true, fmt.Sprintf("%d scripts", len(scripts)))
	var tree any
	if code, err := api.json("GET", "/drive/tree", nil, &tree); err != nil || code != 200 {
		check("drive tree", opts.lenient, fmt.Sprintf("status %d: %v", code, err))
	} else {
		check("drive tree", true, "")
	}

	// 8. WebDAV: Basic auth + discovery, twice (auth cache perf).
	davURL := fmt.Sprintf("http://%s:%d/dav/", opts.host, opts.apiPort)
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(opts.user+":"+opts.password))
	for i := 1; i <= 2; i++ {
		d, err := elapsed(func() error {
			code, body, e := davPropfind(davURL, basic)
			if e != nil || code != 207 || !strings.Contains(body, "current-user-principal") {
				return fmt.Errorf("status %d: %w", code, e)
			}
			return nil
		})
		if err != nil {
			failHard(fmt.Sprintf("webdav propfind #%d", i), err)
		}
		threshold := opts.perfThreshold
		if i == 1 {
			threshold = opts.coldThreshold
		}
		check(fmt.Sprintf("webdav propfind #%d", i), d <= threshold, d.String())
	}

	// 9. Direct IMAP logins: the second login must hit the engine auth cache.
	for i := 1; i <= 2; i++ {
		d, err := elapsed(func() error {
			return imapLogin(opts.host, opts.imapPort, opts.user, opts.password)
		})
		if err != nil {
			failHard(fmt.Sprintf("imap login #%d", i), err)
		}
		threshold := opts.perfThreshold
		if i == 1 {
			threshold = opts.coldThreshold
		}
		check(fmt.Sprintf("imap login #%d", i), d <= threshold, d.String())
	}

	// 9b. Concurrent reads: the per-user IMAP pool must serve bursts without
	// errors (idle pool of 2 plus overflow dials).
	{
		type result struct {
			code int
			err  error
		}
		const workers = 16
		ch := make(chan result, workers)
		for i := 0; i < workers; i++ {
			go func() {
				req, err := http.NewRequest("GET", baseURL+"/mail/messages?folder=Inbox&page=0", nil)
				if err != nil {
					ch <- result{0, err}
					return
				}
				resp, err := api.http.Do(req)
				if err != nil {
					ch <- result{0, err}
					return
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				ch <- result{resp.StatusCode, nil}
			}()
		}
		okCount := 0
		for i := 0; i < workers; i++ {
			r := <-ch
			if r.err == nil && r.code == 200 {
				okCount++
			}
		}
		check(fmt.Sprintf("concurrent reads (%d)", workers), okCount == workers, fmt.Sprintf("%d/%d ok", okCount, workers))
	}

	// 9c. Extended functional surface: profile, drafts, templates, scheduled
	// send, snooze, thread, app tokens, drive upload/download.
	var me struct {
		Email string `json:"email"`
	}
	if code, err := api.json("GET", "/me", nil, &me); err != nil || code != 200 {
		failHard("me profile", fmt.Errorf("status %d: %w", code, err))
	}
	check("me profile", strings.EqualFold(me.Email, opts.user), me.Email)

	var draftResp struct {
		UID float64 `json:"uid"`
	}
	draftSubject := "e2e draft " + time.Now().Format("150405")
	if code, err := api.json("POST", "/mail/draft", map[string]any{
		"to": []string{}, "subject": draftSubject, "text": "draft body",
	}, &draftResp); err != nil || (code != 200 && code != 201) {
		failHard("save draft", fmt.Errorf("status %d: %w", code, err))
	}
	var drafts []mailRow
	if code, err := api.json("GET", "/mail/messages?folder=Drafts&page=0", nil, &drafts); err != nil || code != 200 {
		failHard("list drafts", fmt.Errorf("status %d: %w", code, err))
	}
	draftSeen := false
	for _, d := range drafts {
		if strings.Contains(d.Subject, draftSubject) {
			draftSeen = true
		}
	}
	check("draft saved", draftSeen, "")
	if draftResp.UID > 0 {
		// Clean up: move the draft to Trash.
		if code, err := api.json("POST", "/mail/move", map[string]any{
			"folder": "Drafts", "uid": draftResp.UID, "destination": "Trash",
		}, nil); err != nil || code != 204 {
			failHard("cleanup draft", fmt.Errorf("status %d: %w", code, err))
		}
	}

	templateName := "e2e-tpl-" + time.Now().Format("150405")
	var tpl struct {
		ID uint `json:"id"`
	}
	if code, err := api.json("POST", "/mail/templates", map[string]any{
		"name": templateName, "subject": "tpl subject", "text": "tpl body",
	}, &tpl); err != nil || (code != 200 && code != 201) {
		failHard("save template", fmt.Errorf("status %d: %w", code, err))
	}
	var templates []struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
	if code, err := api.json("GET", "/mail/templates", nil, &templates); err != nil || code != 200 {
		failHard("list templates", fmt.Errorf("status %d: %w", code, err))
	}
	tplSeen := false
	for _, t := range templates {
		if t.ID == tpl.ID {
			tplSeen = true
		}
	}
	check("template saved", tplSeen, "")
	if tpl.ID > 0 {
		// Templates must be editable in place (the UI exposes edit; the
		// backend accepts id on save).
		var updated struct {
			ID      uint   `json:"id"`
			Subject string `json:"subject"`
		}
		if code, err := api.json("POST", "/mail/templates", map[string]any{
			"id": tpl.ID, "name": templateName + "-v2", "subject": "tpl subject v2", "text": "tpl body v2",
		}, &updated); err != nil || (code != 200 && code != 201) || updated.ID != tpl.ID || updated.Subject != "tpl subject v2" {
			failHard("update template", fmt.Errorf("status %d: %w id=%d subj=%q", code, err, updated.ID, updated.Subject))
		}
		check("template updated in place", updated.ID == tpl.ID, updated.Subject)
		if code, err := api.json("DELETE", fmt.Sprintf("/mail/templates/%d", tpl.ID), nil, nil); err != nil || code != 204 {
			failHard("delete template", fmt.Errorf("status %d: %w", code, err))
		}
	}

	// Aggregated accounts must be editable (create → update → delete).
	{
		accName := "e2e-acc-" + time.Now().Format("150405")
		var acc struct {
			ID uint `json:"id"`
		}
		if code, err := api.json("POST", "/accounts", map[string]any{
			"name": accName, "email": accName + "@example.com",
			"imap_host": "imap.invalid", "imap_port": 993, "imap_security": "tls",
			"username": "u", "password": "p",
		}, &acc); err != nil || (code != 200 && code != 201) || acc.ID == 0 {
			failHard("create account", fmt.Errorf("status %d: %w", code, err))
		}
		var updated struct {
			ID   uint   `json:"id"`
			Name string `json:"name"`
		}
		if code, err := api.json("PUT", fmt.Sprintf("/accounts/%d", acc.ID), map[string]any{
			"name": accName + "-v2", "email": accName + "@example.com",
			"imap_host": "imap.invalid", "imap_port": 993, "imap_security": "tls",
			"username": "u",
		}, &updated); err != nil || code != 200 || updated.ID != acc.ID || updated.Name != accName+"-v2" {
			failHard("update account", fmt.Errorf("status %d: %w", code, err))
		}
		check("account updated in place", updated.ID == acc.ID && updated.Name == accName+"-v2", updated.Name)
		if code, err := api.json("DELETE", fmt.Sprintf("/accounts/%d", acc.ID), nil, nil); err != nil || code != 204 {
			failHard("delete account", fmt.Errorf("status %d: %w", code, err))
		}
	}

	var sched struct {
		OutboxID  uint `json:"outbox_id"`
		Scheduled bool `json:"scheduled"`
	}
	schedSubject := "e2e-scheduled-" + time.Now().Format("150405")
	sendAt := time.Now().Add(30 * time.Minute).Format(time.RFC3339)
	if code, err := api.json("POST", "/mail/send", map[string]any{
		"to": []string{opts.user}, "subject": schedSubject, "body": "scheduled", "send_at": sendAt,
	}, &sched); err != nil || code != 200 || !sched.Scheduled || sched.OutboxID == 0 {
		failHard("schedule send", fmt.Errorf("status %d: %w outbox=%d", code, err, sched.OutboxID))
	}
	var outbox []struct {
		ID uint `json:"id"`
	}
	if code, err := api.json("GET", "/mail/scheduled", nil, &outbox); err != nil || code != 200 {
		failHard("list scheduled", fmt.Errorf("status %d: %w", code, err))
	}
	schedSeen := false
	for _, o := range outbox {
		if o.ID == sched.OutboxID {
			schedSeen = true
		}
	}
	check("scheduled send parked", schedSeen, "")
	if code, err := api.json("DELETE", fmt.Sprintf("/mail/outbox/%d", sched.OutboxID), nil, nil); err != nil || code != 204 {
		failHard("cancel scheduled", fmt.Errorf("status %d: %w", code, err))
	}

	var inboxRows []mailRow
	if code, err := api.json("GET", "/mail/messages?folder=Inbox&page=0", nil, &inboxRows); err != nil || code != 200 || len(inboxRows) == 0 {
		failHard("relist for snooze", fmt.Errorf("status %d: %w", code, err))
	}
	snoozeUID := inboxRows[0].UID
	until := time.Now().Add(time.Hour).Unix()
	if code, err := api.json("POST", "/mail/snooze", map[string]any{
		"folder": "Inbox", "uid": snoozeUID, "until": until,
	}, nil); err != nil || code != 204 {
		failHard("snooze message", fmt.Errorf("status %d: %w", code, err))
	}
	var snoozed []struct {
		UID float64 `json:"uid"`
	}
	if code, err := api.json("GET", "/mail/snoozed", nil, &snoozed); err != nil || code != 200 {
		failHard("list snoozed", fmt.Errorf("status %d: %w", code, err))
	}
	snoozeSeen := false
	for _, s := range snoozed {
		if s.UID == snoozeUID {
			snoozeSeen = true
		}
	}
	check("snoozed listed", snoozeSeen, "")
	if code, err := api.json("POST", "/mail/snooze", map[string]any{
		"folder": "Inbox", "uid": snoozeUID, "until": 0,
	}, nil); err != nil || code != 204 {
		failHard("unsnooze", fmt.Errorf("status %d: %w", code, err))
	}

	var threadResp struct {
		ThreadID string    `json:"thread_id"`
		Messages []mailRow `json:"messages"`
	}
	var freshInbox []mailRow
	if code, err := api.json("GET", "/mail/messages?folder=Inbox&page=0", nil, &freshInbox); err != nil || code != 200 {
		failHard("relist for thread", fmt.Errorf("status %d: %w", code, err))
	}
	threadTID := sentThreadID
	for _, m := range freshInbox {
		if strings.Contains(m.Subject, schedSubject) == false && strings.Contains(m.Subject, "e2e-enterprise") && m.ThreadID != "" {
			threadTID = m.ThreadID
			break
		}
	}
	if code, err := api.json("GET", "/mail/thread?folder=Inbox&thread_id="+threadTID, nil, &threadResp); err != nil || code != 200 {
		failHard("thread view", fmt.Errorf("status %d: %w", code, err))
	}
	check("thread view", len(threadResp.Messages) > 0, fmt.Sprintf("%d messages", len(threadResp.Messages)))

	var tok struct {
		ID    uint   `json:"id"`
		Token string `json:"token"`
	}
	if code, err := api.json("POST", "/tokens", map[string]any{}, &tok); err != nil || code != 201 || tok.Token == "" {
		failHard("create app token", fmt.Errorf("status %d: %w", code, err))
	}
	if err := imapLogin(opts.host, opts.imapPort, opts.user, tok.Token); err != nil {
		failHard("imap login with app token", err)
	}
	check("app token authenticates IMAP", true, "")
	if code, err := api.json("DELETE", fmt.Sprintf("/tokens/%d", tok.ID), nil, nil); err != nil || code != 204 {
		failHard("delete app token", fmt.Errorf("status %d: %w", code, err))
	}

	driveBody := fmt.Sprintf("e2e drive payload %d", time.Now().Unix())
	uploadID, err := api.uploadFile("/drive/upload", "file", "e2e.txt", []byte(driveBody))
	if err != nil || uploadID == 0 {
		failHard("drive upload", err)
	}
	down, err := api.download("/drive/download/" + fmt.Sprintf("%d", uploadID))
	if err != nil || !strings.Contains(string(down), driveBody) {
		failHard("drive download", err)
	}
	check("drive upload+download", true, "")

	// 10. Admin surface (best effort).
	adminAPI := newAPIClient(baseURL)
	if code, err := adminAPI.json("POST", "/sso/login", map[string]string{"email": opts.adminEmail, "pw": opts.adminPassword}, nil); err == nil && code == 200 {
		var overview any
		if code, err := adminAPI.json("GET", "/admin/overview", nil, &overview); err == nil && code == 200 {
			check("admin overview", true, "")
		} else {
			check("admin overview", opts.lenient, fmt.Sprintf("status %d: %v", code, err))
		}
	} else {
		check("admin login", opts.lenient, fmt.Sprintf("status %d: %v", code, err))
	}

	fmt.Printf("\n=== e2e-enterprise: %d/%d checks passed ===\n", passed, checks)
	if passed != checks {
		os.Exit(1)
	}
}

type mailRow struct {
	ID          string   `json:"id"`
	UID         float64  `json:"uid"`
	Subject     string   `json:"subject"`
	Flags       []string `json:"flags"`
	ThreadID    string   `json:"thread_id"`
	ThreadCount int      `json:"thread_count"`
}

type apiClient struct {
	base    string
	http    *http.Client
	cookies *cookiejar.Jar
}

func newAPIClient(base string) *apiClient {
	jar, _ := cookiejar.New(nil)
	return &apiClient{
		base:    base,
		http:    &http.Client{Timeout: 90 * time.Second, Jar: jar},
		cookies: jar,
	}
}

func (a *apiClient) json(method, path string, body, out any) (int, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.base+path, r)
	if err != nil {
		return 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if out != nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, out)
	}
	return resp.StatusCode, nil
}

// uploadFile posts one multipart file and returns the created row id.
func (a *apiClient) uploadFile(path, field, filename string, data []byte) (uint, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		return 0, err
	}
	if _, err := fw.Write(data); err != nil {
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	req, err := http.NewRequest("POST", a.base+path, &buf)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := a.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		return 0, fmt.Errorf("upload status %d: %s", resp.StatusCode, raw)
	}
	var out struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// download fetches a raw body for an authenticated endpoint.
func (a *apiClient) download(path string) ([]byte, error) {
	req, err := http.NewRequest("GET", a.base+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download status %d", resp.StatusCode)
	}
	return raw, nil
}

func davPropfind(url, basic string) (int, string, error) {
	req, err := http.NewRequest("PROPFIND", url, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Authorization", basic)
	req.Header.Set("Depth", "0")
	hc := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, string(raw), nil
}

func imapLogin(host string, port int, email, password string) error {
	addr := fmt.Sprintf("%s:%d", host, port)
	cli, err := client.Dial(addr)
	if err != nil {
		return err
	}
	_ = cli.StartTLS(&tls.Config{InsecureSkipVerify: true})
	if err := cli.Login(email, password); err != nil {
		_ = cli.Logout()
		return err
	}
	return cli.Logout()
}

func imapFindSubject(host string, port int, email, password, folder, subject string) (bool, error) {
	cli, err := client.Dial(fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return false, err
	}
	defer cli.Logout()
	_ = cli.StartTLS(&tls.Config{InsecureSkipVerify: true})
	if err := cli.Login(email, password); err != nil {
		return false, err
	}
	if _, err := cli.Select(folder, true); err != nil {
		return false, err
	}
	msgs := make(chan *imap.Message, 16)
	done := make(chan error, 1)
	seq := new(imap.SeqSet)
	seq.AddRange(1, 0)
	go func() {
		done <- cli.Fetch(seq, []imap.FetchItem{imap.FetchEnvelope}, msgs)
	}()
	for m := range msgs {
		if m.Envelope != nil && strings.Contains(m.Envelope.Subject, subject) {
			return true, nil
		}
	}
	return false, <-done
}

func urlQuery(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "%20"), "#", "%23")
}

func firstUIDs(rows []mailRow) string {
	parts := make([]string, 0, 3)
	for i, r := range rows {
		if i >= 3 {
			break
		}
		parts = append(parts, strconv.FormatFloat(r.UID, 'f', 0, 64))
	}
	return strings.Join(parts, ",")
}
