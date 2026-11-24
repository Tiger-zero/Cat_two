package store

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"github.com/usememos/memos/filter"
	"github.com/usememos/memos/internal/identifier"
	storepb "github.com/usememos/memos/proto/gen/store"
)

type Attachment struct {
	// ID is the system generated unique identifier for the attachment.
	ID int32
	// UID is the user defined unique identifier for the attachment.
	UID string

	// Standard fields
	CreatorID int32
	CreatedTs int64
	UpdatedTs int64

	// Domain specific fields
	Filename    string
	Blob        []byte
	Type        string
	Size        int64
	StorageType storepb.AttachmentStorageType
	Reference   string
	Payload     *storepb.AttachmentPayload

	// The related memo ID.
	MemoID *int32
	// Policy is present for transport-facing direct-link mutations. Drivers
	// revalidate it in the same transaction as the attachment insert.
	Policy *MemoWritePolicy

	// Composed field
	MemoUID         *string
	CreatorUsername *string
}

type FindAttachment struct {
	GetBlob        bool
	ID             *int32
	UID            *string
	CreatorID      *int32
	Filename       *string
	FilenameSearch *string
	MemoID         *int32
	MemoIDList     []int32
	HasRelatedMemo bool
	Filters        []string
	PreparedFilter *filter.Program
	// Access applies memo-local authorization to linked attachments
	// before pagination. Unlinked attachments are visible only to the active
	// matching creator. Nil is reserved for trusted internal callers.
	Access           *MemoAccessScope
	Limit            *int
	Offset           *int
	SkipDefaultLimit bool
}

type UpdateAttachment struct {
	ID        int32
	UID       *string
	UpdatedTs *int64
	Filename  *string
	MemoID    *int32
	Payload   *storepb.AttachmentPayload
	// Content replaces the stored file. The caller has already written the new
	// object; the previous one is the caller's to delete once this commits.
	Content *AttachmentContent
	// Policy is present for transport-facing updates. Drivers discover the
	// current attachment binding and authorize any linked memo in the same
	// transaction as the update.
	Policy *MemoWritePolicy
}

// AttachmentContent is where an attachment's file is stored and what it is.
type AttachmentContent struct {
	Type        string
	Size        int64
	StorageType storepb.AttachmentStorageType
	// Reference is the local path of a LOCAL file; empty otherwise.
	Reference string
	// Blob holds the file of a DATABASE attachment; nil otherwise.
	Blob []byte
}

type DeleteAttachment struct {
	ID     int32
	MemoID *int32
}

const (
	thumbnailCacheFolder = ".thumbnail_cache"
	motionCacheFolder    = ".motion_cache"
)

type deleteAttachmentStorageFailpointKey struct{}
type createAttachmentPolicyFailpointKey struct{}
type createAttachmentPostCommitFailpointKey struct{}

// ErrDeleteAttachmentStorageFailpoint is returned by the test-only attachment storage failpoint.
var ErrDeleteAttachmentStorageFailpoint = errors.New("delete attachment storage failpoint")

// ErrCreateAttachmentPostCommitFailpoint is returned after the test-only attachment create failpoint persists a row.
var ErrCreateAttachmentPostCommitFailpoint = errors.New("create attachment post-commit failpoint")

// WithDeleteAttachmentStorageFailpoint forces DeleteAttachmentStorage to return a failpoint error.
func WithDeleteAttachmentStorageFailpoint(ctx context.Context) context.Context {
	return context.WithValue(ctx, deleteAttachmentStorageFailpointKey{}, true)
}

// WithCreateAttachmentPolicyFailpoint forces a policy-bearing attachment create to fail before its database insert.
func WithCreateAttachmentPolicyFailpoint(ctx context.Context) context.Context {
	return context.WithValue(ctx, createAttachmentPolicyFailpointKey{}, true)
}

// WithCreateAttachmentPostCommitFailpoint forces CreateAttachment to return an error after its database insert succeeds.
func WithCreateAttachmentPostCommitFailpoint(ctx context.Context) context.Context {
	return context.WithValue(ctx, createAttachmentPostCommitFailpointKey{}, true)
}

func (s *Store) CreateAttachment(ctx context.Context, create *Attachment) (*Attachment, error) {
	if !identifier.UIDMatcher.MatchString(create.UID) {
		return nil, errors.New("invalid uid")
	}
	if err := validateMemoWritePolicy(create.Policy); err != nil {
		return nil, err
	}
	var (
		attachment *Attachment
		err        error
	)
	if create.Policy != nil {
		if create.MemoID == nil {
			return nil, errors.New("attachment write policy requires a memo")
		}
		if create.CreatorID != create.Policy.ActorUserID {
			return nil, ErrMemoPermissionDenied
		}
		if shouldFailCreateAttachmentPolicy(ctx) {
			return nil, ErrMemoPermissionDenied
		}
		attachment, err = s.driver.CreateAttachment(ctx, create)
	} else {
		attachment, err = s.driver.CreateAttachment(ctx, create)
	}
	if err != nil {
		return nil, err
	}
	if shouldFailCreateAttachmentPostCommit(ctx) {
		return nil, ErrCreateAttachmentPostCommitFailpoint
	}
	return attachment, nil
}

func (s *Store) ListAttachments(ctx context.Context, find *FindAttachment) ([]*Attachment, error) {
	// Set default limits to prevent loading too many attachments at once
	shouldApplyDefaultLimit := find.Limit == nil && find.MemoID == nil && len(find.MemoIDList) == 0 && !find.SkipDefaultLimit
	if shouldApplyDefaultLimit && find.GetBlob {
		// When fetching blobs, we should be especially careful with limits
		defaultLimit := 10
		find.Limit = &defaultLimit
	} else if shouldApplyDefaultLimit {
		// Even without blobs, let's default to a reasonable limit
		defaultLimit := 100
		find.Limit = &defaultLimit
	}

	return s.driver.ListAttachments(ctx, find)
}

func (s *Store) GetAttachment(ctx context.Context, find *FindAttachment) (*Attachment, error) {
	attachments, err := s.ListAttachments(ctx, find)
	if err != nil {
		return nil, err
	}

	if len(attachments) == 0 {
		return nil, nil
	}

	return attachments[0], nil
}

func (s *Store) UpdateAttachment(ctx context.Context, update *UpdateAttachment) error {
	if update.UID != nil && !identifier.UIDMatcher.MatchString(*update.UID) {
		return errors.New("invalid uid")
	}
	if err := validateMemoWritePolicy(update.Policy); err != nil {
		return err
	}
	if update.Policy != nil {
		if update.MemoID != nil {
			return errors.New("policy-bearing attachment updates cannot change memo binding")
		}
		return s.driver.UpdateAttachment(ctx, update)
	}
	return s.driver.UpdateAttachment(ctx, update)
}

func (s *Store) DeleteAttachment(ctx context.Context, delete *DeleteAttachment) error {
	attachment, err := s.GetAttachment(ctx, &FindAttachment{ID: &delete.ID})
	if err != nil {
		return errors.Wrap(err, "failed to get attachment")
	}
	if attachment == nil {
		return errors.New("attachment not found")
	}

	if err := s.DeleteAttachmentStorage(ctx, attachment); err != nil {
		if attachment.StorageType == storepb.AttachmentStorageType_LOCAL {
			return errors.Wrap(err, "failed to delete local file")
		}
		slog.Warn("Failed to delete attachment storage", slog.Any("err", err))
	}

	return s.driver.DeleteAttachment(ctx, delete)
}

func (s *Store) DeleteAttachments(ctx context.Context, attachments []*Attachment) error {
	if len(attachments) == 0 {
		return nil
	}

	deletes := make([]*DeleteAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		deletes = append(deletes, &DeleteAttachment{ID: attachment.ID, MemoID: attachment.MemoID})
	}
	if len(deletes) == 0 {
		return nil
	}

	if err := s.driver.DeleteAttachments(ctx, deletes); err != nil {
		return err
	}
	return s.deleteAttachmentStorageSnapshots(ctx, attachments)
}

func (s *Store) deleteAttachmentStorageSnapshots(ctx context.Context, attachments []*Attachment) error {
	for _, attachment := range attachments {
		if attachment == nil {
			continue
		}
		if err := s.DeleteAttachmentStorage(ctx, attachment); err != nil {
			if attachment.StorageType == storepb.AttachmentStorageType_LOCAL {
				return errors.Wrap(err, "failed to delete local file")
			}
			slog.Warn("Failed to delete attachment storage", slog.Any("err", err))
		}
	}
	return nil
}

// DeleteAttachmentStorage removes a local attachment file and any derived assets.
func (s *Store) DeleteAttachmentStorage(ctx context.Context, attachment *Attachment) error {
	if attachment == nil {
		return nil
	}
	if shouldFailDeleteAttachmentStorage(ctx) {
		return ErrDeleteAttachmentStorageFailpoint
	}

	if attachment.StorageType == storepb.AttachmentStorageType_LOCAL {
		referenced, err := s.localAttachmentStorageReferenced(ctx, attachment.Reference, attachment.ID, false)
		if err != nil {
			return errors.Wrap(err, "failed to check other references to the attachment file")
		}
		if !referenced {
			p := filepath.FromSlash(attachment.Reference)
			if !filepath.IsAbs(p) {
				p = filepath.Join(s.profile.Data, p)
			}
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return errors.Wrap(err, "failed to delete local file")
			}
		}
	}

	s.DeleteAttachmentDerivedCaches(attachment)
	return nil
}

// DeleteAttachmentDerivedCaches removes every derived asset the file server
// may have written for the attachment: the current thumbnail, its failure
// marker, the pre-v2 thumbnail name, and the extracted motion clip. The names
// must match server/fileserver.
func (s *Store) DeleteAttachmentDerivedCaches(attachment *Attachment) {
	for _, cachePath := range []string{
		filepath.Join(s.profile.Data, thumbnailCacheFolder, attachment.UID+".v2.jpeg"),
		filepath.Join(s.profile.Data, thumbnailCacheFolder, attachment.UID+".v2.jpeg.failed"),
		filepath.Join(s.profile.Data, thumbnailCacheFolder, attachment.UID+".jpeg"),
		filepath.Join(s.profile.Data, motionCacheFolder, attachment.UID+".mp4"),
	} {
		if err := os.Remove(cachePath); err != nil && !os.IsNotExist(err) {
			slog.Warn("Failed to delete derived attachment cache", slog.String("path", cachePath), slog.Any("err", err))
		}
	}
}

func shouldFailDeleteAttachmentStorage(ctx context.Context) bool {
	failpoint, ok := ctx.Value(deleteAttachmentStorageFailpointKey{}).(bool)
	return ok && failpoint
}

func shouldFailCreateAttachmentPolicy(ctx context.Context) bool {
	failpoint, ok := ctx.Value(createAttachmentPolicyFailpointKey{}).(bool)
	return ok && failpoint
}

func shouldFailCreateAttachmentPostCommit(ctx context.Context) bool {
	failpoint, ok := ctx.Value(createAttachmentPostCommitFailpointKey{}).(bool)
	return ok && failpoint
}

// GetAttachmentStorageUsage returns the total persisted attachment size for a
// creator in bytes. External links are not counted.
func (s *Store) GetAttachmentStorageUsage(ctx context.Context, creatorID int32) (int64, error) {
	return s.driver.GetAttachmentStorageUsage(ctx, creatorID)
}
