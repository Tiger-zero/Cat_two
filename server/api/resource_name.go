package api

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/lithammer/shortuuid/v4"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/core/instancesetting"
	coreuser "github.com/usememos/memos/core/user"
	"github.com/usememos/memos/internal/identifier"
)

const (
	InstanceSettingNamePrefix = instancesetting.NamePrefix
	UserNamePrefix            = coreuser.NamePrefix
	MemoNamePrefix            = "memos/"
	MemoShareNamePrefix       = "shares/"
	AttachmentNamePrefix      = "attachments/"
	ReactionNamePrefix        = "reactions/"
)

// GetNameParentTokens returns the tokens from a resource name.
func GetNameParentTokens(name string, tokenPrefixes ...string) ([]string, error) {
	parts := strings.Split(name, "/")
	if len(parts) != 2*len(tokenPrefixes) {
		return nil, errors.Errorf("invalid request %q", name)
	}

	var tokens []string
	for i, tokenPrefix := range tokenPrefixes {
		if fmt.Sprintf("%s/", parts[2*i]) != tokenPrefix {
			return nil, errors.Errorf("invalid prefix %q in request %q", tokenPrefix, name)
		}
		if parts[2*i+1] == "" {
			return nil, errors.Errorf("invalid request %q with empty prefix %q", name, tokenPrefix)
		}
		tokens = append(tokens, parts[2*i+1])
	}
	return tokens, nil
}

func ExtractInstanceSettingKeyFromName(name string) (string, error) {
	const prefix = "instance/settings/"
	if !strings.HasPrefix(name, prefix) {
		return "", errors.Errorf("invalid instance setting name: expected prefix %q, got %q", prefix, name)
	}

	settingKey := strings.TrimPrefix(name, prefix)
	if settingKey == "" {
		return "", errors.Errorf("invalid instance setting name: empty setting key in %q", name)
	}

	// Ensure there are no additional path segments
	if strings.Contains(settingKey, "/") {
		return "", errors.Errorf("invalid instance setting name: setting key cannot contain '/' in %q", name)
	}

	return settingKey, nil
}

// ExtractUserIDFromName returns the uid from a resource name.
func ExtractUserIDFromName(name string) (int32, error) {
	tokens, err := GetNameParentTokens(name, UserNamePrefix)
	if err != nil {
		return 0, err
	}
	id, err := parseInt32(tokens[0])
	if err != nil {
		return 0, errors.Errorf("invalid user ID %q", tokens[0])
	}
	return id, nil
}

// ExtractMemoUIDFromName returns the memo UID from a resource name.
// e.g., "memos/uuid" -> "uuid".
func ExtractMemoUIDFromName(name string) (string, error) {
	tokens, err := GetNameParentTokens(name, MemoNamePrefix)
	if err != nil {
		return "", err
	}
	if !identifier.UIDMatcher.MatchString(tokens[0]) {
		return "", errors.New("invalid memo UID")
	}
	return tokens[0], nil
}

func buildMemoName(uid string) string {
	return MemoNamePrefix + uid
}

// ExtractAttachmentUIDFromName returns the attachment UID from a resource name.
func ExtractAttachmentUIDFromName(name string) (string, error) {
	tokens, err := GetNameParentTokens(name, AttachmentNamePrefix)
	if err != nil {
		return "", err
	}
	if !identifier.UIDMatcher.MatchString(tokens[0]) {
		return "", errors.New("invalid attachment UID")
	}
	return tokens[0], nil
}

// ExtractMemoReactionIDFromName returns the memo UID and reaction ID from a resource name.
// e.g., "memos/abc/reactions/123" -> ("abc", 123).
func ExtractMemoReactionIDFromName(name string) (string, int32, error) {
	tokens, err := GetNameParentTokens(name, MemoNamePrefix, ReactionNamePrefix)
	if err != nil {
		return "", 0, err
	}
	memoUID := tokens[0]
	reactionID, err := parseInt32(tokens[1])
	if err != nil {
		return "", 0, errors.Errorf("invalid reaction ID %q", tokens[1])
	}
	return memoUID, reactionID, nil
}

// ValidateAndGenerateUID validates a user-provided UID or generates a new one.
// If provided is empty, a new shortuuid is generated.
// If provided is non-empty, it is validated against identifier.UIDMatcher.
func ValidateAndGenerateUID(provided string) (string, error) {
	uid := strings.TrimSpace(provided)
	if uid == "" {
		return shortuuid.New(), nil
	}
	if !identifier.UIDMatcher.MatchString(uid) {
		return "", status.Errorf(codes.InvalidArgument, "invalid UID: must be 1-36 characters, contain only letters, digits, or hyphens, and start and end with a letter or digit")
	}
	return uid, nil
}

func parseInt32(src string) (int32, error) {
	parsed, err := strconv.ParseInt(src, 10, 32)
	if err != nil {
		return 0, err
	}
	return int32(parsed), nil
}
