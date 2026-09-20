package push

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"mailez/backend/internal/core"
	"mailez/backend/internal/mail"
)

// MailEvent is pushed down an SSE connection when a mailbox changes.
type MailEvent struct {
	Type    string    `json:"type"`
	Folders []string  `json:"folders,omitempty"`
	At      time.Time `json:"at"`
}

// Hub fans mailbox-change events out to the open webmail tabs of each user.
// It is intentionally small: one event channel per SSE connection, and the
// watcher checks each connected user once regardless of how many tabs they
// have open.
type Hub struct {
	mu    sync.Mutex
	users map[string]*userSubs

	// onJoin/onLeave fire when a user's first connection opens and their
	// last one closes (outside the hub lock — they may call back into the
	// hub). The SSE watcher uses them to start/stop the per-user IDLE push.
	onJoin  func(email string)
	onLeave func(email string)
}

type userSubs struct {
	token string
	subs  map[chan MailEvent]struct{}
}

// NewHub builds an empty event hub.
func NewHub() *Hub {
	return &Hub{users: make(map[string]*userSubs)}
}

// SetLifecycle installs the first-connect / last-disconnect callbacks. Call
// once at startup, before any connection.
func (h *Hub) SetLifecycle(onJoin, onLeave func(email string)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onJoin, h.onLeave = onJoin, onLeave
}

// Subscribe registers an SSE connection for email and returns its event
// channel plus an unsubscribe func. token is the background-worker mailbox
// credential the watcher uses to check the mailbox; the latest registration
// wins when several tabs are open.
func (h *Hub) Subscribe(email, token string) (<-chan MailEvent, func()) {
	ch := make(chan MailEvent, 16)
	var joined bool
	h.mu.Lock()
	u := h.users[email]
	if u == nil {
		u = &userSubs{token: token, subs: make(map[chan MailEvent]struct{})}
		h.users[email] = u
		joined = h.onJoin != nil
	} else if token != "" {
		u.token = token
	}
	u.subs[ch] = struct{}{}
	h.mu.Unlock()
	if joined {
		h.onJoin(email)
	}

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			u := h.users[email]
			if u == nil {
				h.mu.Unlock()
				return
			}
			delete(u.subs, ch)
			left := h.onLeave != nil && len(u.subs) == 0
			if len(u.subs) == 0 {
				delete(h.users, email)
			}
			h.mu.Unlock()
			if left {
				h.onLeave(email)
			}
		})
	}
	return ch, unsubscribe
}

// Publish delivers an event to every open connection of the user.
func (h *Hub) Publish(email string, ev MailEvent) {
	h.mu.Lock()
	var targets []chan MailEvent
	if u := h.users[email]; u != nil {
		for ch := range u.subs {
			targets = append(targets, ch)
		}
	}
	h.mu.Unlock()
	for _, ch := range targets {
		select {
		case ch <- ev:
		default:
			// Slow consumer: drop rather than block the watcher. The client
			// re-baselines on its next reconnect anyway.
		}
	}
}

// Emails returns the users with at least one open connection.
func (h *Hub) Emails() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.users))
	for email := range h.users {
		out = append(out, email)
	}
	return out
}

// Token returns the mailbox token of a connected user ("" when unknown).
func (h *Hub) Token(email string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if u := h.users[email]; u != nil {
		return u.token
	}
	return ""
}

// folderStater is the IMAP surface the watcher needs; *mail.Client and test
// fakes implement it.
type folderStater interface {
	// FolderVersion reports the mailbox's CONDSTORE version, and whether the
	// server reported one.
	FolderVersion(email, token, folder string) (uint64, bool, error)
}

// idleWatcher is the IDLE push surface; *mail.Client implements it.
type idleWatcher interface {
	IdleWatch(email, token, folder string, stop <-chan struct{}, wake func())
}

// watchedFolders are the mailboxes polled for new arrivals. Inbox covers
// regular delivery, Junk covers spam; other folders only change as a
// consequence of user actions already reflected by the client.
var watchedFolders = []string{"Inbox", "Junk"}

// idleFolder is the mailbox held open under IDLE for instant push. It covers
// the delivery path users actually watch; Junk and any third-party server
// without IDLE fall back to the poll tick.
const idleFolder = "Inbox"

// EventWatcher compares folder counters for users with an open SSE
// connection and publishes a MailEvent when new mail arrives. Primary
// latency comes from an IDLE connection per connected user (changes are
// pushed the moment they happen, from any client); the poll tick and the
// engine delivery receipt are the fallbacks for folders without IDLE
// (Junk) and servers without IDLE support. All three share the same
// baseline diff, deduplicated per user.
type EventWatcher struct {
	Mail     folderStater
	Hub      *Hub
	Interval time.Duration
	// Idle, when set, watches the user's Inbox over IDLE; *mail.Client.
	Idle idleWatcher

	mu       sync.Mutex
	baseline map[string]map[string]uint64
	kicks    map[string]time.Time

	idleMu    sync.Mutex
	idleStops map[string]chan struct{}
}

// NewEventWatcher builds a watcher for the given hub.
func NewEventWatcher(cfg core.Config, hub *Hub) *EventWatcher {
	mc := mail.New(cfg.MailImapAddr, "", "").SetInsecureTLS(cfg.FetchInsecure).SetForceTLS(cfg.MailForceTLS)
	w := &EventWatcher{
		Mail:      mc,
		Hub:       hub,
		Interval:  time.Duration(cfg.EventsInterval) * time.Second,
		Idle:      mc,
		baseline:  make(map[string]map[string]uint64),
		idleStops: make(map[string]chan struct{}),
	}
	hub.SetLifecycle(w.startIdleWatch, w.stopIdleWatch)
	return w
}

// startIdleWatch spawns the per-user IDLE supervisor on the user's first
// SSE connection (hub onJoin). Token lookup must happen here — the hub has
// it from Subscribe.
func (w *EventWatcher) startIdleWatch(email string) {
	// Prime the baseline as soon as the client connects. The stream sends a
	// "ready" event that makes the client re-read its folders, and without
	// this check the first poll tick (up to Interval later) would record the
	// baseline that the ready-refresh already raced against.
	go w.checkEmail(email)
	if w.Idle == nil {
		return
	}
	w.idleMu.Lock()
	if w.idleStops == nil {
		w.idleStops = make(map[string]chan struct{})
	}
	if w.idleStops[email] != nil {
		w.idleMu.Unlock()
		return
	}
	stop := make(chan struct{})
	w.idleStops[email] = stop
	w.idleMu.Unlock()

	go func() {
		defer func() {
			w.idleMu.Lock()
			if cur := w.idleStops[email]; cur == stop {
				delete(w.idleStops, email)
			}
			w.idleMu.Unlock()
		}()
		token := w.Hub.Token(email)
		if token == "" {
			return
		}
		w.Idle.IdleWatch(email, token, idleFolder, stop, func() { w.Kick(email) })
	}()
}

// stopIdleWatch tears the per-user IDLE supervisor down on the user's last
// SSE disconnect (hub onLeave).
func (w *EventWatcher) stopIdleWatch(email string) {
	w.idleMu.Lock()
	stop := w.idleStops[email]
	if stop != nil {
		delete(w.idleStops, email)
	}
	w.idleMu.Unlock()
	if stop != nil {
		close(stop)
	}
}

// Run polls connected mailboxes until ctx is cancelled.
func (w *EventWatcher) Run(ctx context.Context) {
	interval := w.Interval
	if interval <= 0 {
		interval = 20 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.pollOnce()
		}
	}
}

// pollOnce snapshots the connected users, compares their watched folder
// counters against the previous tick and publishes a mail event on growth.
func (w *EventWatcher) pollOnce() {
	emails := w.Hub.Emails()
	active := make(map[string]bool, len(emails))
	for _, email := range emails {
		active[email] = true
	}
	// Drop baselines for users that disconnected so a later reconnect starts
	// with a clean baseline instead of a stale one. Runs even when nobody is
	// connected so the map cannot leak between sessions.
	w.mu.Lock()
	for email := range w.baseline {
		if !active[email] {
			delete(w.baseline, email)
		}
	}
	w.mu.Unlock()

	if len(emails) == 0 {
		return
	}
	for _, email := range emails {
		w.checkEmail(email)
	}
}

// checkEmail diffs one connected user's watched folders against the baseline
// and publishes a mail event for the ones that changed. The poll tick, the
// IDLE wake-up and the delivery-receipt Kick share it.
//
// The mailbox version is the signal, not the counters: every delivery, flag
// change, expunge and move bumps it, so one cheap STATUS per folder catches
// the lot — including the flag-only edits (star, read) counters cannot see,
// which is what keeps other tabs in sync without waiting for the client's
// safety refresh. Reading counters instead would also cost a whole-mailbox
// walk per folder per tick.
func (w *EventWatcher) checkEmail(email string) {
	token := w.Hub.Token(email)
	if token == "" {
		return
	}
	versions := make(map[string]uint64, len(watchedFolders))
	for _, folder := range watchedFolders {
		v, ok, err := w.Mail.FolderVersion(email, token, folder)
		if err != nil {
			// Transient IMAP errors (e.g. the engine restarting) are
			// common; keep the old baseline and retry next tick.
			log.Printf("events: version %s/%s: %v", email, folder, err)
			continue
		}
		if !ok {
			// A server without CONDSTORE cannot drive push; say so once per
			// tick rather than silently never notifying.
			log.Printf("events: version %s/%s: no HIGHESTMODSEQ reported", email, folder)
			continue
		}
		versions[folder] = v
	}
	if len(versions) == 0 {
		return
	}

	var changed []string
	w.mu.Lock()
	prev, ok := w.baseline[email]
	if !ok {
		// First observation for this connection: the version just read may
		// already include a message that landed after the client's own
		// refresh, so the baseline is announced once (the client re-reads the
		// folder). Every later change goes through the diff below.
		changed = append(changed, watchedFolders...)
	} else {
		for _, folder := range watchedFolders {
			cur, haveCur := versions[folder]
			old, haveOld := prev[folder]
			if !haveCur || !haveOld {
				continue
			}
			if cur != old {
				changed = append(changed, folder)
			}
		}
	}
	w.baseline[email] = versions
	w.mu.Unlock()

	if len(changed) > 0 {
		w.Hub.Publish(email, MailEvent{Type: "mail", Folders: changed, At: time.Now()})
	}
}

// Kick rechecks one user immediately (engine delivery receipt). Coalesced
// per user so a delivery burst costs one IMAP pass; skipped entirely when
// the user has no open SSE connection (the next poll would be a no-op too).
func (w *EventWatcher) Kick(email string) {
	if w.Hub.Token(email) == "" {
		return
	}
	w.mu.Lock()
	if w.kicks == nil {
		w.kicks = make(map[string]time.Time)
	}
	if t, ok := w.kicks[email]; ok && time.Since(t) < eventsKickWindow {
		w.mu.Unlock()
		return
	}
	w.kicks[email] = time.Now()
	w.mu.Unlock()

	go w.checkEmail(email)
}

const eventsKickWindow = 2 * time.Second

// mailEvents streams mailbox-change events to the open webmail tab (SSE).
// The endpoint is authenticated like every other API route; EventSource
// carries the session cookie automatically.
//
// @Summary Mailbox change stream (SSE)
// @Tags push
// @Produce text/event-stream
// @Success 200
// @Router /events [get]
func (h *Handler) mailEvents(c *fiber.Ctx) error {
	user := currentUser(c)
	token, err := core.EnsureWorkerToken(c.UserContext(), h.DB, h.Cfg.SecretKey, user.Email)
	if err != nil {
		return core.Fail(c, 500, err, "internal error")
	}
	ch, unsubscribe := h.Events.Subscribe(user.Email, token)

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("X-Accel-Buffering", "no")
	c.Set("Connection", "keep-alive")

	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		defer unsubscribe()
		// write sends one SSE frame; it reports false when the client is gone
		// so the stream can tear down instead of spinning on a dead socket.
		write := func(event string, payload any) bool {
			b, err := json.Marshal(payload)
			if err != nil {
				return true
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b); err != nil {
				return false
			}
			return w.Flush() == nil
		}
		// The client re-baselines on (re)connect, so any event missed while
		// the connection was down is picked up immediately after reconnect.
		if !write("ready", MailEvent{Type: "ready", At: time.Now()}) {
			return
		}

		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()
		for {
			select {
			case ev := <-ch:
				if !write(ev.Type, ev) {
					return
				}
			case <-ping.C:
				// Keepalive comment + disconnect probe: a dead client fails the
				// flush, which ends the stream. (fasthttp's RequestCtx.Done()
				// must not be used here: the ctx is pooled and reset while the
				// stream goroutine is still running.)
				if _, err := w.WriteString(": ping\n\n"); err != nil || w.Flush() != nil {
					return
				}
			}
		}
	})
	return nil
}
