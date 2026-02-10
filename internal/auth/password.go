package auth

import (
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 8
	maxUsernameLength = 32
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]*[a-z0-9])?$`)

// NormalizeUsername returns canonical lowercase username and validates allowed characters.
func NormalizeUsername(raw string) (string, error) {
	username := strings.TrimSpace(strings.ToLower(raw))
	if username == "" {
		return "", fmt.Errorf("username is required")
	}
	if len(username) > maxUsernameLength {
		return "", fmt.Errorf("username too long")
	}
	if !usernamePattern.MatchString(username) {
		return "", fmt.Errorf("invalid username")
	}
	return username, nil
}

// ValidatePassword checks minimal password requirements.
func ValidatePassword(password string) error {
	if len(password) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	return nil
}

// HashPassword hashes one plaintext password for persistent storage.
func HashPassword(password string) (string, error) {
	if err := ValidatePassword(password); err != nil {
		return "", err
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// VerifyPassword verifies plaintext password against a bcrypt hash.
func VerifyPassword(passwordHash, candidate string) bool {
	if strings.TrimSpace(passwordHash) == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(candidate)) == nil
}
