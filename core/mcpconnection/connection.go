package mcpconnection

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkg/errors"

	"github.com/usememos/memos/internal/random"
	"github.com/usememos/memos/store"
)

const tokenPrefix = "cat_two_mcp_"

var (
	// ErrInvalidLabel indicates that a connection label is empty or too long.
	ErrInvalidLabel = errors.New("enter a connection name of 1 to 80 characters")
	// ErrInvalidDecision indicates that an approval or revocation action is unknown.
	ErrInvalidDecision = errors.New("invalid connection decision")
	// ErrDecisionConflict indicates that the connection is no longer in a state that accepts the action.
	ErrDecisionConflict = errors.New("connection cannot accept this decision")
	// ErrInvalidToken indicates that the credential or its account is unavailable.
	ErrInvalidToken = errors.New("a Cat_two MCP connection token is required")
	// ErrPending indicates that the owner must approve the connection.
	ErrPending = errors.New("approve this connection in Cat_two Settings → MCP and API, then reconnect")
	// ErrInactive indicates that an approval was denied or is no longer valid.
	ErrInactive = errors.New("this Cat_two MCP connection is denied, revoked, or expired")
)

// Issue creates an MCP-only connection token and returns its plaintext once.
func Issue(ctx context.Context, db *store.Store, userID int32, label string, now time.Time) (*store.MCPConnection, string, error) {
	label = strings.TrimSpace(label)
	if label == "" || utf8.RuneCountInString(label) > 80 {
		return nil, "", ErrInvalidLabel
	}
	token := tokenPrefix + rand.Text()
	connection := &store.MCPConnection{
		ID:        random.UUID(),
		UserID:    userID,
		Label:     label,
		TokenHash: hashToken(token),
		Status:    store.MCPConnectionAwaiting,
		CreatedAt: now.Unix(),
	}
	if err := db.CreateMCPConnection(ctx, connection); err != nil {
		return nil, "", err
	}
	return connection, token, nil
}

// StatusAt reports expiration without changing the stored approval.
func StatusAt(connection *store.MCPConnection, now time.Time) store.MCPConnectionStatus {
	if connection.Status == store.MCPConnectionApproved && connection.ExpiresAt != 0 && connection.ExpiresAt <= now.Unix() {
		return "expired"
	}
	return connection.Status
}

// Decide applies the owner's approval, denial, or revocation to the current connection state.
func Decide(ctx context.Context, db *store.Store, userID int32, id, action string, now time.Time) error {
	from := []store.MCPConnectionStatus{store.MCPConnectionPending}
	to := store.MCPConnectionApproved
	var expiresAt int64
	switch action {
	case "approve_day":
		expiresAt = now.Add(24 * time.Hour).Unix()
	case "approve_permanent":
	case "deny":
		to = store.MCPConnectionDenied
	case "revoke":
		from = []store.MCPConnectionStatus{store.MCPConnectionAwaiting, store.MCPConnectionApproved}
		to = store.MCPConnectionRevoked
	default:
		return ErrInvalidDecision
	}
	for _, state := range from {
		updated, err := db.UpdateMCPConnection(ctx, userID, id, state, to, expiresAt)
		if err != nil || updated {
			return err
		}
	}
	return ErrDecisionConflict
}

// Authenticate records the first connection attempt and resolves a live approval for any subsequent request.
func Authenticate(ctx context.Context, db *store.Store, token, remoteIP string, recordAttempt bool, now time.Time) (*store.User, error) {
	if !strings.HasPrefix(token, tokenPrefix) {
		return nil, ErrInvalidToken
	}
	digest := hashToken(token)
	connection, err := db.GetMCPConnectionByHash(ctx, digest)
	if err != nil {
		return nil, err
	}
	if connection == nil {
		return nil, ErrInvalidToken
	}
	switch StatusAt(connection, now) {
	case store.MCPConnectionAwaiting:
		if recordAttempt {
			if err := db.MarkMCPConnectionRequested(ctx, digest, now.Unix(), remoteIP); err != nil {
				return nil, err
			}
		}
		return nil, ErrPending
	case store.MCPConnectionPending:
		return nil, ErrPending
	case store.MCPConnectionApproved:
		user, err := db.GetUser(ctx, &store.FindUser{ID: &connection.UserID})
		if err != nil {
			return nil, err
		}
		if user == nil || user.RowStatus != store.Normal {
			return nil, ErrInvalidToken
		}
		return user, nil
	default:
		return nil, ErrInactive
	}
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
