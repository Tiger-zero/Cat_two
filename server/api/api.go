package api

import (
	"context"
	"log/slog"
	"mime"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"connectrpc.com/connect"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/pkg/errors"
	"golang.org/x/sync/semaphore"

	"github.com/usememos/memos/core/notification"
	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/internal/linkmeta"
	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/ratelimit"
	"github.com/usememos/memos/markdown"
	apipb "github.com/usememos/memos/proto/gen/api"
	"github.com/usememos/memos/proto/gen/api/apiconnect"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

// MaxAPIRequestBytes caps the size of a request body accepted by the API. The
// in-process MCP endpoint forwards every tool call through these same routes, so
// it derives its own limit from this constant to keep the two gates in lockstep.
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
	apipb.UnimplementedSpaceServiceServer
	apipb.UnimplementedAttachmentServiceServer
	apipb.UnimplementedIdentityProviderServiceServer

	Secret                  string
	Profile                 *profile.Profile
	Store                   *store.Store
	MarkdownService         markdown.Service
	SSEHub                  *SSEHub
	NotificationEmailSender notification.EmailSender
	NotificationSettings    NotificationSettingsService
	NetworkSettings         NetworkSettingsService

	// RateLimiter bounds request rates; nil disables every limit.
	RateLimiter ratelimit.Limiter
	// Challenge verifies proof-of-humanity tokens on signup and password
	// sign-in; nil means no challenge is configured.
	Challenge ratelimit.Challenge
	// SignupPolicy decides whether a self-service registration may proceed;
	// nil allows every registration.
	SignupPolicy ratelimit.SignupPolicy

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

// NewAPIService creates an API v1 service with its shared dependencies.
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
		NotificationEmailSender:  nil,
		RateLimiter:              newProfileRateLimiter(profile),
		thumbnailSemaphore:       semaphore.NewWeighted(3), // Limit to 3 concurrent thumbnail generations
		imageProcessingSemaphore: semaphore.NewWeighted(2),
	}
	service.linkMetadataFetcher = linkmeta.NewHTMLMetaFetcher()
	return service
}

// NewJSONMarshaler mirrors grpc-gateway's default JSON marshaler with one
// change: EmitDefaultValues replaces EmitUnpopulated. Both keep proto3 scalar
// defaults ("" / 0 / false) and empty lists in the payload — the generated
// OpenAPI schema declares several of them required — but EmitUnpopulated also
// writes `null` for every unset message field (e.g. Attachment.motion_media on
// a plain image). No schema marks those fields nullable, so a client that
// validates responses against the spec rejects them; the MCP tools serve the
// same schema as their outputSchema, and strict MCP clients fail every call
// that returns an attachment. EmitDefaultValues omits unset message fields
// instead, which the schema already allows.
func NewJSONMarshaler() *runtime.HTTPBodyMarshaler {
	return &runtime.HTTPBodyMarshaler{
		Marshaler: &runtime.JSONPb{
			EmitDefaultValues: true,
			DiscardUnknown:    true,
		},
	}
}

// NewHandler serves REST, Connect, and SSE with shared authentication policies.
func (s *APIService) NewHandler(ctx context.Context) (http.Handler, error) {
	// Shared authorizer: one source of truth for authentication and anonymous-access
	// policy, used by both the gRPC-Gateway middleware and the Connect interceptor.
	authorizer := NewAuthorizer(s.Store, s.Secret).WithRateLimiter(s.RateLimiter)

	// grpc-gateway does not hand the matched procedure to middleware:
	// runtime.RPCMethod is only populated by the generated handler, which runs
	// after middleware. Resolve the procedure from the proto HTTP bindings
	// instead so the policy check actually runs on this transport.
	routeResolver, err := newGatewayRouteResolver()
	if err != nil {
		return nil, errors.Wrap(err, "failed to build gateway route resolver")
	}

	gatewayAuthMiddleware := func(next runtime.HandlerFunc) runtime.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
			// grpc-gateway does not pass through the Connect metadata interceptor.
			// Apply the same no-store policy here so a memo that later loses access
			// cannot remain readable from a browser or intermediary response cache.
			setAPIResponseNoStoreHeaders(w.Header())
			ctx := r.Context()

			// An unresolved path yields an empty procedure, which CheckAccess
			// treats as protected: authenticated callers pass and anonymous ones
			// are refused. Failing closed keeps a routing gap from becoming an
			// access-control gap.
			procedure, _ := routeResolver.resolveRequest(r)
			if rejectsAuthBrowserOrigin(r, procedure, s.Profile.InstanceURL) {
				writeGatewayStatusError(w, status.Error(codes.PermissionDenied, authBrowserOriginError))
				return
			}
			result := authorizer.Authenticate(ctx, r.Header.Get("Authorization"))
			if s.isRetiredProcedure(procedure) {
				writeGatewayStatusError(w, retiredModuleError())
				return
			}
			if err := authorizer.CheckAccess(ctx, procedure, result); err != nil {
				writeGatewayAuthorizationError(w, err)
				return
			}
			if err := authorizer.Throttle(ctx, procedure, result); err != nil {
				writeGatewayStatusError(w, err)
				return
			}

			// Apply the identity to the context (no-op for permitted anonymous requests).
			if result != nil {
				r = r.WithContext(auth.ApplyToContext(ctx, result))
			}
			// A real browser cookie takes precedence over the older gateway
			// metadata header. Do not depend on header iteration order.
			if len(r.Header.Values("Cookie")) != 0 && len(r.Header.Values(gatewayMetadataCookieHeader)) != 0 {
				r = r.Clone(r.Context())
				r.Header.Del(gatewayMetadataCookieHeader)
			}
			r.Body = http.MaxBytesReader(w, r.Body, requestBodyLimit(procedure))

			next(w, r, pathParams)
		}
	}

	// Create gRPC-Gateway mux with auth middleware.
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, NewJSONMarshaler()),
		runtime.WithMiddlewares(gatewayAuthMiddleware),
		runtime.WithErrorHandler(gatewayErrorHandler),
		runtime.WithIncomingHeaderMatcher(gatewayIncomingHeaderMatcher),
		runtime.WithOutgoingHeaderMatcher(gatewayOutgoingHeaderMatcher),
	)
	if err := apipb.RegisterInstanceServiceHandlerServer(ctx, gwMux, s); err != nil {
		return nil, err
	}
	if err := apipb.RegisterAuthServiceHandlerServer(ctx, gwMux, s); err != nil {
		return nil, err
	}
	if err := s.registerTwoFactorRoutes(gwMux); err != nil {
		return nil, err
	}
	if err := apipb.RegisterUserServiceHandlerServer(ctx, gwMux, s); err != nil {
		return nil, err
	}
	if err := apipb.RegisterMemoServiceHandlerServer(ctx, gwMux, s); err != nil {
		return nil, err
	}
	if err := s.registerMemoTagRoutes(gwMux); err != nil {
		return nil, err
	}
	if err := apipb.RegisterSpaceServiceHandlerServer(ctx, gwMux, s); err != nil {
		return nil, err
	}
	if err := apipb.RegisterAttachmentServiceHandlerServer(ctx, gwMux, s); err != nil {
		return nil, err
	}
	if err := apipb.RegisterIdentityProviderServiceHandlerServer(ctx, gwMux, s); err != nil {
		return nil, err
	}

	// Connect handlers for browser clients (replaces grpc-web).
	logStacktraces := s.Profile.Demo
	connectInterceptors := connect.WithInterceptors(
		NewMetadataInterceptor(), // Convert HTTP headers to gRPC metadata first
		NewLoggingInterceptor(logStacktraces),
		NewRecoveryInterceptor(logStacktraces),
		connect.UnaryInterceptorFunc(s.rejectRetiredConnectProcedures),
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

	// Prefixes within a path segment (memos.api.*) are dispatched explicitly;
	// ServeMux wildcards match complete segments only.
	sseMux := http.NewServeMux()
	RegisterSSERoutes(sseMux, s.SSEHub, s.Store, s.Secret)
	trashHandler := s.newMemoTrashHandler(authorizer)
	mcpConnectionHandler := s.newMCPConnectionHandler(authorizer)
	loginBlocklistHandler := s.newLoginBlocklistHandler(authorizer)
	captureSettingsHandler := s.newCaptureSettingsHandler(authorizer)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := r.URL.Path
		if r.URL.RawPath != "" {
			requestPath = r.URL.RawPath
		}
		switch {
		case requestPath == "/api/sse" && r.Method == http.MethodGet:
			sseMux.ServeHTTP(w, r)
		case s.personalMode() && requestPath == sessionActivityPath:
			s.recordSessionActivity(w, r, authorizer)
		case requestPath == "/api/trash" || strings.HasPrefix(requestPath, "/api/trash/"):
			trashHandler.ServeHTTP(w, r)
		case s.Profile.Personal && (requestPath == "/api/mcpConnections" || strings.HasPrefix(requestPath, "/api/mcpConnections/")):
			mcpConnectionHandler.ServeHTTP(w, r)
		case s.personalMode() && (requestPath == loginBlocklistPath || strings.HasPrefix(requestPath, loginBlocklistPath+"/")):
			loginBlocklistHandler.ServeHTTP(w, r)
		case requestPath == notificationSettingsPath || requestPath == networkSettingsPath:
			captureSettingsHandler.ServeHTTP(w, r)
		case strings.HasPrefix(requestPath, "/api/v1/"):
			removedAPIVersionHandler(false).ServeHTTP(w, r)
		case strings.HasPrefix(requestPath, "/memos.api.v1."):
			removedAPIVersionHandler(true).ServeHTTP(w, r)
		case strings.HasPrefix(requestPath, "/memos.api."):
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
		case strings.HasPrefix(requestPath, "/api/") || strings.HasPrefix(requestPath, "/file/"):
			gwMux.ServeHTTP(w, r)
		default:
			httphandler.NotFound(w, r)
		}
	}), nil
}

// removedAPIMessage tells clients of the removed v1 API where it went.
const removedAPIMessage = "API v1 has been removed. Use /api/... for REST and /memos.api.<Service>/<Method> for Connect. " +
	"See https://github.com/usememos/memos/blob/main/proto/api/CHANGELOG.md"

// removedAPIVersionHandler answers requests to the removed /api/v1 and
// memos.api.v1 routes with 410 Gone, in the error body shape each transport's
// clients decode. It exists for one release so old clients fail with a
// pointer to the migration; delete it in the release after.
func removedAPIVersionHandler(connectProtocol bool) http.Handler {
	return httphandler.Func(func(w http.ResponseWriter, _ *http.Request) error {
		if connectProtocol {
			return httphandler.JSON(w, http.StatusGone, map[string]string{"code": connect.CodeUnimplemented.String(), "message": removedAPIMessage})
		}
		return httphandler.JSON(w, http.StatusGone, map[string]any{"code": int(codes.Unimplemented), "message": removedAPIMessage})
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

const gatewayMetadataCookieHeader = runtime.MetadataHeaderPrefix + "Cookie"

// gatewayIncomingHeaderMatcher forwards the challenge token and the headers
// used to read a refresh cookie and determine its security attributes.
func gatewayIncomingHeaderMatcher(key string) (string, bool) {
	switch http.CanonicalHeaderKey(key) {
	case challengeTokenHeader:
		return challengeTokenMetadataKey, true
	case "Cookie", "Origin", "X-Forwarded-Proto", "Forwarded", "X-Memos-Session-Id":
		return strings.ToLower(key), true
	}
	return runtime.DefaultHeaderMatcher(key)
}

// Set-Cookie is a browser-managed header. Prefixing it as ordinary gRPC
// metadata would expose the refresh token to scripts and leave it unstored.
func gatewayOutgoingHeaderMatcher(key string) (string, bool) {
	if strings.EqualFold(key, "Set-Cookie") {
		return "Set-Cookie", true
	}
	if strings.EqualFold(key, sessionIdleExpiryHeader) {
		return sessionIdleExpiryHeader, true
	}
	return runtime.MetadataHeaderPrefix + key, true
}

// gatewayErrorHandler adds the rate-limit header fields to a refusal before
// the default handler writes the status body.
func gatewayErrorHandler(ctx context.Context, mux *runtime.ServeMux, marshaler runtime.Marshaler, w http.ResponseWriter, r *http.Request, err error) {
	if st, ok := status.FromError(err); ok {
		for key, values := range rateLimitHTTPHeaders(st) {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
	}
	runtime.DefaultHTTPErrorHandler(ctx, mux, marshaler, w, r, err)
}

// writeGatewayStatusError writes a gRPC status as the gateway's JSON error body,
// including its details and any rate-limit header fields.
func writeGatewayStatusError(w http.ResponseWriter, err error) {
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
	w.WriteHeader(runtime.HTTPStatusFromCode(st.Code()))
	_, _ = w.Write(body)
}

func writeGatewayAuthorizationError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUnauthenticated) {
		writeGatewayStatusError(w, status.Error(codes.Unauthenticated, "authentication required"))
		return
	}
	slog.Error("failed to resolve API access policy", "error", err)
	writeGatewayStatusError(w, status.Error(codes.Internal, "failed to resolve API access policy"))
}
