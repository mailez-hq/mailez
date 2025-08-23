package fetch

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
)

// Fetcher polls remote POP/IMAP accounts and delivers new mail to the local
// user through the internal SMTP stack (fetchmail-style polling).
type Fetcher struct {
	DB       *gorm.DB
	SmtpAddr string // local smtp host:port for delivery
	Secret   string
	Interval time.Duration
	Insecure bool // skip TLS verification for external accounts (opt-out)
}

func New(db *gorm.DB, smtpAddr, secret string, insecure bool, interval time.Duration) *Fetcher {
	return &Fetcher{DB: db, SmtpAddr: smtpAddr, Secret: secret, Interval: interval, Insecure: insecure}
}

// Run starts the periodic poll loop.
func (f *Fetcher) Run(ctx context.Context) {
	if f.Interval <= 0 {
		return
	}
	ticker := time.NewTicker(f.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.fetchAll()
		}
	}
}

func (f *Fetcher) fetchAll() {
	var fetches []models.Fetch
	if err := f.DB.
		Joins("JOIN user ON user.email = fetch.user_email").
		Where("user.enabled = ?", true).
		Find(&fetches).Error; err != nil {
		log.Printf("fetch: list: %v", err)
		return
	}
	for i := range fetches {
		f.fetchOne(&fetches[i])
	}
}

func (f *Fetcher) fetchOne(fetch *models.Fetch) {
	password, err := crypto.Decrypt(f.Secret, fetch.Password)
	if err != nil {
		f.recordError(fetch, err)
		return
	}
	switch fetch.Protocol {
	case "pop3":
		err = f.fetchPOP3(fetch, password)
	default:
		err = f.fetchIMAP(fetch, password)
	}
	if err != nil {
		f.recordError(fetch, err)
		return
	}
	f.DB.Model(fetch).Updates(map[string]any{"last_check": time.Now(), "error": ""})
}

// markDelivered persists the per-message deduplication state right after a
// successful delivery so a later failure never re-delivers the same message.
func (f *Fetcher) markDelivered(fetch *models.Fetch, updates map[string]any) {
	if len(updates) > 0 {
		f.DB.Model(fetch).Updates(updates)
	}
}

func (f *Fetcher) recordError(fetch *models.Fetch, err error) {
	log.Printf("fetch %s@%s -> %v", fetch.Username, fetch.Host, err)
	f.DB.Model(fetch).Update("error", err.Error())
}

// fetchIMAP downloads messages newer than the stored UID cursor and delivers
// them. The cursor (not SEARCH SINCE, which only has date granularity on the
// server) is what keeps a same-day re-poll from re-delivering mail; a
// UIDVALIDITY change resets it because the server renumbered every message.
func (f *Fetcher) fetchIMAP(fetch *models.Fetch, password string) error {
	cli, err := client.Dial(net.JoinHostPort(fetch.Host, strconv.Itoa(fetch.Port)))
	if err != nil {
		return err
	}
	defer cli.Logout()
	if fetch.TLS {
		if err := cli.StartTLS(&tls.Config{InsecureSkipVerify: f.Insecure, ServerName: fetch.Host}); err != nil {
			return err
		}
	}
	if err := cli.Login(fetch.Username, password); err != nil {
		return err
	}
	mbox, err := cli.Select("INBOX", true)
	if err != nil {
		return err
	}

	lastUID := fetch.LastUID
	if fetch.UIDValidity != 0 && mbox.UidValidity != fetch.UIDValidity {
		lastUID = 0
	}
	if fetch.UIDValidity != mbox.UidValidity {
		f.markDelivered(fetch, map[string]any{"uid_validity": mbox.UidValidity})
	}

	criteria := imap.NewSearchCriteria()
	if lastUID > 0 {
		criteria.Uid = new(imap.SeqSet)
		criteria.Uid.AddRange(lastUID+1, 0) // 0 = highest existing UID
	} else {
		// First poll (or reset mailbox): bootstrap with the recent week only.
		criteria.Since = time.Now().Add(-7 * 24 * time.Hour)
	}
	uids, err := cli.Search(criteria)
	if err != nil {
		return err
	}
	for _, uid := range uids {
		if uid <= lastUID {
			continue
		}
		raw, err := f.fetchRawIMAP(cli, uid)
		if err != nil {
			return err
		}
		if err := f.deliver(fetch.UserEmail, raw); err != nil {
			return err
		}
		f.markDelivered(fetch, map[string]any{"last_uid": uid})
	}
	// Advance past anything the bootstrap window skipped so it is never
	// re-considered on later polls.
	if lastUID == 0 && mbox.UidNext > 1 {
		f.markDelivered(fetch, map[string]any{"last_uid": mbox.UidNext - 1})
	}
	return nil
}

func (f *Fetcher) fetchRawIMAP(cli *client.Client, uid uint32) (string, error) {
	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)
	section := &imap.BodySectionName{}
	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- cli.UidFetch(seqset, []imap.FetchItem{section.FetchItem()}, messages)
	}()
	var raw string
	for msg := range messages {
		if body := msg.GetBody(section); body != nil {
			buf := new(strings.Builder)
			if _, err := io.Copy(buf, body); err != nil {
				return "", err
			}
			raw = buf.String()
		}
	}
	if err := <-done; err != nil {
		return "", err
	}
	if raw == "" {
		return "", nil
	}
	return raw, nil
}

// fetchPOP3 downloads messages and optionally deletes them (Keep=false).
// In Keep mode the delivered UIDLs are remembered so re-polls never deliver
// the same remote message twice (message numbers alone are not stable).
func (f *Fetcher) fetchPOP3(fetch *models.Fetch, password string) error {
	conn, err := dialPOP3(fetch.Host, fetch.Port, fetch.TLS, f.Insecure)
	if err != nil {
		return err
	}
	defer conn.quit()
	if err := conn.auth(fetch.Username, password); err != nil {
		return err
	}
	if !fetch.Keep {
		count, err := conn.stat()
		if err != nil {
			return err
		}
		// deliver from newest to oldest so delivery order stays stable
		for n := count; n >= 1; n-- {
			raw, err := conn.retr(n)
			if err != nil {
				return err
			}
			if err := f.deliver(fetch.UserEmail, raw); err != nil {
				return err
			}
			if err := conn.dele(n); err != nil {
				return err
			}
		}
		return nil
	}

	entries, err := conn.uidl()
	if err != nil {
		return err
	}
	var seen []string
	if fetch.SeenUIDLs != "" {
		_ = json.Unmarshal([]byte(fetch.SeenUIDLs), &seen)
	}
	seenSet := make(map[string]struct{}, len(seen))
	for _, id := range seen {
		seenSet[id] = struct{}{}
	}
	for _, e := range entries {
		if _, ok := seenSet[e.UIDL]; ok {
			continue
		}
		raw, err := conn.retr(e.Num)
		if err != nil {
			return err
		}
		if err := f.deliver(fetch.UserEmail, raw); err != nil {
			return err
		}
		seen = append(seen, e.UIDL)
		seenSet[e.UIDL] = struct{}{}
		f.markDelivered(fetch, map[string]any{"seen_uidls": marshalUIDLs(seen)})
	}
	return nil
}

// marshalUIDLs serializes the delivered-UIDL list, dropping the oldest
// entries when the list outgrows its cap.
func marshalUIDLs(seen []string) string {
	const maxSeen = 10000
	if len(seen) > maxSeen {
		seen = seen[len(seen)-maxSeen:]
	}
	b, err := json.Marshal(seen)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// deliver submits a raw message to the local SMTP stack for the recipient.
func (f *Fetcher) deliver(recipient, raw string) error {
	msg := []byte(raw)
	if len(msg) == 0 {
		return nil
	}
	if !strings.HasSuffix(raw, "\r\n") {
		msg = append(msg, '\r', '\n')
	}
	return smtp.SendMail(f.SmtpAddr, nil, recipient, []string{recipient}, msg)
}
