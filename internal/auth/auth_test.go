package auth

import (
	"context"
	"testing"

	"github.com/jeffWelling/commentarr/internal/db"
)

func newRepo(t *testing.T) *Repo {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return NewRepo(d)
}

func TestHashPassword_Verify(t *testing.T) {
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(hash, "hunter2") {
		t.Fatal("expected correct password to verify")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("wrong password should not verify")
	}
}

func TestRepo_SaveAdminAndLookup(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	hash, _ := HashPassword("pw")
	if err := r.SaveAdmin(ctx, "admin", hash); err != nil {
		t.Fatalf("SaveAdmin: %v", err)
	}
	admin, err := r.Admin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if admin.Username != "admin" || !VerifyPassword(admin.PasswordHash, "pw") {
		t.Fatalf("round-trip mismatch: %+v", admin)
	}
}

func TestRepo_SaveAdmin_Updates(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	hash1, _ := HashPassword("old")
	hash2, _ := HashPassword("new")
	_ = r.SaveAdmin(ctx, "admin", hash1)
	_ = r.SaveAdmin(ctx, "admin", hash2)
	admin, _ := r.Admin(ctx)
	if !VerifyPassword(admin.PasswordHash, "new") {
		t.Fatalf("SaveAdmin should overwrite; hash still matches old pw")
	}
}

func TestRepo_APIKeys_GenerateValidateRotate(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()

	key, err := r.GenerateAPIKey(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}
	if len(key) < 20 {
		t.Fatalf("generated key too short: %q", key)
	}
	if !r.ValidateAPIKey(ctx, key) {
		t.Fatal("freshly-generated key should validate")
	}
	if r.ValidateAPIKey(ctx, "not-a-real-key") {
		t.Fatal("bogus key should not validate")
	}

	key2, _ := r.GenerateAPIKey(ctx, "rotated")
	if !r.ValidateAPIKey(ctx, key) || !r.ValidateAPIKey(ctx, key2) {
		t.Fatal("both keys should validate before revoke")
	}
	if err := r.RevokeAPIKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	if r.ValidateAPIKey(ctx, key) {
		t.Fatal("revoked key should not validate")
	}
	if !r.ValidateAPIKey(ctx, key2) {
		t.Fatal("non-revoked key should still validate")
	}
}

func TestRepo_ListAPIKeys(t *testing.T) {
	r := newRepo(t)
	ctx := context.Background()
	_, _ = r.GenerateAPIKey(ctx, "k1")
	_, _ = r.GenerateAPIKey(ctx, "k2")
	keys, err := r.ListAPIKeys(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2, got %d", len(keys))
	}
}
