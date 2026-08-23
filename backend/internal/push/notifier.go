package push

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/mail"
)

// Notifier polls unseen counts for users with push subscriptions and raises a
// notification when the INBOX count grows. The first poll only establishes the
// baseline so users are not spammed on restart.
type Notifier struct {
	DB   *gorm.DB
	Cfg  core.Config
	Mail *mail.Client

	mu   sync.Mutex
	last map[string]int
}

func NewNotifier(db *gorm.DB, cfg core.Config) *Notifier {
	return &Notifier{
		DB:   db,
		Cfg:  cfg,
		Mail: mail.New(cfg.MailImapAddr, "", ""),
		last: map[string]int{},
	}
}

func (n *Notifier) Run(ctx context.Context) {
	interval := time.Duration(n.Cfg.PushInterval) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n.pollOnce(ctx)
		}
	}
}

func (n *Notifier) pollOnce(ctx context.Context) {
	key, err := EnsureVAPID(n.DB)
	if err != nil {
		log.Printf("push: vapid: %v", err)
		return
	}
	var subs []models.PushSubscription
	if err := n.DB.Find(&subs).Error; err != nil || len(subs) == 0 {
		return
	}
	seen := map[string]bool{}
	for _, s := range subs {
		if seen[s.UserEmail] {
			continue
		}
		seen[s.UserEmail] = true
		secret, err := crypto.Decrypt(n.Cfg.SecretKey, s.TokenEnc)
		if err != nil {
			log.Printf("push: decrypt token for %s: %v", s.UserEmail, err)
			continue
		}
		counts, err := n.Mail.UnseenCounts(s.UserEmail, secret)
		if err != nil {
			log.Printf("push: unseen for %s: %v", s.UserEmail, err)
			continue
		}
		total := counts["Inbox"]
		n.mu.Lock()
		prev, ok := n.last[s.UserEmail]
		n.last[s.UserEmail] = total
		n.mu.Unlock()
		if ok && total > prev {
			body := fmt.Sprintf("%d 封新邮件", total-prev)
			if err := Notify(n.DB, key, s.UserEmail, "mailez", body, "/", n.Cfg.Domain); err != nil {
				log.Printf("push: notify %s: %v", s.UserEmail, err)
			}
		}
	}
}
