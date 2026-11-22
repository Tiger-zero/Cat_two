package api

import (
	"encoding/base64"
	"net/http"
	"net/netip"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/usememos/memos/internal/httphandler"
	"github.com/usememos/memos/server/auth"
	"github.com/usememos/memos/store"
)

const (
	loginBlocklistPath     = "/api/auth/loginBlocklist"
	loginBlocklistPageSize = 100
	loginIPFailureLimit    = 6
)

func canonicalLoginIP(value string) string {
	ip, err := netip.ParseAddr(value)
	if err != nil {
		return ""
	}
	return ip.WithZone("").Unmap().String()
}

func blockedLoginIPError() error {
	st := status.New(codes.PermissionDenied, "this IP is blocked from signing in")
	detailed, err := st.WithDetails(&errdetails.ErrorInfo{Reason: "LOGIN_IP_BLOCKED", Domain: errorInfoDomain})
	if err != nil {
		return st.Err()
	}
	return detailed.Err()
}

type loginBlockedEntry struct {
	IP        string    `json:"ip"`
	BlockedAt time.Time `json:"blockedAt"`
}

type loginBlocklistPage struct {
	Entries       []loginBlockedEntry `json:"entries"`
	NextPageToken string              `json:"nextPageToken,omitempty"`
}

func (s *APIService) newLoginBlocklistHandler(authorizer *Authorizer) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+loginBlocklistPath, s.authorizeLoginBlocklist(authorizer, s.listLoginBlocklist))
	mux.HandleFunc("DELETE "+loginBlocklistPath+"/{ip}", s.authorizeLoginBlocklist(authorizer, s.removeLoginBlock))
	return mux
}

func (s *APIService) authorizeLoginBlocklist(authorizer *Authorizer, handler func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
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
		user, err := s.fetchCurrentUser(auth.ApplyToContext(ctx, result))
		if err != nil {
			writeGatewayStatusError(w, status.Error(codes.Internal, "failed to check the administrator session"))
			return
		}
		if user == nil || user.Role != store.RoleAdmin {
			writeGatewayStatusError(w, status.Error(codes.PermissionDenied, "a web administrator session is required"))
			return
		}
		if err := handler(w, r); err != nil {
			writeGatewayStatusError(w, err)
		}
	}
}

func (s *APIService) listLoginBlocklist(w http.ResponseWriter, r *http.Request) error {
	afterIP := ""
	if token := r.URL.Query().Get("pageToken"); token != "" {
		if len(token) > base64.RawURLEncoding.EncodedLen(len("ffff:ffff:ffff:ffff:ffff:ffff:255.255.255.255")) {
			return status.Error(codes.InvalidArgument, "invalid page token")
		}
		value, err := base64.RawURLEncoding.Strict().DecodeString(token)
		afterIP = canonicalLoginIP(string(value))
		if err != nil || afterIP == "" {
			return status.Error(codes.InvalidArgument, "invalid page token")
		}
	}
	rows, err := s.Store.ListBlockedLoginIPs(r.Context(), afterIP, loginBlocklistPageSize+1)
	if err != nil {
		return status.Error(codes.Internal, "failed to list blocked login IPs")
	}
	page := loginBlocklistPage{Entries: make([]loginBlockedEntry, 0, min(len(rows), loginBlocklistPageSize))}
	if len(rows) > loginBlocklistPageSize {
		rows = rows[:loginBlocklistPageSize]
		page.NextPageToken = base64.RawURLEncoding.EncodeToString([]byte(rows[len(rows)-1].IP))
	}
	for _, row := range rows {
		page.Entries = append(page.Entries, loginBlockedEntry{IP: row.IP, BlockedAt: row.BlockedAt})
	}
	return httphandler.JSON(w, http.StatusOK, page)
}

func (s *APIService) removeLoginBlock(w http.ResponseWriter, r *http.Request) error {
	ip := canonicalLoginIP(r.PathValue("ip"))
	if ip == "" {
		return status.Error(codes.InvalidArgument, "invalid IP address")
	}
	if _, err := s.Store.UnblockLoginIP(r.Context(), ip); err != nil {
		return status.Error(codes.Internal, "failed to unblock login IP")
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
