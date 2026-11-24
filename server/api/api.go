package api

import (
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"connectrpc.com/connect"
	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"

	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/internal/linkmeta"
	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/ratelimit"
	"github.com/usememos/memos/markdown"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
	"github.com/usememos/memos/store"
)

// MaxAPIRequestBytes caps the size of a request body accepted by the API. The
// in-process MCP transport also shares this bound.
const MaxAPIRequestBytes = 256 << 20

// maxGeneralRequestBytes caps every procedure that does not carry file bytes.
// Request bodies are buffered in memory before they are parsed, so the cap is
// what bounds memory per in-flight request; it leaves ample room for the
// largest memo content and settings an instance can be configured to accept.
const maxGeneralRequestBytes = 16 << 20

// maxAuthRequestBytes bounds small authentication and account setup messages
// that may reach a decoder before the client is authenticated.
const maxAuthRequestBytes = 64 << 10

// requestBodyLimit returns the request body cap for a procedure. Chunked
// uploads carry at most one chunk per call, so they get a much lower cap.
func requestBodyLimit(procedure string) int64 {
	switch procedure {
	case attachmentUploadProcedure, importMemosProcedure:
		return uploadRequestLimit
	case apiconnect.AttachmentServiceCreateAttachmentProcedure:
		return MaxAPIRequestBytes
	case apiconnect.AuthServiceSignInProcedure, apiconnect.AuthServiceRefreshTokenProcedure,
		apiconnect.AuthServiceSignOutProcedure, apiconnect.AuthServiceGetCurrentUserProcedure,
		apiconnect.UserServiceCreateUserProcedure:
		return maxAuthRequestBytes
	default:
		return maxGeneralRequestBytes
	}
}

// isInlineAttachmentRPC identifies the accepted unary wire formats for legacy
// attachment upload. Connect still handles unsupported methods, media types,
// and protocol versions.
func isInlineAttachmentRPC(r *http.Request, errorWriter *connect.ErrorWriter) bool {
	if r.Method != http.MethodPost || r.URL.Path != apiconnect.AttachmentServiceCreateAttachmentProcedure || !errorWriter.IsSupported(r) {
		return false
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return false
	}
	switch mediaType {
	case "application/json", "application/grpc+json", "application/grpc-web+json":
		return len(params) == 0 || len(params) == 1 && strings.EqualFold(params["charset"], "utf-8")
	case "application/proto", "application/grpc", "application/grpc+proto", "application/grpc-web", "application/grpc-web+proto":
		return len(params) == 0
	default:
		return false
	}
}

type APIService struct {
	apipb.UnimplementedInstanceServiceServer
	apipb.UnimplementedAuthServiceServer
	apipb.UnimplementedUserServiceServer
	apipb.UnimplementedMemoServiceServer
	apipb.UnimplementedAttachmentServiceServer

	Secret               string
	Profile              *profile.Profile
	Store                *store.Store
	MarkdownService      markdown.Service
	SSEHub               *SSEHub
	NotificationSettings NotificationSettingsService
	NetworkSettings      NetworkSettingsService

	// RateLimiter bounds request rates; nil disables every limit.
	RateLimiter ratelimit.Limiter
	// Challenge verifies proof-of-humanity tokens on signup and password
	// sign-in; nil means no challenge is configured.
	Challenge ratelimit.Challenge

	// thumbnailSemaphore limits concurrent thumbnail generation to prevent memory exhaustion
	thumbnailSemaphore       *semaphore.Weighted
	imageProcessingSemaphore *semaphore.Weighted

	// instanceStatsCache memoizes GetInstanceStats results for instanceStatsCacheTTL.
	instanceStatsCache instanceStatsCache

	linkMetadataFetcher linkMetadataFetcher
	attachmentUploads   attachmentUploads
	attachmentReplaces  attachmentReplaceLocks
	memoImports         memoImports
}

// NewAPIService initializes the shared website and tool service.
func NewAPIService(secret string, profile *profile.Profile, store *store.Store) *APIService {
	markdownService := markdown.NewService(
		markdown.WithTagExtension(),
		markdown.WithMentionExtension(),
	)
	service := &APIService{
		Secret:                   secret,
		Profile:                  profile,
		Store:                    store,
		MarkdownService:          markdownService,
		SSEHub:                   NewSSEHub(),
		RateLimiter:              newProfileRateLimiter(profile),
		thumbnailSemaphore:       semaphore.NewWeighted(3), // Limit to 3 concurrent thumbnail generations
		imageProcessingSemaphore: semaphore.NewWeighted(2),
	}
	service.linkMetadataFetcher = linkmeta.NewHTMLMetaFetcher()
	return service
}

// NewHandler serves the website's Connect and session-scoped HTTP requests.
func (s *APIService) NewHandler() http.Handler {
	authorizer := NewAuthorizer(s.Store, s.Secret).WithRateLimiter(s.RateLimiter)

	// Connect handlers for browser clients.
	logStacktraces := s.Profile.Demo
	connectInterceptors := connect.WithInterceptors(
		NewMetadataInterceptor(), // Convert HTTP headers to gRPC metadata first
		NewLoggingInterceptor(logStacktraces),
		NewRecoveryInterceptor(logStacktraces),
		connect.UnaryInterceptorFunc(rejectRetiredConnectProcedures),
		NewAuthInterceptor(authorizer),
	)
	connectMux := http.NewServeMux()
	connectErrors := connect.NewErrorWriter()
	connectHandler := NewConnectServiceHandler(s)
	connectHandler.RegisterConnectHandlers(connectMux, connectInterceptors,
		// Bound the decompressed message as well as the wire bytes below.
		connect.WithConditionalHandlerOptions(func(spec connect.Spec) []connect.HandlerOption {
			return []connect.HandlerOption{connect.WithReadMaxBytes(int(requestBodyLimit(spec.Procedure)))}
		}))

	internalMux := s.newInternalHandler(authorizer)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/internal/"):
			internalMux.ServeHTTP(w, r)
		case strings.HasPrefix(r.URL.Path, "/memos.api."):
			if r.Method == http.MethodPost && rejectsAuthBrowserOrigin(r, r.URL.Path, s.Profile.InstanceURL) && connectErrors.IsSupported(r) {
				setAPIResponseNoStoreHeaders(w.Header())
				_ = connectErrors.Write(w, r, connect.NewError(connect.CodePermissionDenied, errors.New(authBrowserOriginError)))
				return
			}
			// Unary interceptors run after Connect buffers and decodes a
			// message. Gate the largest request before that work and let the
			// interceptor recheck the credentials before serving it.
			if isInlineAttachmentRPC(r, connectErrors) &&
				!authorizer.authenticator.VerifyBearer(r.Context(), r.Header.Get("Authorization")) {
				setAPIResponseNoStoreHeaders(w.Header())
				_ = connectErrors.Write(w, r, newAuthorizationConnectError(ErrUnauthenticated))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, requestBodyLimit(r.URL.Path))
			connectMux.ServeHTTP(w, r)
		default:
			httphandler.NotFound(w, r)
		}
	})
}

func setAPIResponseNoStoreHeaders(header http.Header) {
	header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	header.Set("Pragma", "no-cache")
	header.Set("Expires", "0")
}

// newProfileRateLimiter builds the default limiter, or nil when the profile
// turns rate limiting off.
func newProfileRateLimiter(profile *profile.Profile) ratelimit.Limiter {
	if !profile.RateLimit {
		return nil
	}
	return ratelimit.NewMemoryLimiter(ratelimit.DefaultPolicy())
}

// writeHTTPStatusError writes a structured status for a native web route,
// including its details and any rate-limit header fields.
func writeHTTPStatusError(w http.ResponseWriter, err error) {
	st := status.Convert(err)
	for key, values := range rateLimitHTTPHeaders(st) {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	body, marshalErr := (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(st.Proto())
	if marshalErr != nil {
		http.Error(w, `{"code": 13, "message": "failed to encode error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(HTTPStatusFromCode(st.Code()))
	_, _ = w.Write(body)
}

func writeHTTPAuthorizationError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUnauthenticated) {
		writeHTTPStatusError(w, status.Error(codes.Unauthenticated, "authentication required"))
		return
	}
	slog.Error("failed to resolve API access policy", "error", err)
	writeHTTPStatusError(w, status.Error(codes.Internal, "failed to resolve API access policy"))
}
