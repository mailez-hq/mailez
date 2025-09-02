package mail

import (
	"log"
	"time"

	"github.com/emersion/go-imap/client"
)

// IdleWatch holds one dedicated IMAP connection open in IDLE on a folder and
// calls wake every time the server pushes a mailbox change (RFC 2177): new
// mail, expunges or flag changes from any client. It blocks until stop is
// closed; transport errors reconnect with a bounded backoff. wake calls are
// coalesced by the caller (push.EventWatcher.Kick).
//
// The go-imap v1 Idle primitive idles until its stop channel closes — it does
// not return when the server sends data. Unilateral changes are observable
// through the client's Updates channel instead, so this loop watches both:
// a change ends the round (DONE is sent, then IDLE is re-issued), giving
// sub-second notification latency while a single connection stays warm.
func (c *Client) IdleWatch(email, token, folder string, stop <-chan struct{}, wake func()) {
	backoff := time.Duration(0)
	const maxBackoff = 5 * time.Second
	for {
		select {
		case <-stop:
			return
		default:
		}
		if backoff > 0 && !sleepStop(stop, backoff) {
			return
		}
		stopped, ok := c.idleRound(email, token, folder, stop, wake)
		if stopped {
			return
		}
		if ok {
			backoff = 0
			continue
		}
		if backoff == 0 {
			backoff = 500 * time.Millisecond
		} else {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// idleRound runs one dial-select-idle cycle until a change arrives, an error
// occurs or stop closes. It reports (stopped, roundOK): stopped means stop
// was observed; roundOK means the round ended normally (a change arrived or
// the idler cycled), as opposed to a transport error.
func (c *Client) idleRound(email, token, folder string, stop <-chan struct{}, wake func()) (stopped, ok bool) {
	cli, err := c.dialIMAP(email, token)
	if err != nil {
		log.Printf("idle %s: dial: %v", email, err)
		return false, false
	}
	defer func() { _ = cli.Logout() }()
	// dialIMAP bounds every command, but IDLE is a long-lived command by
	// design: it completes only when the stop channel closes or the server
	// pushes a change, so the per-command timeout must be disabled again
	// here. Hang protection comes from the dial bound above plus the
	// bounded backoff loop.
	cli.Timeout = 0

	// Unilateral updates surface here even while IDLE is running; a buffered
	// channel keeps a burst from blocking the client's reader goroutine.
	updates := make(chan client.Update, 32)
	cli.Updates = updates

	if _, err := c.selectFolder(&pooledConn{Client: cli}, folder, true); err != nil {
		log.Printf("idle %s: select %s: %v", email, folder, err)
		return false, false
	}

	for {
		idleStop := make(chan struct{})
		done := make(chan error, 1)
		go func() { done <- cli.Idle(idleStop, nil) }()

		select {
		case <-stop:
			close(idleStop)
			<-done
			return true, true
		case <-updates:
			// Anything the server pushed (EXISTS/EXPUNGE/FLAGS) ends the
			// round; leftover buffered updates are stale — the wake diff is
			// authoritative, so dropping them is safe.
			close(idleStop)
			<-done
			wake()
		case err := <-done:
			if err != nil {
				log.Printf("idle %s: %v", email, err)
				return false, false
			}
			// The idler restarted without a change; re-issue it.
		}
	}
}

func sleepStop(stop <-chan struct{}, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-stop:
		return false
	case <-t.C:
		return true
	}
}
