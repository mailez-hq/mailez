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
}

type userSubs struct {
	token string
	subs  map[chan MailEvent]struct{}
}

// NewHub builds an empty event hub.
func NewHub() *Hub {
	return &Hub{users: make(map[string]*userSubs)}
}

// Subscribe registers an SSE connection for email and returns its event
// channel plus an unsubscribe func. token is the background-worker mailbox
// credential the watcher uses to check the mailbox; the latest registration
// wins when several tabs are open.
func (h *Hub) Subscribe(email, token string) (<-chan MailEvent, func()) {
	ch := make(chan MailEvent, 16)
	h.mu.Lock()
	u := h.users[email]
	if u == nil {
		u = &userSubs{token: token, subs: make(map[chan MailEvent]struct{})}
		h.users[email] = u
	} else if token != "" {
		u.token = token
	}
	u.subs[ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			u := h.users[email]
			if u == nil {
				return
			}
			delete(u.subs, ch)
			if len(u.subs) == 0 {
				delete(h.users, email)
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
	FolderStat(email, token, folder string) (mail.FolderStat, error)
}

// watchedFolders are the mailboxes polled for new arrivals. Inbox covers
// regular delivery, Junk covers spam; other folders only change as a
// consequence of user actions already reflected by the client.
var watchedFolders = []string{"Inbox", "Junk"}

// EventWatcher periodically compares folder counters for users with an open
// SSE connection and publishes a MailEvent when new mail arrives. It is the
// webmail counterpart of the ActiveSync Ping long-poll, but server-side and
// deduplicated per user: one IMAP check per connected user per tick, no
// per-tab or per-request overhead.
type EventWatcher struct {
	Mail     folderStater
	Hub      *Hub
	Interval time.Duration

	mu       sync.Mutex
	baseline map[string]map[string]mail.FolderStat
	kicks    map[string]time.Time
}

// NewEventWatcher builds a watcher for the given hub.
func NewEventWatcher(cfg core.Config, hub *Hub) *EventWatcher {
	return &EventWatcher{
		Mail:     mail.New(cfg.MailImapAddr, "", "").SetInsecureTLS(cfg.FetchInsecure),
		Hub:      hub,
		Interval: time.Duration(cfg.EventsInterval) * time.Second,
		baseline: make(map[string]map[string]mail.FolderStat),
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
// and publishes a mail event on growth. The poll tick and the delivery-receipt
// Kick share it.
func (w *EventWatcher) checkEmail(email string) {
	token := w.Hub.Token(email)
	if token == "" {
		return
	}
	stats := make(map[string]mail.FolderStat, len(watchedFolders))
	for _, folder := range watchedFolders {
		st, err := w.Mail.FolderStat(email, token, folder)
		if err != nil {
			// Transient IMAP errors (e.g. the engine restarting) are
			// common; keep the old baseline and retry next tick.
			log.Printf("events: stat %s/%s: %v", email, folder, err)
			continue
		}
		stats[folder] = st
	}
	if len(stats) == 0 {
		return
	}

	var changed []string
	w.mu.Lock()
	prev, ok := w.baseline[email]
	if ok {
		for _, folder := range watchedFolders {
			cur, haveCur := stats[folder]
			old, haveOld := prev[folder]
			if !haveCur || !haveOld {
				continue
			}
			// New mail advances UIDNEXT (and usually the message count)
			// regardless of read state; a growing UNSEEN count also fires,
			// because a snooze wake-up (engine sweeper) resurfaces an old
			// message as unread and the client must refresh then too.
			// Other flag-only changes (read/archive) don't fire.
			if cur.UidNext > old.UidNext || cur.Messages > old.Messages || cur.Unseen > old.Unseen {
				changed = append(changed, folder)
			}
		}
	}
	w.baseline[email] = stats
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
