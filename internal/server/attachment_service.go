package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"grns/internal/models"
	"grns/internal/store"
)

// AttachmentService orchestrates attachment workflows and validation.
//
// This is a scaffold service intended to keep attachment concerns separate from
// TaskService. Handlers should call this service (not stores directly).
type AttachmentService struct {
	taskStore       store.TaskServiceStore
	attachmentStore store.AttachmentStore
}

// NewAttachmentService constructs an AttachmentService.
func NewAttachmentService(taskStore store.TaskServiceStore, attachmentStore store.AttachmentStore) *AttachmentService {
	return &AttachmentService{taskStore: taskStore, attachmentStore: attachmentStore}
}

// CreateManagedAttachmentInput describes creation of a managed-blob attachment.
type CreateManagedAttachmentInput struct {
	Kind            string
	Title           string
	Filename        string
	MediaType       string
	MediaTypeSource string
	BlobID          string
	Labels          []string
	Meta            map[string]any
	ExpiresAt       *time.Time
}

// CreateLinkAttachmentInput describes creation of an external-url/repo-path attachment.
type CreateLinkAttachmentInput struct {
	Kind            string
	Title           string
	Filename        string
	MediaType       string
	MediaTypeSource string
	ExternalURL     string
	RepoPath        string
	Labels          []string
	Meta            map[string]any
	ExpiresAt       *time.Time
}

// CreateManagedAttachment creates an attachment row pointing at an existing managed blob.
func (s *AttachmentService) CreateManagedAttachment(ctx context.Context, taskID string, in CreateManagedAttachmentInput) (models.Attachment, error) {
	var zero models.Attachment
	if s == nil || s.taskStore == nil || s.attachmentStore == nil {
		return zero, internalError(fmt.Errorf("attachment service is not configured"))
	}

	taskID = strings.TrimSpace(taskID)
	if !validateID(taskID) {
		return zero, badRequestCode(fmt.Errorf("invalid task_id"), ErrCodeInvalidID)
	}
	if strings.TrimSpace(in.BlobID) == "" {
		return zero, badRequestCode(fmt.Errorf("blob_id is required"), ErrCodeMissingRequired)
	}
	if err := s.ensureTaskExists(taskID); err != nil {
		return zero, err
	}

	kind, err := models.ParseAttachmentKind(in.Kind)
	if err != nil {
		return zero, badRequestCode(err, ErrCodeInvalidArgument)
	}
	mediaTypeSource, err := normalizeAttachmentMediaTypeSource(in.MediaTypeSource, in.MediaType)
	if err != nil {
		return zero, err
	}
	labels, err := normalizeLabels(in.Labels)
	if err != nil {
		return zero, badRequest(err)
	}

	id, err := s.nextAttachmentID(ctx)
	if err != nil {
		return zero, err
	}
	now := time.Now().UTC()
	attachment := &models.Attachment{
		ID:              id,
		TaskID:          taskID,
		Kind:            string(kind),
		SourceType:      string(models.AttachmentSourceManagedBlob),
		Title:           strings.TrimSpace(in.Title),
		Filename:        strings.TrimSpace(in.Filename),
		MediaType:       strings.ToLower(strings.TrimSpace(in.MediaType)),
		MediaTypeSource: mediaTypeSource,
		BlobID:          strings.TrimSpace(in.BlobID),
		Meta:            in.Meta,
		Labels:          labels,
		CreatedAt:       now,
		UpdatedAt:       now,
		ExpiresAt:       in.ExpiresAt,
	}
	if err := s.attachmentStore.CreateAttachment(ctx, attachment); err != nil {
		if isUniqueConstraint(err) {
			return zero, conflictCode(fmt.Errorf("attachment already exists"), ErrCodeConflict)
		}
		return zero, err
	}

	stored, err := s.attachmentStore.GetAttachment(ctx, attachment.ID)
	if err != nil {
		return zero, err
	}
	if stored == nil {
		return zero, internalError(fmt.Errorf("attachment not found after create"))
	}
	return *stored, nil
}

// CreateManagedAttachmentFromReader computes blob metadata from a content stream,
// upserts blob metadata, and creates a managed attachment pointing to that blob.
func (s *AttachmentService) CreateManagedAttachmentFromReader(ctx context.Context, taskID string, in CreateManagedAttachmentInput, content io.Reader) (models.Attachment, error) {
	var zero models.Attachment
	if content == nil {
		return zero, badRequestCode(fmt.Errorf("content is required"), ErrCodeMissingRequired)
	}
	if s == nil || s.attachmentStore == nil {
		return zero, internalError(fmt.Errorf("attachment service is not configured"))
	}

	shaHex, sizeBytes, blobKey, err := blobPutFromReader(content)
	if err != nil {
		return zero, err
	}

	blob, err := s.attachmentStore.UpsertBlob(ctx, &models.Blob{
		SHA256:         shaHex,
		SizeBytes:      sizeBytes,
		StorageBackend: "local_cas",
		BlobKey:        blobKey,
	})
	if err != nil {
		return zero, err
	}
	if blob == nil {
		return zero, internalError(fmt.Errorf("blob upsert returned nil blob"))
	}

	in.BlobID = blob.ID
	return s.CreateManagedAttachment(ctx, taskID, in)
}

// CreateLinkAttachment creates an attachment pointing to an external URL or repo path.
func (s *AttachmentService) CreateLinkAttachment(ctx context.Context, taskID string, in CreateLinkAttachmentInput) (models.Attachment, error) {
	var zero models.Attachment
	if s == nil || s.taskStore == nil || s.attachmentStore == nil {
		return zero, internalError(fmt.Errorf("attachment service is not configured"))
	}

	taskID = strings.TrimSpace(taskID)
	if !validateID(taskID) {
		return zero, badRequestCode(fmt.Errorf("invalid task_id"), ErrCodeInvalidID)
	}
	if err := s.ensureTaskExists(taskID); err != nil {
		return zero, err
	}

	externalURL := strings.TrimSpace(in.ExternalURL)
	repoPath := strings.TrimSpace(in.RepoPath)
	if (externalURL == "" && repoPath == "") || (externalURL != "" && repoPath != "") {
		return zero, badRequestCode(fmt.Errorf("exactly one of external_url or repo_path is required"), ErrCodeMissingRequired)
	}
	if externalURL != "" {
		u, err := url.Parse(externalURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return zero, badRequestCode(fmt.Errorf("invalid external_url"), ErrCodeInvalidArgument)
		}
	}

	kind, err := models.ParseAttachmentKind(in.Kind)
	if err != nil {
		return zero, badRequestCode(err, ErrCodeInvalidArgument)
	}
	mediaTypeSource, err := normalizeAttachmentMediaTypeSource(in.MediaTypeSource, in.MediaType)
	if err != nil {
		return zero, err
	}
	labels, err := normalizeLabels(in.Labels)
	if err != nil {
		return zero, badRequest(err)
	}

	sourceType := string(models.AttachmentSourceRepoPath)
	if externalURL != "" {
		sourceType = string(models.AttachmentSourceExternalURL)
	}

	id, err := s.nextAttachmentID(ctx)
	if err != nil {
		return zero, err
	}
	now := time.Now().UTC()
	attachment := &models.Attachment{
		ID:              id,
		TaskID:          taskID,
		Kind:            string(kind),
		SourceType:      sourceType,
		Title:           strings.TrimSpace(in.Title),
		Filename:        strings.TrimSpace(in.Filename),
		MediaType:       strings.ToLower(strings.TrimSpace(in.MediaType)),
		MediaTypeSource: mediaTypeSource,
		ExternalURL:     externalURL,
		RepoPath:        repoPath,
		Meta:            in.Meta,
		Labels:          labels,
		CreatedAt:       now,
		UpdatedAt:       now,
		ExpiresAt:       in.ExpiresAt,
	}
	if err := s.attachmentStore.CreateAttachment(ctx, attachment); err != nil {
		if isUniqueConstraint(err) {
			return zero, conflictCode(fmt.Errorf("attachment already exists"), ErrCodeConflict)
		}
		return zero, err
	}

	stored, err := s.attachmentStore.GetAttachment(ctx, attachment.ID)
	if err != nil {
		return zero, err
	}
	if stored == nil {
		return zero, internalError(fmt.Errorf("attachment not found after create"))
	}
	return *stored, nil
}

// ListTaskAttachments lists all attachments for one task.
func (s *AttachmentService) ListTaskAttachments(ctx context.Context, taskID string) ([]models.Attachment, error) {
	if s == nil || s.taskStore == nil || s.attachmentStore == nil {
		return nil, internalError(fmt.Errorf("attachment service is not configured"))
	}

	taskID = strings.TrimSpace(taskID)
	if !validateID(taskID) {
		return nil, badRequestCode(fmt.Errorf("invalid task_id"), ErrCodeInvalidID)
	}
	if err := s.ensureTaskExists(taskID); err != nil {
		return nil, err
	}
	return s.attachmentStore.ListAttachmentsByTask(ctx, taskID)
}

// GetAttachment returns one attachment by id.
func (s *AttachmentService) GetAttachment(ctx context.Context, id string) (models.Attachment, error) {
	var zero models.Attachment
	if s == nil || s.attachmentStore == nil {
		return zero, internalError(fmt.Errorf("attachment service is not configured"))
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return zero, badRequestCode(fmt.Errorf("attachment id is required"), ErrCodeMissingRequired)
	}

	attachment, err := s.attachmentStore.GetAttachment(ctx, id)
	if err != nil {
		return zero, err
	}
	if attachment == nil {
		return zero, notFoundCode(fmt.Errorf("attachment not found"), ErrCodeAttachmentNotFound)
	}
	return *attachment, nil
}

// DeleteAttachment removes one attachment row.
func (s *AttachmentService) DeleteAttachment(ctx context.Context, id string) error {
	if s == nil || s.attachmentStore == nil {
		return internalError(fmt.Errorf("attachment service is not configured"))
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return badRequestCode(fmt.Errorf("attachment id is required"), ErrCodeMissingRequired)
	}

	attachment, err := s.attachmentStore.GetAttachment(ctx, id)
	if err != nil {
		return err
	}
	if attachment == nil {
		return notFoundCode(fmt.Errorf("attachment not found"), ErrCodeAttachmentNotFound)
	}
	return s.attachmentStore.DeleteAttachment(ctx, id)
}

// StreamAttachmentContent is intentionally left as a scaffold placeholder.
func (s *AttachmentService) StreamAttachmentContent(ctx context.Context, attachmentID string) error {
	_ = ctx
	_ = attachmentID
	return fmt.Errorf("attachment content streaming is not implemented")
}

// GCBlobs is intentionally left as a scaffold placeholder.
func (s *AttachmentService) GCBlobs(ctx context.Context, limit int, apply bool) error {
	_ = ctx
	_ = limit
	_ = apply
	return fmt.Errorf("blob gc orchestration is not implemented")
}

func (s *AttachmentService) ensureTaskExists(id string) error {
	exists, err := s.taskStore.TaskExists(id)
	if err != nil {
		return err
	}
	if !exists {
		return notFoundCode(fmt.Errorf("task not found"), ErrCodeTaskNotFound)
	}
	return nil
}

func (s *AttachmentService) nextAttachmentID(ctx context.Context) (string, error) {
	exists := func(id string) (bool, error) {
		attachment, err := s.attachmentStore.GetAttachment(ctx, id)
		if err != nil {
			return false, err
		}
		return attachment != nil, nil
	}
	return store.GenerateAttachmentID(exists)
}

func normalizeAttachmentMediaTypeSource(raw, mediaType string) (string, error) {
	raw = strings.TrimSpace(raw)
	mediaType = strings.TrimSpace(mediaType)
	if raw == "" {
		if mediaType != "" {
			return string(models.MediaTypeSourceDeclared), nil
		}
		return string(models.MediaTypeSourceUnknown), nil
	}
	parsed, err := models.ParseAttachmentMediaTypeSource(raw)
	if err != nil {
		return "", badRequestCode(err, ErrCodeInvalidArgument)
	}
	return string(parsed), nil
}

func blobPutFromReader(r io.Reader) (shaHex string, sizeBytes int64, blobKey string, err error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", 0, "", err
	}
	digest := hex.EncodeToString(h.Sum(nil))
	key := fmt.Sprintf("sha256/%s/%s/%s", digest[0:2], digest[2:4], digest)
	return digest, n, key, nil
}
