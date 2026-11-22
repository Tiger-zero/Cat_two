package api

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

// authorizeAdminSession is used by operational settings that require an active
// administrator session and must not be available to personal access tokens.
func (s *APIService) authorizeAdminSession(authorizer *Authorizer, handler func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setAPIResponseNoStoreHeaders(w.Header())
		ctx := r.Context()
		result := authorizer.Authenticate(ctx, r.Header.Get("Authorization"))
		if result == nil {
			writeGatewayAuthorizationError(w, ErrUnauthenticated)
			return
		}
		if result.Claims == nil {
			writeGatewayStatusError(w, status.Error(codes.PermissionDenied, "a web administrator session is required"))
			return
		}
		if err := authorizer.Throttle(ctx, "", result); err != nil {
			writeGatewayStatusError(w, err)
			return
		}
		ctx = auth.ApplyToContext(ctx, result)
		user, err := s.fetchCurrentUser(ctx)
		if err != nil {
			writeGatewayStatusError(w, status.Error(codes.Internal, "failed to check the administrator session"))
			return
		}
		if user == nil || user.Role != store.RoleAdmin {
			writeGatewayStatusError(w, status.Error(codes.PermissionDenied, "a web administrator session is required"))
			return
		}
		if err := handler(w, r.WithContext(ctx)); err != nil {
			writeGatewayStatusError(w, err)
		}
	}
}
