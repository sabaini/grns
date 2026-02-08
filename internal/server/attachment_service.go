package server

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"grns/internal/blobstore"
	"grns/internal/models"
	"grns/internal/store"
)

const (
	defaultBlobGCBatchSize             = 500
	attachmentAllowedMediaTypesEnvKey  = "GRNS_ATTACH_ALLOWED_MEDIA_TYPES"
	attachmentRejectMismatchEnvKey     = "GRNS_ATTACH_REJECT_MEDIA_TYPE_MISMATCH"
	fallbackAttachmentContentMediaType = "application/octet-stream"
)

// AttachmentService orchestrates attachment workflows and validation.
type AttachmentService struct {
	taskStore       store.TaskServiceStore
	attachmentStore store.AttachmentStore
	blobStore       blobstore.BlobStore

	allowedMediaTypes map[string]struct{}
	rejectMismatch    bool
	gcBatchSize       int
}

// AttachmentContent describes managed attachment stream metadata.
type AttachmentContent struct {
	Reader    io.ReadCloser
	SizeBytes int64
	MediaType string
	Filename  string
}

// BlobGCResult reports one GC run result.
type BlobGCResult struct {
	CandidateCount int   `json:"candidate_count"`
	DeletedCount   int   `json:"deleted_count"`
	FailedCount    int   `json:"failed_count"`
	ReclaimedBytes int64 `json:"reclaimed_bytes"`
	DryRun         bool  `json:"dry_run"`
}

// NewAttachmentService constructs an AttachmentService.
func NewAttachmentService(taskStore store.TaskServiceStore, attachmentStore store.AttachmentStore, blobStore blobstore.BlobStore) *AttachmentService {
	svc := &AttachmentService{taskStore: taskStore, attachmentStore: attachmentStore, blobStore: blobStore}
	svc.ConfigurePolicy(nil, rejectMediaTypeMismatch(), defaultBlobGCBatchSize)
	if envAllowed := allowedAttachmentMediaTypes(); len(envAllowed) > 0 {
		configured := make([]string, 0, len(envAllowed))
		for mediaType := range envAllowed {
			configured = append(configured, mediaType)
		}
		svc.ConfigurePolicy(configured, svc.rejectMismatch, svc.gcBatchSize)
	}
	return svc
}

// ConfigurePolicy overrides attachment media and GC policy.
func (s *AttachmentService) ConfigurePolicy(allowedMediaTypes []string, rejectMismatch bool, gcBatchSize int) {
	if s == nil {
		return
	}
	normalized := map[string]struct{}{}
	for _, raw := range allowedMediaTypes {
		mediaType, err := normalizeMediaType(raw)
		if err != nil || mediaType == "" {
			continue
		}
		normalized[mediaType] = struct{}{}
	}
	if len(normalized) == 0 {
		s.allowedMediaTypes = nil
	} else {
		s.allowedMediaTypes = normalized
	}
	s.rejectMismatch = rejectMismatch
	if gcBatchSize <= 0 {
		gcBatchSize = defaultBlobGCBatchSize
	}
	s.gcBatchSize = gcBatchSize
}

// CreateManagedAttachmentInput describes creation of a managed-blob attachment.
type CreateManagedAttachmentInput struct {
	Kind              string
	Title             string
	Filename          string
	MediaType         string
	MediaTypeSource   string
	DeclaredMediaType string
	SniffedMediaType  string
	BlobID            string
	Labels            []string
	Meta              map[string]any
	ExpiresAt         *time.Time
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
	if !validateBlobID(in.BlobID) {
		return zero, badRequestCode(fmt.Errorf("invalid blob_id"), ErrCodeInvalidID)
	}
	if err := s.ensureTaskExists(taskID); err != nil {
		return zero, err
	}

	kind, err := models.ParseAttachmentKind(in.Kind)
	if err != nil {
		return zero, badRequestCode(err, ErrCodeInvalidArgument)
	}
	labels, err := normalizeLabels(in.Labels)
	if err != nil {
		return zero, badRequest(err)
	}
	mediaType, mediaTypeSource, err := s.resolveManagedMediaType(in)
	if err != nil {
		return zero, err
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
		MediaType:       mediaType,
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

// CreateManagedAttachmentFromReader persists content bytes, upserts blob metadata,
// and creates a managed attachment pointing to that blob.
func (s *AttachmentService) CreateManagedAttachmentFromReader(ctx context.Context, taskID string, in CreateManagedAttachmentInput, content io.Reader) (models.Attachment, error) {
	var zero models.Attachment
	if content == nil {
		return zero, badRequestCode(fmt.Errorf("content is required"), ErrCodeMissingRequired)
	}
	if s == nil || s.taskStore == nil || s.attachmentStore == nil || s.blobStore == nil {
		return zero, internalError(fmt.Errorf("attachment service is not configured"))
	}

	taskID = strings.TrimSpace(taskID)
	if !validateID(taskID) {
		return zero, badRequestCode(fmt.Errorf("invalid task_id"), ErrCodeInvalidID)
	}
	if err := s.ensureTaskExists(taskID); err != nil {
		return zero, err
	}

	kind, err := models.ParseAttachmentKind(in.Kind)
	if err != nil {
		return zero, badRequestCode(err, ErrCodeInvalidArgument)
	}
	labels, err := normalizeLabels(in.Labels)
	if err != nil {
		return zero, badRequest(err)
	}
	mediaType, mediaTypeSource, err := s.resolveManagedMediaType(in)
	if err != nil {
		return zero, err
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
		MediaType:       mediaType,
		MediaTypeSource: mediaTypeSource,
		Meta:            in.Meta,
		Labels:          labels,
		CreatedAt:       now,
		UpdatedAt:       now,
		ExpiresAt:       in.ExpiresAt,
	}

	putResult, err := s.blobStore.Put(ctx, content)
	if err != nil {
		return zero, err
	}

	_, err = s.attachmentStore.CreateManagedAttachmentWithBlob(ctx, &models.Blob{
		SHA256:         putResult.SHA256,
		SizeBytes:      putResult.SizeBytes,
		StorageBackend: "local_cas",
		BlobKey:        putResult.BlobKey,
	}, attachment)
	if err != nil {
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
		scheme := strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" {
			return zero, badRequestCode(fmt.Errorf("external_url must use http or https"), ErrCodeInvalidArgument)
		}
	}
	if repoPath != "" {
		if err := validateWorkspaceRelativePath(repoPath); err != nil {
			return zero, badRequestCode(err, ErrCodeInvalidArgument)
		}
	}

	kind, err := models.ParseAttachmentKind(in.Kind)
	if err != nil {
		return zero, badRequestCode(err, ErrCodeInvalidArgument)
	}
	labels, err := normalizeLabels(in.Labels)
	if err != nil {
		return zero, badRequest(err)
	}
	mediaType, err := normalizeMediaType(in.MediaType)
	if err != nil {
		return zero, err
	}
	if err := s.validateAllowedMediaType(mediaType); err != nil {
		return zero, err
	}
	mediaTypeSource, err := normalizeAttachmentMediaTypeSource(in.MediaTypeSource, mediaType)
	if err != nil {
		return zero, err
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
		MediaType:       mediaType,
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
	if !validateAttachmentID(id) {
		return zero, badRequestCode(fmt.Errorf("invalid attachment id"), ErrCodeInvalidID)
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
	if !validateAttachmentID(id) {
		return badRequestCode(fmt.Errorf("invalid attachment id"), ErrCodeInvalidID)
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

// OpenAttachmentContent opens the stream for a managed attachment.
func (s *AttachmentService) OpenAttachmentContent(ctx context.Context, attachmentID string) (*AttachmentContent, error) {
	if s == nil || s.attachmentStore == nil || s.blobStore == nil {
		return nil, internalError(fmt.Errorf("attachment service is not configured"))
	}
	attachmentID = strings.TrimSpace(attachmentID)
	if !validateAttachmentID(attachmentID) {
		return nil, badRequestCode(fmt.Errorf("invalid attachment id"), ErrCodeInvalidID)
	}

	attachment, err := s.attachmentStore.GetAttachment(ctx, attachmentID)
	if err != nil {
		return nil, err
	}
	if attachment == nil {
		return nil, notFoundCode(fmt.Errorf("attachment not found"), ErrCodeAttachmentNotFound)
	}
	if attachment.SourceType != string(models.AttachmentSourceManagedBlob) {
		return nil, badRequestCode(fmt.Errorf("attachment has no managed content"), ErrCodeInvalidArgument)
	}
	if !validateBlobID(attachment.BlobID) {
		return nil, internalError(fmt.Errorf("attachment has invalid blob id"))
	}

	blob, err := s.attachmentStore.GetBlob(ctx, attachment.BlobID)
	if err != nil {
		return nil, err
	}
	if blob == nil {
		return nil, notFoundCode(fmt.Errorf("attachment content not found"), ErrCodeAttachmentNotFound)
	}

	rc, err := s.blobStore.Open(ctx, blob.BlobKey)
	if err != nil {
		return nil, notFoundCode(fmt.Errorf("attachment content not found"), ErrCodeAttachmentNotFound)
	}

	mediaType := strings.TrimSpace(attachment.MediaType)
	if mediaType == "" {
		mediaType = fallbackAttachmentContentMediaType
	}
	filename := strings.TrimSpace(attachment.Filename)
	if filename == "" {
		filename = strings.TrimSpace(attachment.Title)
	}
	if filename == "" {
		filename = attachment.ID
	}

	return &AttachmentContent{Reader: rc, SizeBytes: blob.SizeBytes, MediaType: mediaType, Filename: filename}, nil
}

// GCBlobs sweeps unreferenced blobs and optionally deletes them.
func (s *AttachmentService) GCBlobs(ctx context.Context, batchSize int, apply bool) (BlobGCResult, error) {
	result := BlobGCResult{DryRun: !apply}
	if s == nil || s.attachmentStore == nil || s.blobStore == nil {
		return result, internalError(fmt.Errorf("attachment service is not configured"))
	}
	if batchSize <= 0 {
		batchSize = s.gcBatchSize
		if batchSize <= 0 {
			batchSize = defaultBlobGCBatchSize
		}
	}

	if !apply {
		blobs, err := s.attachmentStore.ListUnreferencedBlobs(ctx, 0)
		if err != nil {
			return result, err
		}
		result.CandidateCount = len(blobs)
		for _, blob := range blobs {
			result.ReclaimedBytes += blob.SizeBytes
		}
		return result, nil
	}

	for {
		blobs, err := s.attachmentStore.ListUnreferencedBlobs(ctx, batchSize)
		if err != nil {
			return result, err
		}
		if len(blobs) == 0 {
			return result, nil
		}
		result.CandidateCount += len(blobs)

		for _, blob := range blobs {
			if err := s.blobStore.Delete(ctx, blob.BlobKey); err != nil {
				result.FailedCount++
				continue
			}
			if err := s.attachmentStore.DeleteBlob(ctx, blob.ID); err != nil {
				result.FailedCount++
				continue
			}
			result.DeletedCount++
			result.ReclaimedBytes += blob.SizeBytes
		}
	}
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

func (s *AttachmentService) resolveManagedMediaType(in CreateManagedAttachmentInput) (string, string, error) {
	declared := strings.TrimSpace(in.DeclaredMediaType)
	if declared == "" && strings.TrimSpace(in.MediaTypeSource) == string(models.MediaTypeSourceDeclared) {
		declared = strings.TrimSpace(in.MediaType)
	}
	if declared == "" && strings.TrimSpace(in.MediaTypeSource) == "" {
		declared = strings.TrimSpace(in.MediaType)
	}
	sniffed := strings.TrimSpace(in.SniffedMediaType)
	if sniffed == "" && strings.TrimSpace(in.MediaTypeSource) == string(models.MediaTypeSourceSniffed) {
		sniffed = strings.TrimSpace(in.MediaType)
	}

	declaredNormalized, err := normalizeMediaType(declared)
	if err != nil {
		return "", "", err
	}
	sniffedNormalized, err := normalizeMediaType(sniffed)
	if err != nil {
		return "", "", err
	}

	if declaredNormalized != "" && s.rejectMismatch && sniffedNormalized != "" && sniffedNormalized != fallbackAttachmentContentMediaType && sniffedNormalized != declaredNormalized {
		return "", "", badRequestCode(fmt.Errorf("declared media_type does not match content type"), ErrCodeInvalidArgument)
	}

	finalMediaType := sniffedNormalized
	source := string(models.MediaTypeSourceSniffed)
	if declaredNormalized != "" {
		finalMediaType = declaredNormalized
		source = string(models.MediaTypeSourceDeclared)
	}
	if finalMediaType == "" {
		source = string(models.MediaTypeSourceUnknown)
	}

	if err := s.validateAllowedMediaType(finalMediaType); err != nil {
		return "", "", err
	}

	return finalMediaType, source, nil
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

func normalizeMediaType(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return "", badRequestCode(fmt.Errorf("invalid media_type"), ErrCodeInvalidArgument)
	}
	return strings.ToLower(strings.TrimSpace(parsed)), nil
}

func (s *AttachmentService) validateAllowedMediaType(mediaType string) error {
	mediaType = strings.TrimSpace(mediaType)
	if mediaType == "" {
		return nil
	}
	if len(s.allowedMediaTypes) == 0 {
		return nil
	}
	if _, ok := s.allowedMediaTypes[mediaType]; ok {
		return nil
	}
	return badRequestCode(fmt.Errorf("media_type is not allowed"), ErrCodeInvalidArgument)
}

func allowedAttachmentMediaTypes() map[string]struct{} {
	raw := strings.TrimSpace(os.Getenv(attachmentAllowedMediaTypesEnvKey))
	if raw == "" {
		return nil
	}
	out := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		v, err := normalizeMediaType(part)
		if err != nil || v == "" {
			continue
		}
		out[v] = struct{}{}
	}
	return out
}

func rejectMediaTypeMismatch() bool {
	raw := strings.TrimSpace(os.Getenv(attachmentRejectMismatchEnvKey))
	if raw == "" {
		return true
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return parsed
}

func validateWorkspaceRelativePath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("repo_path is required")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("repo_path must be relative")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("repo_path must not escape workspace")
	}
	return nil
}
