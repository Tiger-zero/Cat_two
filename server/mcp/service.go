package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/usememos/memos/core/mcpconnection"
	"github.com/usememos/memos/internal/clientip"
	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/internal/profile"
	"github.com/usememos/memos/internal/ratelimit"
	memosproto "github.com/usememos/memos/proto"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

// MCP carries note and filter text. Attachments are uploaded through REST.
const maxMCPRequestBytes int64 = 16 << 20

// toolCatalogTTL is the freshness hint advertised on tools/list and
// server/discover results. The catalog is fixed at startup, so clients may cache
// it for a long time instead of re-listing on every session.
const toolCatalogTTL = 24 * time.Hour

// MCPService serves the OpenAPI-driven MCP endpoint.
type MCPService struct {
	profile *profile.Profile
	store   *store.Store
	// RateLimiter shares API request budgets and bounds MCP transport requests.
	RateLimiter ratelimit.Limiter

	handler http.Handler
}

// NewMCPService connects the personal tool catalog to the typed API service.
func NewMCPService(profile *profile.Profile, apiService *api.APIService) (*MCPService, error) {
	if apiService == nil || apiService.Store == nil {
		return nil, errors.New("MCP connection authentication requires the API service and store")
	}
	spec, err := loadMCPServiceOpenAPISpec()
	if err != nil {
		return nil, err
	}
	registry, err := buildOperationRegistry(spec, curatedOperationIDs...)
	if err != nil {
		return nil, err
	}
	tools, operationsByTool, err := buildCuratedTools(registry)
	if err != nil {
		return nil, err
	}

	version := "dev"
	if profile != nil && profile.Version != "" {
		version = profile.Version
	}
	server := sdkmcp.NewServer(&sdkmcp.Implementation{
		Name:    "memos",
		Version: version,
	}, &sdkmcp.ServerOptions{
		// The tool catalog never changes after startup, so do not advertise
		// listChanged: new-protocol clients would otherwise hold a
		// subscriptions/listen stream open for notifications that never come.
		// Leaving Logging unset also drops the deprecated logging capability.
		Capabilities: &sdkmcp.ServerCapabilities{
			Tools: &sdkmcp.ToolCapabilities{},
		},
	})
	server.AddReceivingMiddleware(catalogCacheMiddleware)

	service := &MCPService{profile: profile, store: apiService.Store, RateLimiter: apiService.RateLimiter}
	execute := personalAPI{service: apiService, marshaler: api.NewJSONMarshaler()}.execute
	for _, tool := range tools {
		server.AddTool(tool, newMCPToolHandler(execute, operationsByTool[tool.Name]))
	}
	registerPersonalMemoSearch(server, execute, operationsByTool["memo_list_memos"])
	registerPersonalMemoBatchGet(server, apiService)
	registerPersonalTagTools(server, apiService)
	server.AddReceivingMiddleware(service.authorizePersonalTools)

	streamableHandler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		return server
	}, &sdkmcp.StreamableHTTPOptions{
		// Stateless is also what lets the SDK serve protocol version
		// 2026-07-28; stateful handlers negotiate down to 2025-11-25.
		Stateless:           true,
		JSONResponse:        true,
		MaxRequestBodyBytes: maxMCPRequestBytes,
		// memos is typically served behind a reverse proxy with the app bound to a
		// loopback address while the public Host header is a real domain. The SDK's
		// DNS-rebinding guard treats that shape as an attack and rejects every
		// request with 403 ("invalid Host header"). Disable it and rely on memos'
		// own Origin/Host allowlist (see RegisterRoutes -> isAllowedMCPOrigin) for
		// CSRF / DNS-rebinding protection instead.
		DisableLocalhostProtection: true,
	})
	service.handler = streamableHandler
	return service, nil
}

// catalogCacheMiddleware stamps the static-catalog TTL onto results the SDK
// would otherwise mark immediately stale (ttlMs: 0).
func catalogCacheMiddleware(next sdkmcp.MethodHandler) sdkmcp.MethodHandler {
	return func(ctx context.Context, method string, request sdkmcp.Request) (sdkmcp.Result, error) {
		result, err := next(ctx, method, request)
		if err != nil {
			return result, err
		}
		switch typed := result.(type) {
		case *sdkmcp.ListToolsResult:
			typed.TTLMs = int(toolCatalogTTL.Milliseconds())
		case *sdkmcp.DiscoverResult:
			typed.TTLMs = int(toolCatalogTTL.Milliseconds())
		default:
			// Other results carry no catalog TTL.
		}
		return result, nil
	}
}

func loadMCPServiceOpenAPISpec() (*openAPISpec, error) {
	spec := &openAPISpec{}
	if err := yaml.Unmarshal(memosproto.OpenAPIYAML(), spec); err != nil {
		return nil, errors.Wrap(err, "failed to parse embedded OpenAPI spec")
	}
	if spec.Paths == nil {
		return nil, errors.New("embedded OpenAPI spec has no paths")
	}
	return spec, nil
}

type toolExecutor func(context.Context, *openAPIOperation, map[string]any) (*sdkmcp.CallToolResult, error)

func newMCPToolHandler(execute toolExecutor, operation *registeredOperation) sdkmcp.ToolHandler {
	return func(ctx context.Context, request *sdkmcp.CallToolRequest) (*sdkmcp.CallToolResult, error) {
		arguments := map[string]any{}
		if request.Params != nil && len(request.Params.Arguments) > 0 {
			if err := json.Unmarshal(request.Params.Arguments, &arguments); err != nil {
				return newToolErrorResult(errors.Wrap(err, "failed to decode MCP tool arguments").Error()), nil
			}
		}
		if err := validateToolArguments(operation.InputSchema, arguments); err != nil {
			return newToolErrorResult(err.Error()), nil
		}

		if operation.OperationID == "MemoService_CreateMemo" {
			ctx = api.WithMCPMemoCreation(ctx)
		}
		return execute(ctx, operation.Operation, arguments)
	}
}

// RegisterRoutes registers the streamable HTTP MCP endpoint.
func (s *MCPService) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxMCPRequestBytes {
			httphandler.WriteError(w, r, &httphandler.Error{Code: http.StatusRequestEntityTooLarge, Message: http.StatusText(http.StatusRequestEntityTooLarge)})
			return
		}
		if !isAllowedMCPOrigin(r, s.profile) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxMCPRequestBytes)
		s.servePersonal(w, r)
	})
}

func (s *MCPService) servePersonal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	remoteIP := clientip.FromContext(ctx)
	if s.RateLimiter != nil {
		if decision := s.RateLimiter.Consume(ratelimit.ScopeMCPIP, remoteIP, 1); !decision.Allowed {
			seconds := max(1, int((decision.RetryAfter+time.Second-1)/time.Second))
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			httphandler.WriteError(w, r, &httphandler.Error{Code: http.StatusTooManyRequests, Message: "MCP request rate limit exceeded"})
			return
		}
	}
	token := auth.ExtractBearerToken(r.Header.Get("Authorization"))
	connection, err := mcpconnection.Authenticate(ctx, s.store, token, remoteIP, r.Method == http.MethodPost, time.Now())
	if err != nil {
		var response *httphandler.Error
		switch {
		case errors.Is(err, mcpconnection.ErrInvalidToken):
			w.Header().Set("WWW-Authenticate", "Bearer")
			response = &httphandler.Error{Code: http.StatusUnauthorized, Message: err.Error()}
		case errors.Is(err, mcpconnection.ErrInactive):
			response = &httphandler.Error{Code: http.StatusForbidden, Message: err.Error()}
		default:
			response = &httphandler.Error{Code: http.StatusServiceUnavailable, Message: "MCP connection check unavailable", Cause: err}
		}
		httphandler.WriteError(w, r, response)
		return
	}
	ctx = auth.SetUserInContext(ctx, &store.User{ID: connection.UserID}, "")
	ctx = context.WithValue(ctx, connectionPermissionContextKey{}, connection.Permission)
	r = r.Clone(ctx)
	r.Header.Del("Authorization")
	s.handler.ServeHTTP(w, r)
}
