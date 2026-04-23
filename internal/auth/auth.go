// Package auth owns the single-admin + API-key authentication model
// (DESIGN.md § 5.14). Radarr-style: one admin, one or more named API
// keys (so rotation works), optional local-network bypass configured at
// the middleware layer.
package auth

import (
	"crypto/rand"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

// Admin holds the single admin account (id=1 always).
type Admin struct {
	Username     string
	PasswordHash string
}

// HashPassword returns a bcrypt hash of the given plaintext.
func HashPassword(plaintext string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// VerifyPassword reports whether hash matches plaintext.
func VerifyPassword(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}

// generateSecret returns a hex-encoded random 32-byte token suitable
// for API keys or session tokens.
func generateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
