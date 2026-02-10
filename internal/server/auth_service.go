package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	internalauth "grns/internal/auth"
	"grns/internal/store"
)

const (
	sessionCookieName = "grns_session"
	authTypeBearer    = "bearer"
	authTypeSession   = "session"
)

var defaultSessionTTL = 24 * time.Hour

// AuthService encapsulates browser auth operations backed by the store.
type AuthService struct {
	store      store.AuthStore
	sessionTTL time.Duration
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
	return &AuthService{store: authStore, sessionTTL: defaultSessionTTL}
}

func (a *AuthService) AuthRequired(ctx context.Context, apiTokenConfigured bool) (bool, error) {
	if apiTokenConfigured {
		return true, nil
	}
	if a == nil || a.store == nil {
		return false, nil
	}
	count, err := a.store.CountEnabledUsers(ctx)
	if err != nil {
		return false, err
	}
	return count > 0, nil
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
		return nil, fmt.Errorf("invalid credentials")
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
