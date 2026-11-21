package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/core/mcpconnection"
	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/store"
)

type mcpUserContextKey struct{}

type mcpAPIRequest struct {
	user      *store.User
	procedure string
}

// WithMCPRequest grants the in-process request the connection owner's identity for one API procedure.
func WithMCPRequest(ctx context.Context, user *store.User, procedure string) context.Context {
	return context.WithValue(ctx, mcpUserContextKey{}, mcpAPIRequest{user: user, procedure: procedure})
}

func (s *APIService) newMCPConnectionHandler(authorizer *Authorizer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/mcpConnections", authorizeMCPConnectionManagement(authorizer, s.listMCPConnections))
	mux.HandleFunc("POST /api/mcpConnections", authorizeMCPConnectionManagement(authorizer, s.createMCPConnection))
	mux.HandleFunc("DELETE /api/mcpConnections/{id}", authorizeMCPConnectionManagement(authorizer, s.deleteMCPConnection))
	mux.HandleFunc("POST /api/mcpConnections/{id}/decision", authorizeMCPConnectionManagement(authorizer, s.revokeMCPConnection))
	return mux
}

func authorizeMCPConnectionManagement(authorizer *Authorizer, handler func(http.ResponseWriter, *http.Request, int32) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setAPIResponseNoStoreHeaders(w.Header())
		ctx := r.Context()
		result := authorizer.Authenticate(ctx, r.Header.Get("Authorization"))
		if result == nil {
			writeGatewayAuthorizationError(w, ErrUnauthenticated)
			return
		}
		// A web sign-in is required to grant or manage access. A general API
		// token cannot authorize itself or mint a connection.
		if result.Claims == nil {
			writeGatewayStatusError(w, status.Error(codes.PermissionDenied, "sign in to manage MCP connections"))
			return
		}
		if err := authorizer.Throttle(ctx, "", result); err != nil {
			writeGatewayStatusError(w, err)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		if err := handler(w, r, result.Claims.UserID); err != nil {
			writeGatewayStatusError(w, err)
		}
	}
}

func (s *APIService) listMCPConnections(w http.ResponseWriter, r *http.Request, userID int32) error {
	connections, err := s.Store.ListMCPConnections(r.Context(), userID)
	if err != nil {
		return status.Error(codes.Internal, "failed to list MCP connections")
	}
	now := time.Now()
	result := make([]store.MCPConnection, 0, len(connections))
	for _, connection := range connections {
		snapshot := *connection
		snapshot.Status = mcpconnection.StatusAt(connection, now)
		result = append(result, snapshot)
	}
	return httphandler.JSON(w, http.StatusOK, map[string]any{"connections": result})
}

func (s *APIService) createMCPConnection(w http.ResponseWriter, r *http.Request, userID int32) error {
	var request struct {
		Label    string `json:"label"`
		Duration string `json:"duration"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF {
		return status.Error(codes.InvalidArgument, "expected a connection name and optional duration")
	}
	connection, token, err := mcpconnection.Issue(r.Context(), s.Store, userID, request.Label, request.Duration, time.Now())
	if errors.Is(err, mcpconnection.ErrInvalidLabel) || errors.Is(err, mcpconnection.ErrInvalidDuration) {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	if err != nil {
		return status.Error(codes.Internal, "failed to create MCP connection")
	}
	return httphandler.JSON(w, http.StatusCreated, map[string]any{"connection": connection, "token": token})
}

func (s *APIService) deleteMCPConnection(w http.ResponseWriter, r *http.Request, userID int32) error {
	deleted, err := s.Store.DeleteMCPConnection(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		return status.Error(codes.Internal, "failed to delete MCP connection")
	}
	if !deleted {
		return status.Error(codes.NotFound, "MCP connection not found")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (s *APIService) revokeMCPConnection(w http.ResponseWriter, r *http.Request, userID int32) error {
	var request struct {
		Action string `json:"action"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF || request.Action != "revoke" {
		return status.Error(codes.InvalidArgument, "expected the revoke action")
	}
	err := mcpconnection.Revoke(r.Context(), s.Store, userID, r.PathValue("id"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
		return nil
	case errors.Is(err, mcpconnection.ErrRevocationConflict):
		return status.Error(codes.Aborted, err.Error())
	default:
		return status.Error(codes.Internal, "failed to revoke MCP connection")
	}
}
