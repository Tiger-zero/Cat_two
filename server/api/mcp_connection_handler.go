package api

import (
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

func (s *APIService) registerMCPConnectionRoutes(mux *http.ServeMux, authorizer *Authorizer) {
	mux.HandleFunc("GET /internal/mcpConnections", authorizeMCPConnectionManagement(authorizer, s.listMCPConnections))
	mux.HandleFunc("POST /internal/mcpConnections", authorizeMCPConnectionManagement(authorizer, s.createMCPConnection))
	mux.HandleFunc("DELETE /internal/mcpConnections/{id}", authorizeMCPConnectionManagement(authorizer, s.deleteMCPConnection))
	mux.HandleFunc("POST /internal/mcpConnections/{id}/decision", authorizeMCPConnectionManagement(authorizer, s.revokeMCPConnection))
}

func authorizeMCPConnectionManagement(authorizer *Authorizer, handler func(http.ResponseWriter, *http.Request, int32) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setAPIResponseNoStoreHeaders(w.Header())
		ctx := r.Context()
		result := authorizer.Authenticate(ctx, r.Header.Get("Authorization"))
		if result == nil {
			writeHTTPAuthorizationError(w, ErrUnauthenticated)
			return
		}
		if err := authorizer.Throttle(ctx, "", result); err != nil {
			writeHTTPStatusError(w, err)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		if err := handler(w, r, result.Claims.UserID); err != nil {
			writeHTTPStatusError(w, err)
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
		Label      string                        `json:"label"`
		Duration   string                        `json:"duration"`
		Permission store.MCPConnectionPermission `json:"permission"`
	}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil || decoder.Decode(new(any)) != io.EOF {
		return status.Error(codes.InvalidArgument, "expected a connection name and optional duration and permission")
	}
	connection, token, err := mcpconnection.Issue(r.Context(), s.Store, userID, request.Label, request.Duration, request.Permission, time.Now())
	if errors.Is(err, mcpconnection.ErrInvalidLabel) || errors.Is(err, mcpconnection.ErrInvalidDuration) || errors.Is(err, mcpconnection.ErrInvalidPermission) {
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
