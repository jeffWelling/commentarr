package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Repo persists admin + API keys + sessions.
type Repo struct {
	db *sql.DB
}

// NewRepo returns a Repo.
func NewRepo(d *sql.DB) *Repo { return &Repo{db: d} }

// SaveAdmin upserts the single admin row (id=1).
func (r *Repo) SaveAdmin(ctx context.Context, username, passwordHash string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO admin_account (id, username, password_hash)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  username = excluded.username,
		  password_hash = excluded.password_hash,
		  updated_at = CURRENT_TIMESTAMP`,
		username, passwordHash)
	if err != nil {
		return fmt.Errorf("save admin: %w", err)
	}
	return nil
}

// Admin returns the admin row. sql.ErrNoRows is returned when no admin
// has been created yet (first-run state).
func (r *Repo) Admin(ctx context.Context) (Admin, error) {
	var a Admin
	err := r.db.QueryRowContext(ctx, `
		SELECT username, password_hash FROM admin_account WHERE id = 1`).
		Scan(&a.Username, &a.PasswordHash)
	if err != nil {
		return Admin{}, err
	}
	return a, nil
}

// GenerateAPIKey creates + persists a new key, returning the plaintext
// key (caller must communicate it; it's not recoverable later). Only the
// hash is stored.
func (r *Repo) GenerateAPIKey(ctx context.Context, label string) (string, error) {
	key, err := generateSecret()
	if err != nil {
		return "", err
	}
	hash := sha256Hex(key)
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO api_keys (key_hash, label) VALUES (?, ?)`, hash, label)
	if err != nil {
		return "", fmt.Errorf("save api key: %w", err)
	}
	return key, nil
}

// ValidateAPIKey returns true if key (plaintext) matches a stored hash.
// Side effect: updates last_used.
func (r *Repo) ValidateAPIKey(ctx context.Context, key string) bool {
	if key == "" {
		return false
	}
	hash := sha256Hex(key)
	var count int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys WHERE key_hash = ?`, hash).Scan(&count)
	if err != nil || count == 0 {
		return false
	}
	_, _ = r.db.ExecContext(ctx, `UPDATE api_keys SET last_used = CURRENT_TIMESTAMP WHERE key_hash = ?`, hash)
	return true
}

// RevokeAPIKey deletes a key.
func (r *Repo) RevokeAPIKey(ctx context.Context, key string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM api_keys WHERE key_hash = ?`, sha256Hex(key))
	return err
}

// APIKeyMetadata is the stored metadata for a key (we can't return the
// plaintext key — only its hash is stored).
type APIKeyMetadata struct {
	Label     string
	Hash      string
	CreatedAt time.Time
	LastUsed  *time.Time
}

// ListAPIKeys returns every recorded key.
func (r *Repo) ListAPIKeys(ctx context.Context) ([]APIKeyMetadata, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT key_hash, label, created_at, last_used FROM api_keys ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []APIKeyMetadata
	for rows.Next() {
		var m APIKeyMetadata
		var lastUsed sql.NullTime
		if err := rows.Scan(&m.Hash, &m.Label, &m.CreatedAt, &lastUsed); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			t := lastUsed.Time
			m.LastUsed = &t
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// sha256Hex returns the hex-encoded SHA-256 hash of s.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
