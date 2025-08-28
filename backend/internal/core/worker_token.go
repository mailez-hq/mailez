package core

import (
	"context"

	"gorm.io/gorm"

	"mailez/backend/internal/core/models"
	"mailez/backend/internal/crypto"
	"mailez/backend/internal/password"
)

// EnsureWorkerToken returns the user's background-worker app token, minting
// and persisting one on first use. The plaintext secret is handed to the
// caller and must never be logged; the matching Token row (IP
// "outbox-worker") is what the mail proxy validates on IMAP login. Shared by
// the outbox queue and the SSE mailbox watcher so both can authenticate as
// the user without a live session credential.
func EnsureWorkerToken(ctx context.Context, db *gorm.DB, secretKey, email string) (string, error) {
	var row models.WorkerToken
	if err := db.WithContext(ctx).Where("user_email = ?", email).First(&row).Error; err == nil && row.TokenEnc != "" {
		if secret, derr := crypto.Decrypt(secretKey, row.TokenEnc); derr == nil {
			return secret, nil
		}
	}
	secret, err := NewAppToken()
	if err != nil {
		return "", err
	}
	hash, err := password.HashPBKDF2SHA256(secret)
	if err != nil {
		return "", err
	}
	t := models.Token{UserEmail: email, Password: hash, IP: "outbox-worker"}
	if err := db.WithContext(ctx).Create(&t).Error; err != nil {
		return "", err
	}
	enc, err := crypto.Encrypt(secretKey, secret)
	if err != nil {
		return "", err
	}
	row = models.WorkerToken{UserEmail: email, TokenEnc: enc}
	if err := db.WithContext(ctx).Save(&row).Error; err != nil {
		return "", err
	}
	return secret, nil
}
