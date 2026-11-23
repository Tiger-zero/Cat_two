package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/internal/ratelimit"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

const (
	maxCaptureIDBytes   = 256
	maxCaptureTextBytes = 64 << 10
)

// CaptureMemo saves an authorized transport delivery to the personal workspace.
// The caller verifies the transport identity and delivery before calling.
func (s *APIService) CaptureMemo(ctx context.Context, channel, externalID, text string) error {
	if !s.personalMode() || s.Profile.Demo {
		return status.Error(codes.FailedPrecondition, "memo capture requires a personal instance outside demo mode")
	}
	var tag string
	switch channel {
	case "telegram":
		tag = "#Telegram"
	case "wechat":
		tag = "#微信"
	default:
		return status.Error(codes.InvalidArgument, "unknown memo capture channel")
	}
	if len(externalID) > maxCaptureIDBytes || strings.TrimSpace(externalID) == "" {
		return status.Error(codes.InvalidArgument, "invalid memo capture ID")
	}
	if len(text) > maxCaptureTextBytes || !utf8.ValidString(text) || strings.TrimSpace(text) == "" {
		return status.Error(codes.InvalidArgument, "invalid memo capture text")
	}

	admins, err := s.Store.ListUsers(ctx, &store.FindUser{
		Role:      new(store.RoleAdmin),
		RowStatus: new(store.Normal),
		Limit:     new(2),
	})
	if err != nil {
		return status.Error(codes.Internal, "failed to resolve memo capture owner")
	}
	if len(admins) != 1 {
		return status.Error(codes.FailedPrecondition, "memo capture requires exactly one active administrator")
	}
	owner := admins[0]
	if err := s.throttleAndCharge(ratelimit.ScopeWriteUser, userKey(owner.ID), 1); err != nil {
		return err
	}

	uid := capturedMemoUID(channel, externalID)
	input := &apipb.Memo{
		// Keeping the tag before the delivery makes it independent of Markdown fences in the message.
		Content:         tag + "\n\n" + text,
		Visibility:      apipb.Visibility_PRIVATE,
		CreationChannel: channel,
	}
	prepared, err := s.prepareMemoCreate(ctx, owner, input, uid, false)
	if code := status.Code(err); code == codes.InvalidArgument || code == codes.FailedPrecondition {
		// Transports carry text and cannot bind instance-managed attachments.
		// Preserve source with indentation when copied markup requires a binding
		// or contains an invalid managed image URL.
		indent := strings.NewReplacer("\r\n", "\r\n    ", "\r", "\r    ", "\n", "\n    ")
		input.Content = tag + "\n\n    " + indent.Replace(text)
		prepared, err = s.prepareMemoCreate(ctx, owner, input, uid, false)
	}
	if err != nil {
		return err
	}
	if err := s.createMemoWithMutation(ctx, owner, prepared.memo, nil, prepared.attachments, prepared.requiredAttachmentIDs, prepared.referenceRelations, false); err != nil {
		err := mapMemoCreateError(err, uid, "failed to create captured memo")
		if status.Code(err) == codes.AlreadyExists {
			// The unique memo UID also covers archived and trashed notes.
			return nil
		}
		return err
	}
	s.SSEHub.publishMemoChanged()
	return nil
}

func capturedMemoUID(channel, externalID string) string {
	sum := sha256.Sum256([]byte(channel + "\x00" + externalID))
	// The prefix and 112 hash bits fit the existing 36-character memo ID limit.
	return "capture-" + hex.EncodeToString(sum[:14])
}
