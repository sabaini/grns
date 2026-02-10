package server

import (
	"context"
	"testing"
	"time"

	"grns/internal/store"
)

func TestAuthServiceAuthRequiredCachesEnabledUsersCount(t *testing.T) {
	fake := &fakeAuthStore{enabledUsersCount: 1}
	svc := NewAuthService(fake)
	if svc == nil {
		t.Fatal("expected auth service")
	}
	svc.enabledUsersCacheTTL = time.Minute

	now := time.Now().UTC()

	required, err := svc.AuthRequired(context.Background(), false, false, now)
	if err != nil {
		t.Fatalf("auth required without strict mode: %v", err)
	}
	if required {
		t.Fatal("expected auth not required when strict user mode is disabled")
	}
	if fake.countEnabledUsersCalls != 0 {
		t.Fatalf("expected no enabled-user count query, got %d", fake.countEnabledUsersCalls)
	}

	required, err = svc.AuthRequired(context.Background(), false, true, now)
	if err != nil {
		t.Fatalf("auth required first strict query: %v", err)
	}
	if !required {
		t.Fatal("expected auth required when enabled users exist in strict user mode")
	}
	if fake.countEnabledUsersCalls != 1 {
		t.Fatalf("expected one enabled-user count query, got %d", fake.countEnabledUsersCalls)
	}

	required, err = svc.AuthRequired(context.Background(), false, true, now.Add(30*time.Second))
	if err != nil {
		t.Fatalf("auth required cached strict query: %v", err)
	}
	if !required {
		t.Fatal("expected cached auth required result")
	}
	if fake.countEnabledUsersCalls != 1 {
		t.Fatalf("expected cached call count to remain 1, got %d", fake.countEnabledUsersCalls)
	}

	required, err = svc.AuthRequired(context.Background(), false, true, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("auth required cache refresh query: %v", err)
	}
	if !required {
		t.Fatal("expected auth required after cache refresh")
	}
	if fake.countEnabledUsersCalls != 2 {
		t.Fatalf("expected cache refresh to query count again, got %d", fake.countEnabledUsersCalls)
	}

	required, err = svc.AuthRequired(context.Background(), true, true, now.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("auth required with api token: %v", err)
	}
	if !required {
		t.Fatal("expected api token mode to require auth")
	}
	if fake.countEnabledUsersCalls != 2 {
		t.Fatalf("expected api token mode to skip store query, got %d", fake.countEnabledUsersCalls)
	}
}

func TestAuthServiceInvalidateAuthRequiredCache(t *testing.T) {
	fake := &fakeAuthStore{enabledUsersCount: 1}
	svc := NewAuthService(fake)
	if svc == nil {
		t.Fatal("expected auth service")
	}
	svc.enabledUsersCacheTTL = 5 * time.Minute

	now := time.Now().UTC()
	required, err := svc.AuthRequired(context.Background(), false, true, now)
	if err != nil {
		t.Fatalf("auth required initial query: %v", err)
	}
	if !required {
		t.Fatal("expected auth required on initial query")
	}
	if fake.countEnabledUsersCalls != 1 {
		t.Fatalf("expected one enabled-user count query, got %d", fake.countEnabledUsersCalls)
	}

	fake.enabledUsersCount = 0
	required, err = svc.AuthRequired(context.Background(), false, true, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("auth required cached query: %v", err)
	}
	if !required {
		t.Fatal("expected cached value to remain true before invalidation")
	}

	svc.InvalidateAuthRequiredCache()
	required, err = svc.AuthRequired(context.Background(), false, true, now.Add(time.Minute+time.Second))
	if err != nil {
		t.Fatalf("auth required post-invalidate query: %v", err)
	}
	if required {
		t.Fatal("expected auth not required after cache invalidation and zero enabled users")
	}
	if fake.countEnabledUsersCalls != 2 {
		t.Fatalf("expected second enabled-user count query after invalidation, got %d", fake.countEnabledUsersCalls)
	}
}

type fakeAuthStore struct {
	enabledUsersCount      int
	countEnabledUsersCalls int
}

func (f *fakeAuthStore) CountEnabledUsers(context.Context) (int, error) {
	f.countEnabledUsersCalls++
	return f.enabledUsersCount, nil
}

func (f *fakeAuthStore) CreateAdminUser(context.Context, string, string, time.Time) (*store.AuthUser, error) {
	return nil, nil
}

func (f *fakeAuthStore) GetUserByUsername(context.Context, string) (*store.AuthUser, error) {
	return nil, nil
}

func (f *fakeAuthStore) GetUserByID(context.Context, string) (*store.AuthUser, error) {
	return nil, nil
}

func (f *fakeAuthStore) ListUsers(context.Context) ([]store.AuthUser, error) {
	return nil, nil
}

func (f *fakeAuthStore) SetUserDisabled(context.Context, string, bool, time.Time) (*store.AuthUser, error) {
	return nil, nil
}

func (f *fakeAuthStore) DeleteUser(context.Context, string) (bool, error) {
	return false, nil
}

func (f *fakeAuthStore) CreateSession(context.Context, string, string, time.Time, time.Time) error {
	return nil
}

func (f *fakeAuthStore) GetUserBySessionTokenHash(context.Context, string, time.Time) (*store.AuthUser, error) {
	return nil, nil
}

func (f *fakeAuthStore) RevokeSessionByTokenHash(context.Context, string, time.Time) error {
	return nil
}
