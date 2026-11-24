package api

import (
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/server/auth"
)

func (s *APIService) newInternalHandler(authorizer *Authorizer) http.Handler {
	mux := http.NewServeMux()
	RegisterSSERoutes(mux, s.SSEHub, s.Store, s.Secret)
	s.registerTwoFactorRoutes(mux, authorizer)
	s.registerMemoTagRoutes(mux, authorizer)
	s.registerMemoTrashRoutes(mux, authorizer)
	if s.personalMode() {
		mux.HandleFunc(sessionActivityPath, func(w http.ResponseWriter, r *http.Request) {
			s.recordSessionActivity(w, r, authorizer)
		})
		s.registerMCPConnectionRoutes(mux, authorizer)
		s.registerLoginBlocklistRoutes(mux, authorizer)
	}
	s.registerCaptureSettingsRoutes(mux, authorizer)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setAPIResponseNoStoreHeaders(w.Header())
		if rejectsBrowserOrigin(r, s.Profile.InstanceURL) {
			writeHTTPStatusError(w, status.Error(codes.PermissionDenied, "browser origin is not allowed for this web request"))
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// requireWebSession authenticates a browser access token before decoding a
// request and applies the same authenticated request budget as Connect.
func requireWebSession(authorizer *Authorizer, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		result := authorizer.Authenticate(ctx, r.Header.Get("Authorization"))
		if result == nil || result.Claims == nil {
			writeHTTPAuthorizationError(w, ErrUnauthenticated)
			return
		}
		if err := authorizer.Throttle(ctx, "", result); err != nil {
			writeHTTPStatusError(w, err)
			return
		}
		next(w, r.WithContext(auth.ApplyToContext(ctx, result)))
	}
}

// HTTPStatusFromCode maps the shared service status to an HTTP response status.
func HTTPStatusFromCode(code codes.Code) int {
	switch code {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return 499
	case codes.InvalidArgument, codes.FailedPrecondition, codes.OutOfRange:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists, codes.Aborted:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
