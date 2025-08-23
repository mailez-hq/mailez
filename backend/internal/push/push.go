package push

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/SherClockHolmes/webpush-go"
	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
)

// EnsureVAPID returns the application-server VAPID key pair, generating and
// persisting it on first use.
func EnsureVAPID(db *gorm.DB) (*models.VapidKey, error) {
	var k models.VapidKey
	if err := db.First(&k).Error; err == nil {
		return &k, nil
	}
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return nil, fmt.Errorf("generate vapid: %w", err)
	}
	k = models.VapidKey{PublicKey: pub, PrivateKey: priv}
	if err := db.Create(&k).Error; err != nil {
		return nil, fmt.Errorf("persist vapid: %w", err)
	}
	return &k, nil
}

// Notify sends a push notification to every subscription of the user and
// prunes endpoints the push service reports as gone (404/410).
func Notify(db *gorm.DB, key *models.VapidKey, userEmail, title, body, url, domain string) error {
	var subs []models.PushSubscription
	if err := db.Where("user_email = ?", userEmail).Find(&subs).Error; err != nil {
		return err
	}
	if len(subs) == 0 {
		return nil
	}
	payload, _ := json.Marshal(map[string]string{
		"title": title, "body": body, "url": url, "tag": "mail",
	})
	opts := &webpush.Options{
		Subscriber:      "mailto:postmaster@" + domain,
		TTL:             60,
		VAPIDPublicKey:  key.PublicKey,
		VAPIDPrivateKey: key.PrivateKey,
	}
	var firstErr error
	for _, s := range subs {
		sub := &webpush.Subscription{
			Endpoint: s.Endpoint,
			Keys:     webpush.Keys{Auth: s.Auth, P256dh: s.P256DH},
		}
		resp, err := webpush.SendNotification(payload, sub, opts)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if resp.StatusCode == 404 || resp.StatusCode == 410 {
			if err := db.Delete(&models.PushSubscription{}, "id = ?", s.ID).Error; err != nil {
				log.Printf("push: prune subscription %d: %v", s.ID, err)
			}
		}
		_ = resp.Body.Close()
	}
	return firstErr
}
