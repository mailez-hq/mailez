package push

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"mailez/backend/internal/cluster"
	"mailez/backend/internal/core"
	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/mail"
	"mailez/backend/internal/password"
)

// Notifier polls unseen counts for users with push subscriptions and raises a
// notification when the INBOX count grows. The first poll only establishes the
// baseline so users are not spammed on restart. Delivery receipts from the
// engine call Kick for the same check without waiting for the tick.
type Notifier struct {
	DB   *gorm.DB
	Cfg  core.Config
	Mail *mail.Client

	mu       sync.Mutex
	last     map[string]int
	repaired map[string]time.Time // last self-heal attempt per user (rate limit)
	kicks    map[string]time.Time // last delivery-receipt kick per user (coalesce)
}

func NewNotifier(db *gorm.DB, cfg core.Config) *Notifier {
	return &Notifier{
		DB:   db,
		Cfg:  cfg,
		Mail: mail.New(cfg.MailImapAddr, "", "").SetInsecureTLS(cfg.FetchInsecure),
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

// isAuthFailure reports whether an unseen-count error is an authentication
// rejection (dead credential) rather than a transient engine error.
func isAuthFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Authentication failed") ||
		strings.Contains(msg, "AUTHENTICATIONFAILED")
}

// mayRepair rate-limits self-heal attempts to once per user per hour so a
// permanently broken account (disabled user, engine down) cannot mint token
// rows in a loop.
func (n *Notifier) mayRepair(email string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.repaired == nil {
		n.repaired = map[string]time.Time{}
	}
	if t, ok := n.repaired[email]; ok && time.Since(t) < time.Hour {
		return false
	}
	n.repaired[email] = time.Now()
	return true
}

// repairNotifierToken mints a fresh push-notifier app token for the user and
// propagates the encrypted copy to every push subscription of that user, so
// the next poll cycle logs in with a live credential.
func (n *Notifier) repairNotifierToken(email string) error {
	secret, err := newAppToken()
	if err != nil {
		return err
	}
	hash, err := password.HashPBKDF2SHA256(secret)
	if err != nil {
		return err
	}
	enc, err := crypto.Encrypt(n.Cfg.SecretKey, secret)
	if err != nil {
		return err
	}
	return n.DB.Transaction(func(tx *gorm.DB) error {
		t := models.Token{UserEmail: email, Password: hash, IP: "push-notifier"}
		if err := tx.Create(&t).Error; err != nil {
			return err
		}
		return tx.Model(&models.PushSubscription{}).
			Where("user_email = ?", email).
			Updates(map[string]any{"token_enc": enc, "token_id": t.ID}).Error
	})
}

func (n *Notifier) pollOnce(ctx context.Context) {
	// Singleton via DB lease: several backend replicas may run, but only
	// the lease holder sends web pushes (otherwise every recipient device
	// would get one push per replica). A delivery-receipt kick landing on
	// a non-leader replica is simply deferred to the leader's next tick.
	if !cluster.TryHold(n.DB, "push_notifier", cluster.LeaseTTL) {
		return
	}
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
		n.checkTarget(ctx, key, t.email, t.token)
	}
}

// checkTarget runs the unseen-delta check and fan-out for one account. The
// poll loop and the delivery-receipt Kick share it so their behaviour cannot
// drift.
func (n *Notifier) checkTarget(ctx context.Context, key *models.VapidKey, email, token string) {
	counts, err := n.Mail.UnseenCounts(email, token)
	if err != nil {
		if isAuthFailure(err) {
			// Dead credential (token revoked on one side): re-mint so
			// the poll heals instead of retrying a dead secret every
			// interval. Rate-limited to one attempt per user per hour.
			if n.mayRepair(email) {
				if rerr := n.repairNotifierToken(email); rerr != nil {
					log.Printf("push: re-mint notifier token for %s: %v", email, rerr)
				} else {
					log.Printf("push: re-minted notifier token for %s after auth failure", email)
				}
			}
		}
		log.Printf("push: unseen for %s: %v", email, err)
		return
	}
	total := counts["Inbox"]
	n.mu.Lock()
	prev, ok := n.last[email]
	n.last[email] = total
	n.mu.Unlock()
	if ok && total > prev {
		body := fmt.Sprintf("%d 封新邮件", total-prev)
		if err := Notify(n.DB, key, email, "mailez", body, "/", n.Cfg.Domain); err != nil {
			log.Printf("push: notify %s: %v", email, err)
		}
		DispatchWebhooks(n.DB, email, "mail.received", map[string]any{
			"folder":       "Inbox",
			"new_count":    total - prev,
			"unseen_total": total,
		})
	}
}

// Kick checks one account immediately (engine delivery receipt). It
// coalesces within kickWindow so a burst of receipts for one account costs
// at most one IMAP round trip, and runs asynchronously: the HTTP caller gets
// its 202 without waiting on the mailbox.
func (n *Notifier) Kick(ctx context.Context, email string) {
	if email == "" {
		return
	}
	n.mu.Lock()
	if n.kicks == nil {
		n.kicks = map[string]time.Time{}
	}
	if t, ok := n.kicks[email]; ok && time.Since(t) < kickWindow {
		n.mu.Unlock()
		return
	}
	n.kicks[email] = time.Now()
	n.mu.Unlock()

	go func() {
		token := n.tokenFor(email)
		if token == "" {
			return
		}
		key, err := EnsureVAPID(n.DB)
		if err != nil {
			log.Printf("push: vapid: %v", err)
			return
		}
		n.checkTarget(ctx, key, email, token)
	}()
}

// kickWindow bounds receipt-driven rechecks for one account.
const kickWindow = 2 * time.Second

// tokenFor resolves the background mailbox credential of the account: the
// push subscription's, falling back to the webhook notifier token.
func (n *Notifier) tokenFor(email string) string {
	var sub models.PushSubscription
	if err := n.DB.Where("user_email = ?", email).First(&sub).Error; err == nil {
		if secret, err := crypto.Decrypt(n.Cfg.SecretKey, sub.TokenEnc); err == nil && secret != "" {
			return secret
		}
	}
	var hook models.Webhook
	if err := n.DB.Where("user_email = ? AND enabled = ? AND token_enc <> ''", email, true).First(&hook).Error; err == nil {
		if secret, err := crypto.Decrypt(n.Cfg.SecretKey, hook.TokenEnc); err == nil && secret != "" {
			return secret
		}
	}
	return ""
}
