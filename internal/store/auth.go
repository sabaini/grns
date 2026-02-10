package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	authUserRoleAdmin = "admin"
)

// CountEnabledUsers returns the number of non-disabled provisioned users.
func (s *Store) CountEnabledUsers(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE disabled = 0").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CreateAdminUser creates one local admin user.
func (s *Store) CreateAdminUser(ctx context.Context, username, passwordHash string, now time.Time) (*AuthUser, error) {
	username = normalizeAuthUsername(username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if strings.TrimSpace(passwordHash) == "" {
		return nil, fmt.Errorf("password hash is required")
	}

	userID, err := generateAuthID("au")
	if err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO users (id, username, password_hash, role, disabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, 0, ?, ?)
	`, userID, username, passwordHash, authUserRoleAdmin, dbFormatTime(now), dbFormatTime(now))
	if err != nil {
		return nil, err
	}

	return &AuthUser{
		ID:           userID,
		Username:     username,
		PasswordHash: passwordHash,
		Role:         authUserRoleAdmin,
		Disabled:     false,
		CreatedAt:    now.UTC(),
		UpdatedAt:    now.UTC(),
	}, nil
}

// GetUserByUsername returns a provisioned user by normalized username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (*AuthUser, error) {
	username = normalizeAuthUsername(username)
	if username == "" {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, disabled, created_at, updated_at
		FROM users
		WHERE username = ?
		LIMIT 1
	`, username)
	return scanAuthUser(row)
}

// GetUserByID returns a provisioned user by id.
func (s *Store) GetUserByID(ctx context.Context, id string) (*AuthUser, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, disabled, created_at, updated_at
		FROM users
		WHERE id = ?
		LIMIT 1
	`, id)
	return scanAuthUser(row)
}

// ListUsers returns all provisioned users sorted by username.
func (s *Store) ListUsers(ctx context.Context) ([]AuthUser, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, password_hash, role, disabled, created_at, updated_at
		FROM users
		ORDER BY username ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]AuthUser, 0)
	for rows.Next() {
		user, err := scanAuthUser(rows)
		if err != nil {
			return nil, err
		}
		if user == nil {
			continue
		}
		users = append(users, *user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return users, nil
}

// SetUserDisabled updates one user's disabled state by username.
func (s *Store) SetUserDisabled(ctx context.Context, username string, disabled bool, now time.Time) (*AuthUser, error) {
	username = normalizeAuthUsername(username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}

	disabledInt := 0
	if disabled {
		disabledInt = 1
	}

	result, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET disabled = ?, updated_at = ?
		WHERE username = ?
	`, disabledInt, dbFormatTime(now), username)
	if err != nil {
		return nil, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, nil
	}
	return s.GetUserByUsername(ctx, username)
}

// DeleteUser deletes one user by username.
func (s *Store) DeleteUser(ctx context.Context, username string) (bool, error) {
	username = normalizeAuthUsername(username)
	if username == "" {
		return false, fmt.Errorf("username is required")
	}

	result, err := s.db.ExecContext(ctx, `
		DELETE FROM users
		WHERE username = ?
	`, username)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// CreateSession creates a browser session bound to one user and token hash.
func (s *Store) CreateSession(ctx context.Context, userID, tokenHash string, expiresAt, createdAt time.Time) error {
	userID = strings.TrimSpace(userID)
	tokenHash = strings.TrimSpace(tokenHash)
	if userID == "" {
		return fmt.Errorf("user id is required")
	}
	if tokenHash == "" {
		return fmt.Errorf("token hash is required")
	}

	sessionID, err := generateAuthID("as")
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, token_hash, expires_at, revoked_at, created_at)
		VALUES (?, ?, ?, ?, NULL, ?)
	`, sessionID, userID, tokenHash, dbFormatTime(expiresAt), dbFormatTime(createdAt))
	return err
}

// GetUserBySessionTokenHash returns the owning user for an active, non-revoked session token hash.
func (s *Store) GetUserBySessionTokenHash(ctx context.Context, tokenHash string, now time.Time) (*AuthUser, error) {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return nil, nil
	}

	row := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.password_hash, u.role, u.disabled, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?
		  AND s.revoked_at IS NULL
		  AND s.expires_at > ?
		  AND u.disabled = 0
		LIMIT 1
	`, tokenHash, dbFormatTime(now))

	return scanAuthUser(row)
}

// RevokeSessionByTokenHash marks one session revoked by token hash.
func (s *Store) RevokeSessionByTokenHash(ctx context.Context, tokenHash string, revokedAt time.Time) error {
	tokenHash = strings.TrimSpace(tokenHash)
	if tokenHash == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET revoked_at = ?
		WHERE token_hash = ?
		  AND revoked_at IS NULL
	`, dbFormatTime(revokedAt), tokenHash)
	return err
}

func scanAuthUser(scanner interface {
	Scan(dest ...any) error
}) (*AuthUser, error) {
	var user AuthUser
	var disabled int
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role, &disabled, &createdAt, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	user.Disabled = disabled != 0
	parsedCreated, err := dbParseTime(createdAt)
	if err != nil {
		return nil, err
	}
	parsedUpdated, err := dbParseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	user.CreatedAt = parsedCreated
	user.UpdatedAt = parsedUpdated
	return &user, nil
}

func normalizeAuthUsername(username string) string {
	return strings.TrimSpace(strings.ToLower(username))
}

func generateAuthID(prefix string) (string, error) {
	id, err := randomHex(10)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", prefix, id), nil
}

func randomHex(numBytes int) (string, error) {
	if numBytes <= 0 {
		return "", fmt.Errorf("numBytes must be > 0")
	}
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
