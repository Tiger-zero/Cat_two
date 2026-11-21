package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"github.com/usememos/memos/core/mcpconnection"
	"github.com/usememos/memos/internal/clientip"
	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/internal/profile"
	memosproto "github.com/usememos/memos/proto"
	"github.com/usememos/memos/server/api"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

// maxMCPRequestBytes caps the /mcp request body. It tracks the API limit because
// every tool call is forwarded in-process through the API routes. The SDK
// enforces its own 4 MiB default, so the value is passed to the transport as
// well; otherwise attachment uploads over MCP fail with 413 before the API sees them.
const maxMCPRequestBytes int64 = api.MaxAPIRequestBytes

// toolCatalogTTL is the freshness hint advertised on tools/list and
// server/discover results. The catalog is fixed at startup, so clients may cache
// it for a long time instead of re-listing on every session.
const toolCatalogTTL = 24 * time.Hour

// MCPService serves the OpenAPI-driven MCP endpoint.
type MCPService struct {
	profile *profile.Profile
	store   *store.Store

	operationsByTool map[string]*registeredOperation
	handler          http.Handler
}

type connectionUserIDContextKey struct{}

// NewMCPService creates an MCP service backed by the in-process API routes.
func NewMCPService(profile *profile.Profile, handler http.Handler, db *store.Store) (*MCPService, error) {
	personal := profile != nil && profile.Personal
	if personal && db == nil {
		return nil, errors.New("MCP connection authentication requires a store")
	}
	spec, err := loadMCPServiceOpenAPISpec()
	if err != nil {
		return nil, err
	}
	registry, err := buildOperationRegistry(spec)
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

	adapter := newAPIAdapter(handler)
	for _, tool := range tools {
		operation := operationsByTool[tool.Name]
		if personal && !isPersonalMCPOperation(operation.OperationID) {
			delete(operationsByTool, tool.Name)
			continue
		}
		server.AddTool(tool, newMCPToolHandler(adapter, operation))
	}

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
	return &MCPService{
		profile:          profile,
		store:            db,
		operationsByTool: operationsByTool,
		handler:          streamableHandler,
	}, nil
}

func isPersonalMCPOperation(id string) bool {
	switch id {
	case "AuthService_GetCurrentUser", "UserService_ListMemoViews",
		"MemoService_ListMemos", "MemoService_GetMemo", "MemoService_CreateMemo", "MemoService_UpdateMemo":
		return true
	default:
		return false
	}
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

func newMCPToolHandler(adapter *apiAdapter, operation *registeredOperation) sdkmcp.ToolHandler {
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

		authorization := ""
		if request.Extra != nil {
			authorization = request.Extra.Header.Get("Authorization")
		}
		if userID, ok := ctx.Value(connectionUserIDContextKey{}).(int32); ok {
			procedure := "/memos.api." + strings.Replace(operation.OperationID, "_", "/", 1)
			ctx = api.WithMCPRequest(ctx, userID, procedure)
		}
		return adapter.execute(ctx, operation.Operation, arguments, authorization)
	}
}

// RegisterRoutes registers the streamable HTTP MCP endpoint.
func (s *MCPService) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maxMCPRequestBytes {
			httphandler.WriteError(w, r, &httphandler.Error{Code: http.StatusRequestEntityTooLarge, Message: http.StatusText(http.StatusRequestEntityTooLarge)})
			return
		}
		if !isAllowedMCPOrigin(r.Host, r.Header.Get("Origin"), s.profile) {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxMCPRequestBytes)
		if s.profile != nil && s.profile.Personal {
			s.servePersonal(w, r)
			return
		}
		s.handler.ServeHTTP(w, r)
	})
}

func (s *MCPService) servePersonal(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	ctx := r.Context()
	token := auth.ExtractBearerToken(r.Header.Get("Authorization"))
	userID, err := mcpconnection.Authenticate(ctx, s.store, token, clientip.FromContext(ctx), r.Method == http.MethodPost, time.Now())
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
	r = r.Clone(context.WithValue(ctx, connectionUserIDContextKey{}, userID))
	r.Header.Del("Authorization")
	s.handler.ServeHTTP(w, r)
}
