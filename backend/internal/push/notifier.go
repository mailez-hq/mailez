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
		// No browser push subscriptions, but webhook-only users still need
		// the poller to raise mail.received events.
		var hookCount int64
		if err := n.DB.Model(&models.Webhook{}).Where("enabled = ?", true).Count(&hookCount).Error; err != nil || hookCount == 0 {
			return
		}
	}
	type notifierTarget struct {
		email string
		token string
	}
	targets := map[string]notifierTarget{}
	for _, s := range subs {
		secret, err := crypto.Decrypt(n.Cfg.SecretKey, s.TokenEnc)
		if err != nil {
			log.Printf("push: decrypt token for %s: %v", s.UserEmail, err)
			continue
		}
		targets[s.UserEmail] = notifierTarget{email: s.UserEmail, token: secret}
	}
	var hooks []models.Webhook
	if err := n.DB.Where("enabled = ? AND token_enc <> ''", true).Find(&hooks).Error; err == nil {
		for _, h := range hooks {
			if _, ok := targets[h.UserEmail]; ok {
				continue
			}
			secret, derr := crypto.Decrypt(n.Cfg.SecretKey, h.TokenEnc)
			if derr != nil {
				log.Printf("push: decrypt webhook token for %s: %v", h.UserEmail, derr)
				continue
			}
			targets[h.UserEmail] = notifierTarget{email: h.UserEmail, token: secret}
		}
	}
	for _, t := range targets {
		counts, err := n.Mail.UnseenCounts(t.email, t.token)
		if err != nil {
			log.Printf("push: unseen for %s: %v", t.email, err)
			continue
		}
		total := counts["Inbox"]
		n.mu.Lock()
		prev, ok := n.last[t.email]
		n.last[t.email] = total
		n.mu.Unlock()
		if ok && total > prev {
			body := fmt.Sprintf("%d 封新邮件", total-prev)
			if err := Notify(n.DB, key, t.email, "mailez", body, "/", n.Cfg.Domain); err != nil {
				log.Printf("push: notify %s: %v", t.email, err)
			}
			DispatchWebhooks(n.DB, t.email, "mail.received", map[string]any{
				"folder":       "Inbox",
				"new_count":    total - prev,
				"unseen_total": total,
			})
		}
	}
}
