package compose

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
)

// fakeMta is a minimal SMTP sink that counts completed DATA transactions.
// It refuses STARTTLS (the client tolerates a plaintext internal link) and
// accepts every envelope, so outbox delivery semantics can be observed
// end to end without the engine.
type fakeMta struct {
	ln        net.Listener
	delivered int32
}

func startFakeMta(t *testing.T) *fakeMta {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	m := &fakeMta{ln: ln}
	go m.serve()
	t.Cleanup(func() { _ = ln.Close() })
	return m
}

func (m *fakeMta) addr() string { return m.ln.Addr().String() }

func (m *fakeMta) count() int32 { return atomic.LoadInt32(&m.delivered) }

func (m *fakeMta) serve() {
	for {
		conn, err := m.ln.Accept()
		if err != nil {
			return
		}
		go m.handle(conn)
	}
}

func (m *fakeMta) handle(conn net.Conn) {
	defer conn.Close()
	fmt.Fprintf(conn, "220 fake-mta ready\r\n")
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	inData := false
	for sc.Scan() {
		line := sc.Text()
		if inData {
			if line == "." {
				inData = false
				atomic.AddInt32(&m.delivered, 1)
				fmt.Fprintf(conn, "250 ok\r\n")
			}
			continue
		}
		verb := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case verb == "DATA":
			inData = true
			fmt.Fprintf(conn, "354 go ahead\r\n")
		case verb == "STARTTLS":
			fmt.Fprintf(conn, "502 tls not supported\r\n")
		case verb == "QUIT":
			fmt.Fprintf(conn, "221 bye\r\n")
			return
		default:
			fmt.Fprintf(conn, "250 ok\r\n")
		}
	}
}

func insertDueOutbox(t *testing.T, db *gorm.DB, n int) {
	t.Helper()
	past := time.Now().UTC().Add(-time.Minute)
	for i := 0; i < n; i++ {
		row := models.Outbox{
			AccountEmail: "alice@example.com",
			AccountID:    0,
			FromAddr:     "alice@example.com",
			Subject:      "claim test",
			Recipients:   "bob@example.com",
			RawMessage:   "Subject: claim test\r\n\r\nhello\r\n",
			SendAfter:    past,
			Status:       "pending",
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("insert outbox: %v", err)
		}
	}
}

// Two replicas flushing concurrently must deliver each parked message
// exactly once: the atomic claim (pending → sending) lets only one of them
// past the gate for any given entry.
func TestOutboxClaimPreventsDoubleDelivery(t *testing.T) {
	db := newOutboxTestDB(t)
	mta := startFakeMta(t)
	const rows = 20
	insertDueOutbox(t, db, rows)

	var wg sync.WaitGroup
	for replica := 0; replica < 2; replica++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := NewOutboxWorker(db, mta.addr(), "secret", nil, nil)
			// Each replica keeps flushing like consecutive worker ticks
			// (flush processes at most Limit(10) entries per round) until
			// nothing is left pending.
			for round := 0; round < rows; round++ {
				w.flush()
				var left int64
				if err := db.Model(&models.Outbox{}).Where("status = ?", "pending").Count(&left).Error; err != nil {
					t.Error(err)
					return
				}
				if left == 0 {
					return
				}
			}
		}()
	}
	wg.Wait()

	if got := mta.count(); got != rows {
		t.Fatalf("delivered = %d, want exactly %d (no doubles)", got, rows)
	}
	var sent int64
	if err := db.Model(&models.Outbox{}).Where("status = ?", "sent").Count(&sent).Error; err != nil {
		t.Fatal(err)
	}
	if sent != rows {
		t.Fatalf("sent rows = %d, want %d", sent, rows)
	}
	var stuck int64
	if err := db.Model(&models.Outbox{}).Where("status IN ?", []string{"pending", "sending"}).Count(&stuck).Error; err != nil {
		t.Fatal(err)
	}
	if stuck != 0 {
		t.Fatalf("%d rows left in pending/sending, want 0", stuck)
	}
}

// A replica that died between claim and delivery leaves a stale `sending`
// row; a later flush must reclaim it (after the claim timeout) and deliver.
func TestOutboxReclaimsStaleClaim(t *testing.T) {
	db := newOutboxTestDB(t)
	mta := startFakeMta(t)
	stale := time.Now().UTC().Add(-30 * time.Minute)
	if err := db.Create(&models.Outbox{
		AccountEmail: "alice@example.com",
		FromAddr:     "alice@example.com",
		Recipients:   "bob@example.com",
		RawMessage:   "Subject: stale\r\n\r\nhello\r\n",
		SendAfter:    time.Now().UTC().Add(-time.Hour),
		Status:       "sending",
		ClaimedAt:    &stale,
	}).Error; err != nil {
		t.Fatal(err)
	}

	w := NewOutboxWorker(db, mta.addr(), "secret", nil, nil)
	w.flush()

	if got := mta.count(); got != 1 {
		t.Fatalf("delivered = %d, want 1", got)
	}
	var row models.Outbox
	if err := db.First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != "sent" {
		t.Fatalf("status = %q, want sent", row.Status)
	}
}
