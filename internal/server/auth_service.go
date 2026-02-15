package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	internalauth "grns/internal/auth"
	"grns/internal/store"
)

const (
	sessionCookieName           = "grns_session"
	authTypeBearer              = "bearer"
	authTypeSession             = "session"
	defaultEnabledUsersCacheTTL = 5 * time.Second
)

var (
	defaultSessionTTL     = 24 * time.Hour
	errInvalidCredentials = errors.New("invalid credentials")
)

// AuthService encapsulates browser auth operations backed by the store.
type AuthService struct {
	store                store.AuthStore
	sessionTTL           time.Duration
	enabledUsersCacheTTL time.Duration

	mu                   sync.Mutex
	enabledUsersCached   bool
	enabledUsersValue    bool
	enabledUsersCachedAt time.Time
}

type authLoginResult struct {
	User      *store.AuthUser
	Token     string
	ExpiresAt time.Time
}

func NewAuthService(authStore store.AuthStore) *AuthService {
	if authStore == nil {
		return nil
	}
	return &AuthService{
		store:                authStore,
		sessionTTL:           defaultSessionTTL,
		enabledUsersCacheTTL: defaultEnabledUsersCacheTTL,
	}
}

func (a *AuthService) AuthRequired(ctx context.Context, apiTokenConfigured, requireAuthWithUsers bool, now time.Time) (bool, error) {
	if apiTokenConfigured {
		return true, nil
	}
	if !requireAuthWithUsers {
		return false, nil
	}
	if a == nil || a.store == nil {
		return false, nil
	}

	cached, ok := a.cachedEnabledUsers(now)
	if ok {
		return cached, nil
	}

	count, err := a.store.CountEnabledUsers(ctx)
	if err != nil {
		return false, err
	}
	value := count > 0
	a.setCachedEnabledUsers(now, value)
	return value, nil
}

func (a *AuthService) cachedEnabledUsers(now time.Time) (bool, bool) {
	if a == nil {
		return false, false
	}
	if a.enabledUsersCacheTTL <= 0 {
		return false, false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.enabledUsersCached {
		return false, false
	}
	if now.Sub(a.enabledUsersCachedAt) > a.enabledUsersCacheTTL {
		a.enabledUsersCached = false
		return false, false
	}
	return a.enabledUsersValue, true
}

func (a *AuthService) setCachedEnabledUsers(now time.Time, value bool) {
	if a == nil || a.enabledUsersCacheTTL <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.enabledUsersCached = true
	a.enabledUsersValue = value
	a.enabledUsersCachedAt = now
}

func (a *AuthService) InvalidateAuthRequiredCache() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.enabledUsersCached = false
	a.enabledUsersCachedAt = time.Time{}
	a.enabledUsersValue = false
}

func (a *AuthService) Login(ctx context.Context, username, password string, now time.Time) (*authLoginResult, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("auth store is required")
	}

	normalized, err := internalauth.NormalizeUsername(username)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("password is required")
	}

	user, err := a.store.GetUserByUsername(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if user == nil || user.Disabled || !internalauth.VerifyPassword(user.PasswordHash, password) {
		return nil, errInvalidCredentials
	}

	token, err := generateSessionToken()
	if err != nil {
		return nil, err
	}
	tokenHash := hashSessionToken(token)
	expiresAt := now.Add(a.sessionTTL)
	if err := a.store.CreateSession(ctx, user.ID, tokenHash, expiresAt, now); err != nil {
		return nil, err
	}

	return &authLoginResult{
		User:      user,
		Token:     token,
		ExpiresAt: expiresAt,
	}, nil
}

func (a *AuthService) AuthenticateSessionToken(ctx context.Context, token string, now time.Time) (*store.AuthUser, error) {
	if a == nil || a.store == nil {
		return nil, nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil
	}
	return a.store.GetUserBySessionTokenHash(ctx, hashSessionToken(token), now)
}

func (a *AuthService) RevokeSessionToken(ctx context.Context, token string, now time.Time) error {
	if a == nil || a.store == nil {
		return nil
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return a.store.RevokeSessionByTokenHash(ctx, hashSessionToken(token), now)
}

func (a *AuthService) CreateAdminUser(ctx context.Context, username, password string, now time.Time) (*store.AuthUser, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("auth store is required")
	}

	normalized, err := internalauth.NormalizeUsername(username)
	if err != nil {
		return nil, err
	}
	passwordHash, err := internalauth.HashPassword(password)
	if err != nil {
		return nil, err
	}

	created, err := a.store.CreateAdminUser(ctx, normalized, passwordHash, now)
	if err != nil {
		return nil, err
	}
	a.InvalidateAuthRequiredCache()
	return created, nil
}

func (a *AuthService) ListUsers(ctx context.Context) ([]store.AuthUser, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("auth store is required")
	}
	return a.store.ListUsers(ctx)
}

func (a *AuthService) SetUserDisabled(ctx context.Context, username string, disabled bool, now time.Time) (*store.AuthUser, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("auth store is required")
	}

	normalized, err := internalauth.NormalizeUsername(username)
	if err != nil {
		return nil, err
	}

	updated, err := a.store.SetUserDisabled(ctx, normalized, disabled, now)
	if err != nil {
		return nil, err
	}
	if updated != nil {
		a.InvalidateAuthRequiredCache()
	}
	return updated, nil
}

func (a *AuthService) DeleteUser(ctx context.Context, username string) (bool, error) {
	if a == nil || a.store == nil {
		return false, fmt.Errorf("auth store is required")
	}

	normalized, err := internalauth.NormalizeUsername(username)
	if err != nil {
		return false, err
	}

	deleted, err := a.store.DeleteUser(ctx, normalized)
	if err != nil {
		return false, err
	}
	if deleted {
		a.InvalidateAuthRequiredCache()
	}
	return deleted, nil
}

func hashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func generateSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
