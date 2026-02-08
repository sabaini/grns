package store

import (
	"context"

	"grns/internal/models"
)

// AttachmentStore is the metadata persistence surface for attachments and blobs.
//
// This is intentionally separate from TaskStore to keep task and attachment
// responsibilities decoupled.
type AttachmentStore interface {
	CreateAttachment(ctx context.Context, attachment *models.Attachment) error
	GetAttachment(ctx context.Context, id string) (*models.Attachment, error)
	ListAttachmentsByTask(ctx context.Context, taskID string) ([]models.Attachment, error)
	DeleteAttachment(ctx context.Context, id string) error

	ReplaceAttachmentLabels(ctx context.Context, attachmentID string, labels []string) error
	ListAttachmentLabels(ctx context.Context, attachmentID string) ([]string, error)

	UpsertBlob(ctx context.Context, blob *models.Blob) (*models.Blob, error)
	CreateManagedAttachmentWithBlob(ctx context.Context, blob *models.Blob, attachment *models.Attachment) (*models.Blob, error)
	GetBlob(ctx context.Context, id string) (*models.Blob, error)
	GetBlobBySHA256(ctx context.Context, sha string) (*models.Blob, error)
	ListUnreferencedBlobs(ctx context.Context, limit int) ([]models.Blob, error)
	DeleteBlob(ctx context.Context, id string) error
}

var _ AttachmentStore = (*Store)(nil)
