package store

import (
	"crypto/rand"
	"fmt"
)

const (
	base36Alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	idHashLength   = 4
	idMaxAttempts  = 20
)

// GenerateID returns a new canonical task ID using a project prefix.
// It retries on collisions using the provided exists function.
func GenerateID(prefix string, exists func(string) (bool, error)) (string, error) {
	if prefix == "" {
		return "", fmt.Errorf("project prefix is required")
	}

	for i := 0; i < idMaxAttempts; i++ {
		hash, err := randomBase36(idHashLength)
		if err != nil {
			return "", err
		}
		id := fmt.Sprintf("%s-%s", prefix, hash)
		if exists == nil {
			return id, nil
		}
		ok, err := exists(id)
		if err != nil {
			return "", err
		}
		if !ok {
			return id, nil
		}
	}

	return "", fmt.Errorf("unable to generate unique id")
}

// GenerateAttachmentID returns a new attachment id using the at- prefix.
func GenerateAttachmentID(exists func(string) (bool, error)) (string, error) {
	return GenerateID("at", exists)
}

// GenerateBlobID returns a new blob id using the bl- prefix.
func GenerateBlobID(exists func(string) (bool, error)) (string, error) {
	return GenerateID("bl", exists)
}

func randomBase36(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for i := 0; i < length; i++ {
		out[i] = base36Alphabet[int(b[i])%len(base36Alphabet)]
	}
	return string(out), nil
}
