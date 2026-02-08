package blobstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	casAlgorithmPrefix = "sha256"
)

// LocalCAS stores blob bytes in a local content-addressed tree.
type LocalCAS struct {
	root string
}

// NewLocalCAS creates a local CAS rooted at root.
func NewLocalCAS(root string) (*LocalCAS, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("local cas root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(abs, "tmp"), 0o755); err != nil {
		return nil, err
	}
	return &LocalCAS{root: abs}, nil
}

// Put streams bytes, computes SHA-256, and stores content by digest.
func (c *LocalCAS) Put(ctx context.Context, r io.Reader) (BlobPutResult, error) {
	var zero BlobPutResult
	if c == nil {
		return zero, fmt.Errorf("blob store is not configured")
	}
	if r == nil {
		return zero, fmt.Errorf("reader is required")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	tmp, err := os.CreateTemp(filepath.Join(c.root, "tmp"), "put-*")
	if err != nil {
		return zero, err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err != nil {
		cleanup()
		return zero, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return zero, err
	}

	digest := hex.EncodeToString(h.Sum(nil))
	key := casKeyFromDigest(digest)
	dst := filepath.Join(c.root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		cleanup()
		return zero, err
	}

	if _, err := os.Stat(dst); err == nil {
		_ = os.Remove(tmpPath)
		return BlobPutResult{SHA256: digest, SizeBytes: n, BlobKey: key}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		cleanup()
		return zero, err
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		if _, statErr := os.Stat(dst); statErr == nil {
			_ = os.Remove(tmpPath)
			return BlobPutResult{SHA256: digest, SizeBytes: n, BlobKey: key}, nil
		}
		cleanup()
		return zero, err
	}

	return BlobPutResult{SHA256: digest, SizeBytes: n, BlobKey: key}, nil
}

// Open returns a reader for blob key content.
func (c *LocalCAS) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	if c == nil {
		return nil, fmt.Errorf("blob store is not configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := c.pathFromKey(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// Delete removes a blob object. Missing files are ignored.
func (c *LocalCAS) Delete(ctx context.Context, key string) error {
	if c == nil {
		return fmt.Errorf("blob store is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := c.pathFromKey(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func casKeyFromDigest(digest string) string {
	return fmt.Sprintf("%s/%s/%s/%s", casAlgorithmPrefix, digest[0:2], digest[2:4], digest)
}

func (c *LocalCAS) pathFromKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("blob key is required")
	}
	if strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("blob key must be relative")
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || strings.HasPrefix(clean, "..") || strings.Contains(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("invalid blob key")
	}
	return filepath.Join(c.root, clean), nil
}
