package push

import (
	"sync"
	"testing"
	"time"

	"mailez/backend/internal/mail"
)

// fakeStater returns canned FolderStat counters keyed by "email/folder".
type fakeStater struct {
	mu   sync.Mutex
	stat map[string]mail.FolderStat
	err  error
}

func (f *fakeStater) FolderStat(email, token, folder string) (mail.FolderStat, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return mail.FolderStat{}, f.err
	}
	return f.stat[email+"/"+folder], nil
}

func (f *fakeStater) set(email, folder string, st mail.FolderStat) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stat[email+"/"+folder] = st
}

func TestHubSubscribePublishUnsubscribe(t *testing.T) {
	h := NewHub()
	ch, unsub := h.Subscribe("a@example.com", "tok-1")
	if got := h.Token("a@example.com"); got != "tok-1" {
		t.Fatalf("token = %q, want tok-1", got)
	}
	if got := h.Emails(); len(got) != 1 || got[0] != "a@example.com" {
		t.Fatalf("emails = %v", got)
	}

	h.Publish("a@example.com", MailEvent{Type: "mail", Folders: []string{"Inbox"}})
	select {
	case ev := <-ch:
		if ev.Type != "mail" || len(ev.Folders) != 1 || ev.Folders[0] != "Inbox" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("expected event on channel")
	}

	unsub()
	if got := h.Emails(); len(got) != 0 {
		t.Fatalf("emails after unsubscribe = %v, want empty", got)
	}
	// Publishing after unsubscribe must not block or panic.
	h.Publish("a@example.com", MailEvent{Type: "mail"})
}

func TestHubMultipleTabsFanOut(t *testing.T) {
	h := NewHub()
	ch1, unsub1 := h.Subscribe("a@example.com", "tok")
	ch2, unsub2 := h.Subscribe("a@example.com", "tok")
	defer unsub1()
	defer unsub2()

	h.Publish("a@example.com", MailEvent{Type: "mail"})
	for i, ch := range []<-chan MailEvent{ch1, ch2} {
		select {
		case <-ch:
		case <-time.After(time.Second):
			t.Fatalf("tab %d did not receive event", i)
		}
	}
	// Emails lists the user once even with two tabs.
	if got := h.Emails(); len(got) != 1 {
		t.Fatalf("emails = %v, want a single user", got)
	}
	// Closing one tab keeps the user subscribed.
	unsub1()
	if got := h.Emails(); len(got) != 1 {
		t.Fatalf("emails after one tab close = %v", got)
	}
}

func TestEventWatcherDetectsNewMail(t *testing.T) {
	hub := NewHub()
	ch, unsub := hub.Subscribe("a@example.com", "tok")
	defer unsub()

	fake := &fakeStater{stat: map[string]mail.FolderStat{}}
	fake.set("a@example.com", "Inbox", mail.FolderStat{UidNext: 1, Messages: 1, Unseen: 1})
	fake.set("a@example.com", "Junk", mail.FolderStat{UidNext: 1, Messages: 0, Unseen: 0})

	w := &EventWatcher{Mail: fake, Hub: hub, baseline: map[string]map[string]mail.FolderStat{}}

	// First poll only establishes the baseline.
	w.pollOnce()
	select {
	case ev := <-ch:
		t.Fatalf("baseline poll must not publish, got %+v", ev)
	default:
	}

	// New mail advances UIDNEXT and the message count.
	fake.set("a@example.com", "Inbox", mail.FolderStat{UidNext: 2, Messages: 2, Unseen: 2})
	w.pollOnce()
	select {
	case ev := <-ch:
		if len(ev.Folders) != 1 || ev.Folders[0] != "Inbox" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("expected mail event")
	}

	// Flag-only changes (reading a message) must not fire an event.
	fake.set("a@example.com", "Inbox", mail.FolderStat{UidNext: 2, Messages: 2, Unseen: 1})
	w.pollOnce()
	select {
	case ev := <-ch:
		t.Fatalf("flag change must not publish, got %+v", ev)
	default:
	}

	// A snooze wake-up resurfaces an old message as unread: UNSEEN grows
	// without UIDNEXT or count changing, and that must fire so the client
	// refreshes (engine sweeper → delivery receipt → Kick path).
	fake.set("a@example.com", "Inbox", mail.FolderStat{UidNext: 2, Messages: 2, Unseen: 2})
	w.pollOnce()
	select {
	case ev := <-ch:
		if len(ev.Folders) != 1 || ev.Folders[0] != "Inbox" {
			t.Fatalf("unexpected wake event: %+v", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("expected mail event on unseen growth (snooze wake)")
	}
}

func TestEventWatcherToleratesErrorsAndPrunes(t *testing.T) {
	hub := NewHub()
	ch, unsub := hub.Subscribe("a@example.com", "tok")
	fake := &fakeStater{
		err:  nil,
		stat: map[string]mail.FolderStat{"a@example.com/Inbox": {UidNext: 5, Messages: 5}},
	}
	w := &EventWatcher{Mail: fake, Hub: hub, baseline: map[string]map[string]mail.FolderStat{}}
	w.pollOnce() // baseline

	// Transient error: keep the old baseline, no event.
	fake.err = errStat
	w.pollOnce()
	select {
	case ev := <-ch:
		t.Fatalf("error tick must not publish, got %+v", ev)
	default:
	}

	// Recovery: growth after the error still fires because the baseline was kept.
	fake.err = nil
	fake.stat["a@example.com/Inbox"] = mail.FolderStat{UidNext: 6, Messages: 6}
	w.pollOnce()
	select {
	case <-ch:
	default:
		t.Fatal("expected event after error recovery")
	}

	unsub()
	// Disconnected user's baseline is pruned.
	w.pollOnce()
	if _, ok := w.baseline["a@example.com"]; ok {
		t.Fatal("baseline must be pruned after disconnect")
	}
}

var errStat = &fakeStatError{}

type fakeStatError struct{}

func (*fakeStatError) Error() string { return "imap stat failed" }

// fakeIdleWatcher records IdleWatch starts; each round blocks until stop
// closes, mirroring the real client so tests can observe the lifecycle.
type fakeIdleWatcher struct {
	mu       sync.Mutex
	starts   []string
	active   map[string]bool
	notified chan string
}

func newFakeIdleWatcher() *fakeIdleWatcher {
	return &fakeIdleWatcher{active: map[string]bool{}, notified: make(chan string, 8)}
}

func (f *fakeIdleWatcher) IdleWatch(email, token, folder string, stop <-chan struct{}, wake func()) {
	f.mu.Lock()
	f.starts = append(f.starts, email+"|"+token+"|"+folder)
	f.active[email] = true
	f.mu.Unlock()
	f.notified <- email
	<-stop
	f.mu.Lock()
	delete(f.active, email)
	f.mu.Unlock()
}

func (f *fakeIdleWatcher) isActive(email string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.active[email]
}

// IDLE push: the hub lifecycle must start a supervisor for the first tab,
// keep it for concurrent tabs, and tear it down with the last one.
func TestEventWatcherIdleLifecycle(t *testing.T) {
	hub := NewHub()
	fake := newFakeIdleWatcher()
	w := &EventWatcher{
		Mail:      &fakeStater{stat: map[string]mail.FolderStat{}},
		Hub:       hub,
		Idle:      fake,
		baseline:  map[string]map[string]mail.FolderStat{},
		idleStops: map[string]chan struct{}{},
	}
	hub.SetLifecycle(w.startIdleWatch, w.stopIdleWatch)

	_, unsub1 := hub.Subscribe("a@example.com", "tok-1")
	select {
	case got := <-fake.notified:
		if got != "a@example.com" {
			t.Fatalf("idle started for %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("IDLE supervisor did not start on first subscribe")
	}
	if tok := hub.Token("a@example.com"); tok != "tok-1" {
		t.Fatalf("supervisor started before token was stored: %q", tok)
	}

	// Second tab: no duplicate supervisor.
	_, unsub2 := hub.Subscribe("a@example.com", "tok-1")
	select {
	case <-fake.notified:
		t.Fatal("second subscribe must not spawn a second supervisor")
	case <-time.After(100 * time.Millisecond):
	}

	// First tab closes; the supervisor must survive for the second.
	unsub1()
	if !fake.isActive("a@example.com") {
		t.Fatal("supervisor stopped while another tab is open")
	}

	// Last tab closes: the supervisor stops.
	unsub2()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if !fake.isActive("a@example.com") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("IDLE supervisor still active after last disconnect")
}

// The IDLE supervisor must not spawn without a watcher or without a stored
// worker token (defensive against hub/lifecycle races).
func TestEventWatcherIdleNilWatcherAndNoToken(t *testing.T) {
	hub := NewHub()
	w := &EventWatcher{Hub: hub, baseline: map[string]map[string]mail.FolderStat{}}
	w.startIdleWatch("nobody@example.com") // Idle == nil: must be a no-op
	w.idleMu.Lock()
	if len(w.idleStops) != 0 {
		t.Fatal("nil watcher must not register a supervisor")
	}
	w.idleMu.Unlock()

	// Hub without lifecycle (plain NewHub) is unaffected.
	ch, unsub := hub.Subscribe("x@example.com", "t")
	_ = ch
	unsub()
}
