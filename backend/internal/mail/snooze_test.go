package mail

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
)

// fakeIMAPMsg is one canned message in the fake IMAP mailbox.
type fakeIMAPMsg struct {
	uid   uint32
	flags []string
}

// fakeIMAPServer serves a scripted IMAP conversation over plaintext TCP for
// the wire-level regression tests in this package (same shape as
// fakeSieveServer, but stateful: the client pool reuses one connection, so
// each accepted conn carries an authenticated session with many command
// groups). Only the commands the mail client actually issues are modelled:
// LOGIN, LIST, EXAMINE/SELECT, UID SEARCH (returns the uid of every canned
// message whose flags contain $Snoozed, regardless of criteria), UID FETCH
// (FLAGS only — envelopeToMessage is nil-envelope safe) and LOGOUT.
func fakeIMAPServer(t *testing.T, folders map[string][]fakeIMAPMsg) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	folderNames := make([]string, 0, len(folders))
	for name := range folders {
		folderNames = append(folderNames, name)
	}

	serve := func(conn net.Conn) {
		defer conn.Close()
		br := bufio.NewReader(conn)
		bw := bufio.NewWriter(conn)
		w := func(s string) { _, _ = bw.WriteString(s + "\r\n"); _ = bw.Flush() }

		current := ""
		w("* OK mailez fake IMAP ready")

		for {
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.TrimRight(line, "\r\n")
			t.Logf("fake imap recv: %s", cmd)
			parts := strings.SplitN(cmd, " ", 3)
			if len(parts) < 2 {
				continue
			}
			tag, verb := parts[0], strings.ToUpper(parts[1])
			rest := ""
			if len(parts) == 3 {
				rest = parts[2]
			}
			// UID SEARCH / UID FETCH arrive as "TAG UID SEARCH ...": fold
			// the two-token verb into one so the switch below matches.
			if verb == "UID" && rest != "" {
				sub := strings.SplitN(rest, " ", 2)
				verb = "UID_" + strings.ToUpper(sub[0])
				if len(sub) == 2 {
					rest = sub[1]
				} else {
					rest = ""
				}
			}

			switch {
			case verb == "CAPABILITY" || verb == "NOOP":
				if verb == "CAPABILITY" {
					w("* CAPABILITY IMAP4rev1")
				}
				w(tag + " OK done")

			case verb == "LOGIN":
				w(tag + " OK LOGIN done")

			case verb == "STARTTLS":
				// Refuse cleanly: a "NO" keeps the connection a pristine
				// text stream. Replying OK would start a TLS handshake the
				// fake cannot perform, and the failed handshake desyncs the
				// byte stream (ClientHello bytes eaten as command lines)
				// until everything deadlocks. dialIMAP logs the refusal and
				// continues in plaintext, exactly like the dev engine.
				w(tag + " NO STARTTLS unavailable")

			case verb == "LOGOUT":
				w("* BYE fake IMAP")
				w(tag + " OK LOGOUT done")
				return

			case verb == "LIST":
				for _, name := range folderNames {
					w(fmt.Sprintf(`* LIST () "/" %q`, name))
				}
				w(tag + " OK LIST done")

			case verb == "EXAMINE" || verb == "SELECT":
				name := strings.Trim(rest, `"`)
				// The client canonicalizes "Inbox" to the protocol spelling
				// "INBOX"; match case-insensitively against the fixture.
				current = ""
				for _, f := range folderNames {
					if strings.EqualFold(f, name) {
						current = f
					}
				}
				if current == "" {
					w(tag + " NO no such mailbox")
					continue
				}
				msgs := folders[current]
				var maxUID uint32
				for _, m := range msgs {
					if m.uid > maxUID {
						maxUID = m.uid
					}
				}
				w("* FLAGS (\\Seen \\Flagged)")
				w(fmt.Sprintf("* %d EXISTS", len(msgs)))
				w("* 0 RECENT")
				w("* OK [UIDVALIDITY 1] UIDs valid")
				w(fmt.Sprintf("* OK [UIDNEXT %d] next", maxUID+1))
				w(tag + " OK [READ-ONLY] done")

			case verb == "UID_SEARCH" || verb == "SEARCH":
				msgs := folders[current]
				var hits []string
				for _, m := range msgs {
					snoozed := false
					for _, f := range m.flags {
						if f == SnoozeFlag {
							snoozed = true
						}
					}
					if snoozed {
						hits = append(hits, strconv.FormatUint(uint64(m.uid), 10))
					}
				}
				w("* SEARCH " + strings.Join(hits, " "))
				w(tag + " OK SEARCH done")

			case verb == "UID_FETCH" || verb == "FETCH":
				msgs := folders[current]
				// The set is the first token of the arguments, e.g. "5" or
				// "1:*". Serve every canned message the set covers.
				fields := strings.Fields(rest)
				if len(fields) > 0 {
					for seq, m := range msgs {
						if !uidSetCovers(fields[0], m.uid, len(msgs)) {
							continue
						}
						w(fmt.Sprintf("* %d FETCH (UID %d FLAGS (%s))", seq+1, m.uid, strings.Join(m.flags, " ")))
					}
				}
				w(tag + " OK FETCH done")

			default:
				w(tag + " OK done")
			}
		}
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serve(conn)
		}
	}()
	return ln.Addr().String()
}

// uidSetCovers reports whether a simplified IMAP uid-set token ("5", "1:3",
// "1:*", "4,5") includes the given uid. seqMax bounds "*" for range ends.
func uidSetCovers(set string, uid uint32, seqMax int) bool {
	for _, part := range strings.Split(set, ",") {
		if !strings.Contains(part, ":") {
			if n, err := strconv.ParseUint(part, 10, 32); err == nil && uint32(n) == uid {
				return true
			}
			continue
		}
		ends := strings.SplitN(part, ":", 2)
		lo, errLo := strconv.ParseUint(ends[0], 10, 32)
		hi := uint64(0)
		if ends[1] == "*" {
			hi = uint64(seqMax)
		} else if n, err := strconv.ParseUint(ends[1], 10, 32); err == nil {
			hi = uint64(n)
		} else {
			continue
		}
		if errLo == nil && uint64(uid) >= lo && uint64(uid) <= hi {
			return true
		}
	}
	return false
}

func snoozedFuture(uid uint32) fakeIMAPMsg {
	// Far-future wake-up so the listing path never triggers the resurface
	// branch (which would issue flag writes the fake does not model).
	return fakeIMAPMsg{uid: uid, flags: []string{"\\Seen", SnoozeFlag, SnoozeUntilPrefix + "9999999999"}}
}

// Regression: SnoozedMessages merges every mailbox, and IMAP uids are only
// unique per folder — every returned row must carry its source folder or the
// webmail Snoozed view collides on duplicate React keys and wakes messages in
// the wrong folder.
func TestSnoozedMessagesTagsSourceFolder(t *testing.T) {
	addr := fakeIMAPServer(t, map[string][]fakeIMAPMsg{
		"Inbox": {snoozedFuture(5)},
		"Sent":  {snoozedFuture(5)}, // same uid in a second folder
		"Trash": {snoozedFuture(7)},
	})
	c := New(addr, "", "")

	got, err := c.SnoozedMessages("amy@example.com", "token-abc")
	if err != nil {
		t.Fatalf("SnoozedMessages: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("rows = %d, want 3 (got %+v)", len(got), got)
	}
	byFolder := map[string]uint32{}
	for _, m := range got {
		if m.Folder == "" {
			t.Fatalf("row uid=%d has no source folder tag", m.UID)
		}
		byFolder[m.Folder] = m.UID
	}
	if byFolder["Inbox"] != 5 || byFolder["Sent"] != 5 || byFolder["Trash"] != 7 {
		t.Fatalf("folder→uid mapping = %v, want Inbox:5 Sent:5 Trash:7", byFolder)
	}
}

// Regression: SearchAllMessagesSpec merges every mailbox (except Trash) and
// must tag each row with its source folder for the same reason.
func TestSearchAllMessagesSpecTagsSourceFolder(t *testing.T) {
	addr := fakeIMAPServer(t, map[string][]fakeIMAPMsg{
		"Inbox": {snoozedFuture(5)},
		"Sent":  {snoozedFuture(5)},
		"Trash": {snoozedFuture(7)}, // skipped by search-all
	})
	c := New(addr, "", "")

	got, err := c.SearchAllMessagesSpec("amy@example.com", "token-abc", SearchQuery{Text: []string{"anything"}})
	if err != nil {
		t.Fatalf("SearchAllMessagesSpec: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("rows = %d, want 2 (got %+v)", len(got), got)
	}
	for _, m := range got {
		if m.Folder != "Inbox" && m.Folder != "Sent" {
			t.Fatalf("row uid=%d tagged %q, want Inbox or Sent", m.UID, m.Folder)
		}
	}
}
