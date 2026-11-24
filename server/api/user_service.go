package api

import (
	"context"
	stderrors "errors"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	coreuser "github.com/usememos/memos/core/user"
	"github.com/usememos/memos/internal/clientip"
	"github.com/usememos/memos/internal/ratelimit"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/store"
)

const maxBatchGetUsers = 100

const (
	// maxAvatarImageBytes is the largest avatar image a user may set. It
	// matches the limit the web client enforces before encoding the file.
	maxAvatarImageBytes = 2 << 20
	// maxAvatarDataURIBytes bounds the stored data URI: the image bytes
	// base64-encoded (4/3 expansion) plus the "data:<type>;base64," prefix.
	maxAvatarDataURIBytes = maxAvatarImageBytes/3*4 + 256
)

func validateUserTagsSetting(setting *apipb.UserSetting_TagsSetting) error {
	if setting == nil {
		return errors.New("tags setting is required")
	}
	for tag, metadata := range setting.Tags {
		if strings.TrimSpace(tag) == "" {
			return errors.New("tag key cannot be empty")
		}
		if _, err := regexp.Compile(tag); err != nil {
			return errors.Wrapf(err, "tag key %q is not a valid regex pattern", tag)
		}
		if metadata == nil {
			return errors.Errorf("tag metadata is required for %q", tag)
		}
		if metadata.GetBackgroundColor() != nil {
			if err := validateInstanceColor(metadata.GetBackgroundColor()); err != nil {
				return errors.Wrapf(err, "background_color for %q", tag)
			}
		}
	}
	return nil
}

func (s *APIService) ListUsers(ctx context.Context, request *apipb.ListUsersRequest) (*apipb.ListUsersResponse, error) {
	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if currentUser == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if currentUser.Role != store.RoleAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	userFind := &store.FindUser{}

	if request.Filter != "" {
		username, err := extractUsernameFromFilter(request.Filter)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid filter: %v", err)
		}
		if username != "" {
			userFind.Username = &username
		}
	}

	var limit, offset int
	if request.PageToken != "" {
		var pageToken apipb.PageToken
		if err := unmarshalPageToken(request.PageToken, &pageToken); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "invalid page token: %v", err)
		}
		limit = normalizePageSize(pageToken.Limit)
		offset = max(int(pageToken.Offset), 0)
	} else {
		limit = normalizePageSize(request.PageSize)
	}
	// Fetch one extra row to detect whether a subsequent page exists.
	limitPlusOne := limit + 1
	userFind.Limit = &limitPlusOne
	userFind.Offset = &offset

	users, err := s.Store.ListUsers(ctx, userFind)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list users: %v", err)
	}

	nextPageToken := ""
	if len(users) == limitPlusOne {
		users = users[:limit]
		nextPageToken, err = getPageToken(limit, offset+limit)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "failed to get next page token: %v", err)
		}
	}

	response := &apipb.ListUsersResponse{
		Users:         make([]*apipb.User, 0, len(users)),
		NextPageToken: nextPageToken,
	}
	for _, user := range users {
		response.Users = append(response.Users, convertUserFromStore(user, currentUser))
	}
	return response, nil
}

// uniqueBatchUsernames parses user resource names into unique usernames,
// skipping empty entries.
func uniqueBatchUsernames(names []string) ([]string, error) {
	uniqueUsernames := make([]string, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		username, err := parseUsernameFromName(name)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[username]; ok {
			continue
		}
		seen[username] = struct{}{}
		uniqueUsernames = append(uniqueUsernames, username)
	}
	return uniqueUsernames, nil
}

func (s *APIService) BatchGetUsers(ctx context.Context, request *apipb.BatchGetUsersRequest) (*apipb.BatchGetUsersResponse, error) {
	if len(request.Names) == 0 {
		return &apipb.BatchGetUsersResponse{Users: []*apipb.User{}}, nil
	}

	nonEmptyNameCount := 0
	for _, name := range request.Names {
		if name != "" {
			nonEmptyNameCount++
		}
	}
	if nonEmptyNameCount > maxBatchGetUsers {
		return nil, status.Errorf(codes.InvalidArgument, "too many names (max %d)", maxBatchGetUsers)
	}
	s.chargeBatch(ctx, nonEmptyNameCount)

	uniqueUsernames, err := uniqueBatchUsernames(request.Names)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user name: %v", err)
	}

	if len(uniqueUsernames) == 0 {
		return &apipb.BatchGetUsersResponse{Users: []*apipb.User{}}, nil
	}

	normal := store.Normal
	users, err := s.Store.ListUsers(ctx, &store.FindUser{
		UsernameList: uniqueUsernames,
		RowStatus:    &normal,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list users: %v", err)
	}

	currentUser, _ := s.fetchCurrentUser(ctx)
	response := &apipb.BatchGetUsersResponse{
		Users: make([]*apipb.User, 0, len(users)),
	}
	for _, user := range users {
		response.Users = append(response.Users, convertUserFromStore(user, currentUser))
	}
	return response, nil
}

func (s *APIService) GetUser(ctx context.Context, request *apipb.GetUserRequest) (*apipb.User, error) {
	user, err := ResolveUserByName(ctx, s.Store, request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user name: %s", request.Name)
	}
	if user == nil {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}
	currentUser, _ := s.fetchCurrentUser(ctx)
	return convertUserFromStore(user, currentUser), nil
}

func (s *APIService) CreateUser(ctx context.Context, request *apipb.CreateUserRequest) (*apipb.User, error) {
	if request.User == nil {
		return nil, status.Errorf(codes.InvalidArgument, "user is required")
	}
	if request.UserId != "" && request.UserId != request.User.Username {
		return nil, status.Errorf(codes.InvalidArgument, "user_id must match user.username")
	}
	if err := coreuser.ValidateUsername(request.User.Username); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := coreuser.ValidatePassword(request.User.Password); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	email, err := store.NormalizeEmail(request.User.Email)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	const registrationClosed = "registration closed after initial administrator setup"
	users, err := s.Store.ListUsers(ctx, &store.FindUser{Limit: new(1)})
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to check initial administrator setup")
	}
	if len(users) != 0 {
		return nil, status.Error(codes.PermissionDenied, registrationClosed)
	}
	scope := ratelimit.ScopeSignupIP
	if request.ValidateOnly {
		scope = ratelimit.ScopeValidateIP
	}
	if err := s.throttleAndCharge(scope, clientip.FromContext(ctx), 1); err != nil {
		return nil, err
	}
	if request.ValidateOnly {
		return &apipb.User{Username: request.User.Username, Role: apipb.User_ADMIN, Email: email, DisplayName: request.User.DisplayName}, nil
	}
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.User.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to protect administrator password")
	}
	user, created, err := s.Store.CreateUserIfNoUsers(ctx, &store.User{
		Username: request.User.Username, Role: store.RoleAdmin, Email: email,
		Nickname: request.User.DisplayName, PasswordHash: string(passwordHash),
	})
	if err != nil {
		return nil, convertUserWriteError(err, "failed to create initial administrator")
	}
	if !created {
		return nil, status.Error(codes.PermissionDenied, registrationClosed)
	}
	return convertUserFromStore(user, user), nil
}

func (s *APIService) UpdateUser(ctx context.Context, request *apipb.UpdateUserRequest) (*apipb.User, error) {
	if request.User == nil {
		return nil, status.Errorf(codes.InvalidArgument, "user is required")
	}
	if request.UpdateMask == nil || len(request.UpdateMask.Paths) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "update mask is empty")
	}
	user, err := ResolveUserByName(ctx, s.Store, request.User.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user name: %v", err)
	}
	if user == nil {
		if request.AllowMissing {
			return nil, status.Errorf(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.NotFound, "user not found")
	}
	userID := user.ID
	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if currentUser == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	// Check permission.
	// Only allow admin or self to update user.
	if currentUser.ID != userID && currentUser.Role != store.RoleAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}

	if err := s.checkManagedUserChange(ctx, user, request); err != nil {
		return nil, err
	}

	currentTsSec := time.Now().Unix()
	update := &store.UpdateUser{
		ID:        user.ID,
		UpdatedTs: &currentTsSec,
	}
	instanceGeneralSetting, err := s.Store.GetInstanceGeneralSetting(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get instance general setting: %v", err)
	}
	for _, field := range request.UpdateMask.Paths {
		switch field {
		case "username":
			if instanceGeneralSetting.DisallowChangeUsername {
				return nil, status.Errorf(codes.PermissionDenied, "permission denied: disallow change username")
			}
			if err := coreuser.ValidateUsername(request.User.Username); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "%v", err)
			}
			update.Username = &request.User.Username
		case "display_name":
			if instanceGeneralSetting.DisallowChangeNickname {
				return nil, status.Errorf(codes.PermissionDenied, "permission denied: disallow change nickname")
			}
			update.Nickname = &request.User.DisplayName
		case "email":
			email, err := store.NormalizeEmail(request.User.Email)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "%v", err)
			}
			update.Email = &email
		case "avatar_url":
			// Validate avatar MIME type to prevent XSS during upload
			if request.User.AvatarUrl != "" {
				if len(request.User.AvatarUrl) > maxAvatarDataURIBytes {
					return nil, status.Errorf(codes.InvalidArgument, "avatar exceeds the maximum size of %d bytes", maxAvatarImageBytes)
				}
				imageType, _, err := extractImageInfo(request.User.AvatarUrl)
				if err != nil {
					return nil, status.Errorf(codes.InvalidArgument, "invalid avatar format: %v", err)
				}
				// Only allow safe image formats for avatars
				allowedAvatarTypes := map[string]bool{
					"image/png":  true,
					"image/jpeg": true,
					"image/jpg":  true,
					"image/gif":  true,
					"image/webp": true,
				}
				if !allowedAvatarTypes[imageType] {
					return nil, status.Errorf(codes.InvalidArgument, "invalid avatar image type: %s. Only PNG, JPEG, GIF, and WebP are allowed", imageType)
				}
			}
			update.AvatarURL = &request.User.AvatarUrl
		case "description":
			update.Description = &request.User.Description
		case "role":
			// Only allow admin to update role.
			if currentUser.Role != store.RoleAdmin {
				return nil, status.Errorf(codes.PermissionDenied, "permission denied")
			}
			role := convertUserRoleToStore(request.User.Role)
			update.Role = &role
		case "password":
			if err := coreuser.ValidatePassword(request.User.Password); err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "%v", err)
			}
			passwordHash, err := bcrypt.GenerateFromPassword([]byte(request.User.Password), bcrypt.DefaultCost)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to generate password hash: %v", err)
			}
			passwordHashStr := string(passwordHash)
			update.PasswordHash = &passwordHashStr
		case "state":
			if currentUser.Role != store.RoleAdmin {
				return nil, status.Errorf(codes.PermissionDenied, "permission denied")
			}
			rowStatus := convertStateToStore(request.User.State)
			update.RowStatus = &rowStatus
		default:
			return nil, status.Errorf(codes.InvalidArgument, "invalid update path: %s", field)
		}
	}

	if update.PasswordHash != nil && currentUser.ID == user.ID {
		update.KeepSessionID = currentSessionID(ctx, s.Secret, user.ID)
	}
	updatedUser, err := s.Store.UpdateUser(ctx, update)
	if err != nil {
		if stderrors.Is(err, store.ErrLastInstanceAdmin) {
			return nil, status.Error(codes.FailedPrecondition, lastInstanceAdminMessage)
		}
		return nil, convertUserWriteError(err, "failed to update user")
	}
	return convertUserFromStore(updatedUser, currentUser), nil
}

func (s *APIService) DeleteUser(ctx context.Context, request *apipb.DeleteUserRequest) (*emptypb.Empty, error) {
	user, err := ResolveUserByName(ctx, s.Store, request.Name)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid user name: %v", err)
	}
	if user == nil {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}
	userID := user.ID
	currentUser, err := s.fetchCurrentUser(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %v", err)
	}
	if currentUser == nil {
		return nil, status.Errorf(codes.Unauthenticated, "user not authenticated")
	}
	if currentUser.ID != userID && currentUser.Role != store.RoleAdmin {
		return nil, status.Errorf(codes.PermissionDenied, "permission denied")
	}
	isSelfDelete := currentUser.ID == userID
	if err := s.checkManagedExistence(ctx, BuildUserName(user.Username)); err != nil {
		return nil, err
	}
	if err := s.checkManagedReference(ctx, BuildUserName(user.Username)); err != nil {
		return nil, err
	}

	deleteResult, err := s.Store.DeleteUser(ctx, &store.DeleteUser{
		ID: user.ID,
	})
	if err != nil {
		if stderrors.Is(err, store.ErrLastInstanceAdmin) {
			return nil, status.Error(codes.FailedPrecondition, lastInstanceAdminMessage)
		}
		return nil, status.Errorf(codes.Internal, "failed to delete user: %v", err)
	}
	if isSelfDelete {
		if err := s.clearAuthCookies(ctx); err != nil {
			slog.Warn("failed to clear auth cookies after self delete", "user_id", userID, "error", err)
		}
	}
	if err := s.cleanupDeletedAttachmentStorage(ctx, deleteResult.Attachments); err != nil {
		return nil, status.Errorf(codes.Internal, "user was deleted but attachment storage cleanup failed: %v", err)
	}

	return &emptypb.Empty{}, nil
}

const emailTakenMessage = "email is already in use"

const lastInstanceAdminMessage = "the instance must retain an active administrator"

// checkManagedUserChange refuses an update that changes the fields of a user
// the configuration manages: username, role, and the profile fields its file
// declares. The password is write-only and never refused. A rename is also
// refused while a managed resource names the user.
func (s *APIService) checkManagedUserChange(ctx context.Context, user *store.User, request *apipb.UpdateUserRequest) error {
	changed := []string{}
	for _, path := range request.UpdateMask.Paths {
		switch path {
		case "username":
			if request.User.Username != user.Username {
				changed = append(changed, "username")
			}
		case "display_name":
			if request.User.DisplayName != user.Nickname {
				changed = append(changed, "displayName")
			}
		case "email":
			if email, err := store.NormalizeEmail(request.User.Email); err != nil || email != user.Email {
				changed = append(changed, "email")
			}
		case "role":
			if convertUserRoleToStore(request.User.Role) != user.Role {
				changed = append(changed, "role")
			}
		default:
		}
	}
	if slices.Contains(changed, "username") {
		if err := s.checkManagedReference(ctx, BuildUserName(user.Username)); err != nil {
			return err
		}
	}
	return s.checkManagedChange(ctx, BuildUserName(user.Username), changed)
}

// convertUserWriteError maps the store's uniqueness sentinels onto
// AlreadyExists and everything else onto Internal with the given context.
func convertUserWriteError(err error, context string) error {
	switch {
	case stderrors.Is(err, store.ErrEmailTaken):
		return status.Error(codes.AlreadyExists, emailTakenMessage)
	case stderrors.Is(err, store.ErrUsernameTaken):
		return status.Error(codes.AlreadyExists, "username is already in use")
	default:
		return status.Errorf(codes.Internal, "%s: %v", context, err)
	}
}

func getDefaultUserGeneralSetting() *apipb.UserSetting_GeneralSetting {
	return &apipb.UserSetting_GeneralSetting{
		Locale:         "zh-Hans",
		MemoVisibility: "PRIVATE",
		Theme:          "",
	}
}

func (s *APIService) resolveUserFromName(ctx context.Context, name string) (*store.User, error) {
	user, err := ResolveUserByName(ctx, s.Store, name)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.Errorf("user not found: %s", name)
	}
	return user, nil
}

func (s *APIService) resolveUserAndSettingKeyFromName(ctx context.Context, name string) (*store.User, string, error) {
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "users" || parts[2] != "settings" {
		return nil, "", errors.Errorf("invalid resource name format: %s", name)
	}

	user, err := s.resolveUserFromName(ctx, BuildUserName(parts[1]))
	if err != nil {
		return nil, "", err
	}
	return user, parts[3], nil
}
