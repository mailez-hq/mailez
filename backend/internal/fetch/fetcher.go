package fetch

import (
	"context"
	"crypto/tls"
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

	"mailez/backend/internal/crypto"
	"mailez/backend/internal/models"
)

// Fetcher polls remote POP/IMAP accounts and delivers new mail to the local
// user through the internal SMTP stack, mirroring the reference implementation's fetchmail service.
type Fetcher struct {
	DB       *gorm.DB
	SmtpAddr string // local smtp host:port for delivery
	Secret   string
	Interval time.Duration
}

func New(db *gorm.DB, smtpAddr, secret string, interval time.Duration) *Fetcher {
	return &Fetcher{DB: db, SmtpAddr: smtpAddr, Secret: secret, Interval: interval}
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

func (f *Fetcher) recordError(fetch *models.Fetch, err error) {
	log.Printf("fetch %s@%s -> %v", fetch.Username, fetch.Host, err)
	f.DB.Model(fetch).Update("error", err.Error())
}

// fetchIMAP downloads messages received since the last check and delivers them.
func (f *Fetcher) fetchIMAP(fetch *models.Fetch, password string) error {
	cli, err := client.Dial(net.JoinHostPort(fetch.Host, strconv.Itoa(fetch.Port)))
	if err != nil {
		return err
	}
	defer cli.Logout()
	if fetch.TLS {
		if err := cli.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
			return err
		}
	}
	if err := cli.Login(fetch.Username, password); err != nil {
		return err
	}
	if _, err := cli.Select("INBOX", true); err != nil {
		return err
	}
	criteria := imap.NewSearchCriteria()
	since := time.Now().Add(-7 * 24 * time.Hour)
	if fetch.LastCheck != nil {
		since = *fetch.LastCheck
	}
	criteria.Since = since
	uids, err := cli.Search(criteria)
	if err != nil {
		return err
	}
	for _, uid := range uids {
		raw, err := f.fetchRawIMAP(cli, uid)
		if err != nil {
			return err
		}
		if err := f.deliver(fetch.UserEmail, raw); err != nil {
			return err
		}
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
func (f *Fetcher) fetchPOP3(fetch *models.Fetch, password string) error {
	conn, err := dialPOP3(fetch.Host, fetch.Port, fetch.TLS)
	if err != nil {
		return err
	}
	defer conn.quit()
	if err := conn.auth(fetch.Username, password); err != nil {
		return err
	}
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
		if !fetch.Keep {
			if err := conn.dele(n); err != nil {
				return err
			}
		}
	}
	return nil
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
