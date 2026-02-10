package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openAuthTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "auth-store.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})
	return st, context.Background()
}

func TestAuthUserAndSessionLifecycle(t *testing.T) {
	st, ctx := openAuthTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	count, err := st.CountEnabledUsers(ctx)
	if err != nil {
		t.Fatalf("count enabled users: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 users, got %d", count)
	}

	created, err := st.CreateAdminUser(ctx, "Admin", "hash-1", now)
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	if created == nil {
		t.Fatal("expected created user")
	}
	if created.Username != "admin" {
		t.Fatalf("expected normalized username admin, got %q", created.Username)
	}
	if created.Role != authUserRoleAdmin {
		t.Fatalf("expected role %q, got %q", authUserRoleAdmin, created.Role)
	}

	count, err = st.CountEnabledUsers(ctx)
	if err != nil {
		t.Fatalf("count enabled users after create: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}

	loaded, err := st.GetUserByUsername(ctx, "ADMIN")
	if err != nil {
		t.Fatalf("get user by username: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected loaded user")
	}
	if loaded.ID != created.ID {
		t.Fatalf("expected loaded id %q, got %q", created.ID, loaded.ID)
	}

	expiresAt := now.Add(2 * time.Hour)
	if err := st.CreateSession(ctx, created.ID, "token-hash", expiresAt, now); err != nil {
		t.Fatalf("create session: %v", err)
	}

	authed, err := st.GetUserBySessionTokenHash(ctx, "token-hash", now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("get user by session token hash: %v", err)
	}
	if authed == nil {
		t.Fatal("expected authenticated user from session")
	}
	if authed.ID != created.ID {
		t.Fatalf("expected session user id %q, got %q", created.ID, authed.ID)
	}

	if err := st.RevokeSessionByTokenHash(ctx, "token-hash", now.Add(time.Hour)); err != nil {
		t.Fatalf("revoke session by token hash: %v", err)
	}

	authed, err = st.GetUserBySessionTokenHash(ctx, "token-hash", now.Add(90*time.Minute))
	if err != nil {
		t.Fatalf("get user by revoked session token hash: %v", err)
	}
	if authed != nil {
		t.Fatal("expected nil user for revoked session")
	}
}

func TestAuthUserManagementLifecycle(t *testing.T) {
	st, ctx := openAuthTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	if _, err := st.CreateAdminUser(ctx, "alice", "hash-a", now); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if _, err := st.CreateAdminUser(ctx, "bob", "hash-b", now); err != nil {
		t.Fatalf("create bob: %v", err)
	}

	users, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Username != "alice" || users[1].Username != "bob" {
		t.Fatalf("expected usernames [alice bob], got [%s %s]", users[0].Username, users[1].Username)
	}

	disabled, err := st.SetUserDisabled(ctx, "alice", true, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("disable alice: %v", err)
	}
	if disabled == nil || !disabled.Disabled {
		t.Fatal("expected alice to be disabled")
	}

	count, err := st.CountEnabledUsers(ctx)
	if err != nil {
		t.Fatalf("count enabled users: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 enabled user, got %d", count)
	}

	enabled, err := st.SetUserDisabled(ctx, "alice", false, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("enable alice: %v", err)
	}
	if enabled == nil || enabled.Disabled {
		t.Fatal("expected alice to be enabled")
	}

	deleted, err := st.DeleteUser(ctx, "bob")
	if err != nil {
		t.Fatalf("delete bob: %v", err)
	}
	if !deleted {
		t.Fatal("expected bob to be deleted")
	}

	users, err = st.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users after delete: %v", err)
	}
	if len(users) != 1 || users[0].Username != "alice" {
		t.Fatalf("expected only alice to remain, got %+v", users)
	}
}
