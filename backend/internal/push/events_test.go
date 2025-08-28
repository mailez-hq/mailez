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
