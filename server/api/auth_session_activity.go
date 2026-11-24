package api

import (
	"net/http"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/server/auth"
)

const sessionActivityPath = "/internal/session/activity"

// recordSessionActivity accepts an edit notification from the browser holding
// the session. It never starts a session, and uses the server's clock.
func (s *APIService) recordSessionActivity(w http.ResponseWriter, r *http.Request, authorizer *Authorizer) {
	setAPIResponseNoStoreHeaders(w.Header())
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		httphandler.WriteError(w, r, &httphandler.Error{Code: http.StatusMethodNotAllowed, Message: http.StatusText(http.StatusMethodNotAllowed)})
		return
	}
	if r.Header.Get("Origin") == "" || rejectsBrowserOrigin(r, s.Profile.InstanceURL) {
		writeHTTPStatusError(w, status.Error(codes.PermissionDenied, authBrowserOriginError))
		return
	}
	ctx := r.Context()
	claims, err := authorizer.authenticator.AuthenticateByAccessTokenV2(ctx, auth.ExtractBearerToken(r.Header.Get("Authorization")))
	if errors.Is(err, auth.ErrSessionIdleExpired) {
		writeHTTPStatusError(w, status.Error(codes.Unauthenticated, auth.ErrSessionIdleExpired.Error()))
		return
	}
	if err != nil || claims == nil || claims.SessionID == "" {
		writeHTTPAuthorizationError(w, ErrUnauthenticated)
		return
	}
	if err := authorizer.Throttle(ctx, "", &auth.AuthResult{Claims: claims}); err != nil {
		writeHTTPStatusError(w, err)
		return
	}
	deadline, err := s.Store.RecordUserSessionEdit(ctx, claims.UserID, claims.SessionID)
	if err != nil {
		writeHTTPStatusError(w, status.Error(codes.Internal, "failed to record session activity"))
		return
	}
	if deadline.IsZero() {
		writeHTTPStatusError(w, status.Error(codes.Unauthenticated, auth.ErrSessionIdleExpired.Error()))
		return
	}
	_ = httphandler.JSON(w, http.StatusOK, map[string]string{"idleExpiresAt": deadline.UTC().Format(time.RFC3339Nano)})
}
