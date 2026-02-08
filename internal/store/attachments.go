package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"grns/internal/models"
)

const attachmentColumns = "id, task_id, kind, source_type, title, filename, media_type, media_type_source, blob_id, external_url, repo_path, meta_json, created_at, updated_at, expires_at"
const blobColumns = "id, sha256, size_bytes, storage_backend, blob_key, created_at"

// CreateAttachment inserts one attachment row and optional labels.
func (s *Store) CreateAttachment(ctx context.Context, attachment *models.Attachment) (err error) {
	if attachment == nil {
		return fmt.Errorf("attachment is required")
	}

	now := time.Now().UTC()
	if attachment.CreatedAt.IsZero() {
		attachment.CreatedAt = now
	}
	if attachment.UpdatedAt.IsZero() {
		attachment.UpdatedAt = attachment.CreatedAt
	}
	if strings.TrimSpace(attachment.MediaTypeSource) == "" {
		attachment.MediaTypeSource = string(models.MediaTypeSourceUnknown)
	}

	metaJSON, err := attachmentMetaToJSON(attachment.Meta)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err := insertAttachmentRowTx(ctx, tx, attachment, metaJSON); err != nil {
		return err
	}

	labels := normalizeAttachmentLabels(attachment.Labels)
	if err := insertAttachmentLabelsTx(ctx, tx, attachment.ID, labels); err != nil {
		return err
	}

	return tx.Commit()
}

// GetAttachment returns one attachment with labels.
func (s *Store) GetAttachment(ctx context.Context, id string) (*models.Attachment, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+attachmentColumns+` FROM attachments WHERE id = ?`, id)
	attachment, err := scanAttachment(row)
	if err != nil || attachment == nil {
		return attachment, err
	}

	labels, err := s.ListAttachmentLabels(ctx, id)
	if err != nil {
		return nil, err
	}
	attachment.Labels = labels
	return attachment, nil
}

// ListAttachmentsByTask lists attachments for a task ordered by created_at descending.
func (s *Store) ListAttachmentsByTask(ctx context.Context, taskID string) ([]models.Attachment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+attachmentColumns+` FROM attachments WHERE task_id = ? ORDER BY created_at DESC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := []models.Attachment{}
	for rows.Next() {
		attachment, err := scanAttachment(rows)
		if err != nil {
			return nil, err
		}
		if attachment == nil {
			continue
		}
		attachments = append(attachments, *attachment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range attachments {
		labels, err := s.ListAttachmentLabels(ctx, attachments[i].ID)
		if err != nil {
			return nil, err
		}
		attachments[i].Labels = labels
	}

	return attachments, nil
}

// DeleteAttachment deletes one attachment row.
func (s *Store) DeleteAttachment(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM attachments WHERE id = ?", id)
	return err
}

// ReplaceAttachmentLabels replaces all labels for one attachment.
func (s *Store) ReplaceAttachmentLabels(ctx context.Context, attachmentID string, labels []string) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, "DELETE FROM attachment_labels WHERE attachment_id = ?", attachmentID); err != nil {
		return err
	}
	if err := insertAttachmentLabelsTx(ctx, tx, attachmentID, normalizeAttachmentLabels(labels)); err != nil {
		return err
	}

	return tx.Commit()
}

// ListAttachmentLabels lists labels for one attachment.
func (s *Store) ListAttachmentLabels(ctx context.Context, attachmentID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT label FROM attachment_labels WHERE attachment_id = ? ORDER BY label ASC", attachmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := []string{}
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}

// UpsertBlob inserts a blob if absent and returns the canonical row by sha256.
func (s *Store) UpsertBlob(ctx context.Context, blob *models.Blob) (*models.Blob, error) {
	if blob == nil {
		return nil, fmt.Errorf("blob is required")
	}
	blob.SHA256 = strings.ToLower(strings.TrimSpace(blob.SHA256))
	blob.BlobKey = strings.TrimSpace(blob.BlobKey)
	if blob.SHA256 == "" {
		return nil, fmt.Errorf("sha256 is required")
	}
	if blob.BlobKey == "" {
		return nil, fmt.Errorf("blob_key is required")
	}
	if blob.SizeBytes < 0 {
		return nil, fmt.Errorf("size_bytes must be >= 0")
	}

	if strings.TrimSpace(blob.ID) == "" {
		generated, err := GenerateBlobID(func(id string) (bool, error) {
			return s.blobIDExists(ctx, id)
		})
		if err != nil {
			return nil, err
		}
		blob.ID = generated
	}

	if strings.TrimSpace(blob.StorageBackend) == "" {
		blob.StorageBackend = "local_cas"
	}
	if blob.CreatedAt.IsZero() {
		blob.CreatedAt = time.Now().UTC()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO blobs (id, sha256, size_bytes, storage_backend, blob_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, blob.ID, blob.SHA256, blob.SizeBytes, blob.StorageBackend, blob.BlobKey, dbFormatTime(blob.CreatedAt))
	if err != nil {
		return nil, err
	}

	return s.GetBlobBySHA256(ctx, blob.SHA256)
}

// CreateManagedAttachmentWithBlob upserts blob metadata and creates attachment rows in one transaction.
func (s *Store) CreateManagedAttachmentWithBlob(ctx context.Context, blob *models.Blob, attachment *models.Attachment) (_ *models.Blob, err error) {
	if blob == nil {
		return nil, fmt.Errorf("blob is required")
	}
	if attachment == nil {
		return nil, fmt.Errorf("attachment is required")
	}

	blob.SHA256 = strings.ToLower(strings.TrimSpace(blob.SHA256))
	blob.BlobKey = strings.TrimSpace(blob.BlobKey)
	if blob.SHA256 == "" {
		return nil, fmt.Errorf("sha256 is required")
	}
	if blob.BlobKey == "" {
		return nil, fmt.Errorf("blob_key is required")
	}
	if blob.SizeBytes < 0 {
		return nil, fmt.Errorf("size_bytes must be >= 0")
	}
	if strings.TrimSpace(blob.StorageBackend) == "" {
		blob.StorageBackend = "local_cas"
	}
	if blob.CreatedAt.IsZero() {
		blob.CreatedAt = time.Now().UTC()
	}

	now := time.Now().UTC()
	if attachment.CreatedAt.IsZero() {
		attachment.CreatedAt = now
	}
	if attachment.UpdatedAt.IsZero() {
		attachment.UpdatedAt = attachment.CreatedAt
	}
	if strings.TrimSpace(attachment.MediaTypeSource) == "" {
		attachment.MediaTypeSource = string(models.MediaTypeSourceUnknown)
	}

	metaJSON, err := attachmentMetaToJSON(attachment.Meta)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if strings.TrimSpace(blob.ID) == "" {
		generated, genErr := GenerateBlobID(func(id string) (bool, error) {
			return s.blobIDExistsTx(ctx, tx, id)
		})
		if genErr != nil {
			err = genErr
			return nil, err
		}
		blob.ID = generated
	}

	if _, err = tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO blobs (id, sha256, size_bytes, storage_backend, blob_key, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, blob.ID, blob.SHA256, blob.SizeBytes, blob.StorageBackend, blob.BlobKey, dbFormatTime(blob.CreatedAt)); err != nil {
		return nil, err
	}

	canonicalBlob, err := scanBlob(tx.QueryRowContext(ctx, `SELECT `+blobColumns+` FROM blobs WHERE sha256 = ?`, blob.SHA256))
	if err != nil {
		return nil, err
	}
	if canonicalBlob == nil {
		return nil, fmt.Errorf("blob not found after upsert")
	}

	attachment.BlobID = canonicalBlob.ID
	if err := insertAttachmentRowTx(ctx, tx, attachment, metaJSON); err != nil {
		return nil, err
	}
	if err := insertAttachmentLabelsTx(ctx, tx, attachment.ID, normalizeAttachmentLabels(attachment.Labels)); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return canonicalBlob, nil
}

// GetBlob returns one blob by id.
func (s *Store) GetBlob(ctx context.Context, id string) (*models.Blob, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+blobColumns+` FROM blobs WHERE id = ?`, id)
	return scanBlob(row)
}

// GetBlobBySHA256 returns one blob by digest.
func (s *Store) GetBlobBySHA256(ctx context.Context, sha string) (*models.Blob, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+blobColumns+` FROM blobs WHERE sha256 = ?`, strings.ToLower(strings.TrimSpace(sha)))
	return scanBlob(row)
}

// ListUnreferencedBlobs returns blobs that are not referenced by attachments.
func (s *Store) ListUnreferencedBlobs(ctx context.Context, limit int) ([]models.Blob, error) {
	query := `
		SELECT b.id, b.sha256, b.size_bytes, b.storage_backend, b.blob_key, b.created_at
		FROM blobs b
		LEFT JOIN attachments a ON a.blob_id = b.id
		WHERE a.id IS NULL
		ORDER BY b.created_at ASC`
	args := []any{}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	blobs := []models.Blob{}
	for rows.Next() {
		blob, err := scanBlob(rows)
		if err != nil {
			return nil, err
		}
		if blob != nil {
			blobs = append(blobs, *blob)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return blobs, nil
}

// DeleteBlob deletes one blob row by id.
func (s *Store) DeleteBlob(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM blobs WHERE id = ?", id)
	return err
}

func (s *Store) blobIDExists(ctx context.Context, id string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, "SELECT 1 FROM blobs WHERE id = ? LIMIT 1", id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) blobIDExistsTx(ctx context.Context, tx *sql.Tx, id string) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, "SELECT 1 FROM blobs WHERE id = ? LIMIT 1", id).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func insertAttachmentRowTx(ctx context.Context, tx *sql.Tx, attachment *models.Attachment, metaJSON any) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO attachments (
			id, task_id, kind, source_type, title, filename, media_type, media_type_source,
			blob_id, external_url, repo_path, meta_json, created_at, updated_at, expires_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		attachment.ID,
		attachment.TaskID,
		attachment.Kind,
		attachment.SourceType,
		nullIfEmpty(strings.TrimSpace(attachment.Title)),
		nullIfEmpty(strings.TrimSpace(attachment.Filename)),
		nullIfEmpty(strings.TrimSpace(attachment.MediaType)),
		attachment.MediaTypeSource,
		nullIfEmpty(strings.TrimSpace(attachment.BlobID)),
		nullIfEmpty(strings.TrimSpace(attachment.ExternalURL)),
		nullIfEmpty(strings.TrimSpace(attachment.RepoPath)),
		metaJSON,
		dbFormatTime(attachment.CreatedAt),
		dbFormatTime(attachment.UpdatedAt),
		nullTime(attachment.ExpiresAt),
	)
	return err
}

func scanAttachment(scanner interface {
	Scan(dest ...any) error
}) (*models.Attachment, error) {
	attachment := models.Attachment{}

	var title, filename, mediaType, mediaTypeSource sql.NullString
	var blobID, externalURL, repoPath, metaJSON sql.NullString
	var createdAt, updatedAt string
	var expiresAt sql.NullString

	err := scanner.Scan(
		&attachment.ID,
		&attachment.TaskID,
		&attachment.Kind,
		&attachment.SourceType,
		&title,
		&filename,
		&mediaType,
		&mediaTypeSource,
		&blobID,
		&externalURL,
		&repoPath,
		&metaJSON,
		&createdAt,
		&updatedAt,
		&expiresAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	attachment.Title = title.String
	attachment.Filename = filename.String
	attachment.MediaType = mediaType.String
	attachment.MediaTypeSource = mediaTypeSource.String
	attachment.BlobID = blobID.String
	attachment.ExternalURL = externalURL.String
	attachment.RepoPath = repoPath.String

	parsedCreated, err := dbParseTime(createdAt)
	if err != nil {
		return nil, err
	}
	parsedUpdated, err := dbParseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	attachment.CreatedAt = parsedCreated
	attachment.UpdatedAt = parsedUpdated

	if expiresAt.Valid {
		parsedExpiresAt, err := dbParseTime(expiresAt.String)
		if err != nil {
			return nil, err
		}
		attachment.ExpiresAt = &parsedExpiresAt
	}

	if metaJSON.Valid && metaJSON.String != "" {
		if err := json.Unmarshal([]byte(metaJSON.String), &attachment.Meta); err != nil {
			return nil, fmt.Errorf("parse attachment meta_json: %w", err)
		}
	}

	return &attachment, nil
}

func scanBlob(scanner interface {
	Scan(dest ...any) error
}) (*models.Blob, error) {
	blob := models.Blob{}
	var createdAt string

	err := scanner.Scan(&blob.ID, &blob.SHA256, &blob.SizeBytes, &blob.StorageBackend, &blob.BlobKey, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	parsedCreated, err := dbParseTime(createdAt)
	if err != nil {
		return nil, err
	}
	blob.CreatedAt = parsedCreated

	return &blob, nil
}

func attachmentMetaToJSON(meta map[string]any) (any, error) {
	if len(meta) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal attachment meta_json: %w", err)
	}
	return string(data), nil
}

func normalizeAttachmentLabels(labels []string) []string {
	if len(labels) == 0 {
		return nil
	}
	out := make([]string, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		normalized := strings.ToLower(strings.TrimSpace(label))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func insertAttachmentLabelsTx(ctx context.Context, tx *sql.Tx, attachmentID string, labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	_, err := tx.ExecContext(ctx, "INSERT OR IGNORE INTO attachment_labels (attachment_id, label) VALUES "+labelValues(len(labels)), labelArgs(attachmentID, labels)...)
	return err
}
