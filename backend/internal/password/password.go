package password

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
)

// DefaultRounds mirrors Mailu's CREDENTIAL_ROUNDS default.
const DefaultRounds = 12

// Hash hashes a password with bcrypt (rounds = DefaultRounds).
func Hash(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), DefaultRounds)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// Verify checks a password against a stored hash. It supports standard bcrypt
// ($2a$/$2b$/$2y$) and Mailu's passlib bcrypt_sha256 scheme so migrated data
// keeps working.
func Verify(stored, password string) bool {
	if stored == "" || password == "" {
		return false
	}
	if strings.HasPrefix(stored, "$bcrypt-sha256$") {
		return verifyBcryptSHA256(stored, password)
	}
	return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil
}

// verifyBcryptSHA256 verifies passlib's bcrypt_sha256 format:
//
//	$bcrypt-sha256$v=2,t=2b,r=<rounds>$<salt22>$<checksum31>
//
// passlib hashes sha256(password) as hex, then bcrypts that digest.
func verifyBcryptSHA256(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 5 {
		return false
	}
	rounds := 12
	for _, kv := range strings.Split(parts[2], ",") {
		if strings.HasPrefix(kv, "r=") {
			if n, err := strconv.Atoi(strings.TrimPrefix(kv, "r=")); err == nil {
				rounds = n
			}
		}
	}
	digest := sha256.Sum256([]byte(password))
	full := fmt.Sprintf("$2b$%d$%s%s", rounds, parts[3], parts[4])
	return bcrypt.CompareHashAndPassword([]byte(full), []byte(hex.EncodeToString(digest[:]))) == nil
}

// VerifyPBKDF2SHA256 verifies passlib's pbkdf2_sha256 scheme, used for app
// tokens ($pbkdf2-sha256$<rounds>$<salt>$<hash>, base64url without padding).
func VerifyPBKDF2SHA256(stored, password string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 5 {
		return false
	}
	rounds, err := strconv.Atoi(parts[2])
	if err != nil {
		return false
	}
	salt, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	expected, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	dk := pbkdf2.Key([]byte(password), salt, rounds, len(expected), sha256.New)
	return subtle.ConstantTimeCompare(dk, expected) == 1
}
