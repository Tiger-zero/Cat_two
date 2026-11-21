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
	// ErrInvalidDuration indicates that a token lifetime is unknown.
	ErrInvalidDuration = errors.New("connection duration must be day or permanent")
	// ErrRevocationConflict indicates that no revocable connection matches the request.
	ErrRevocationConflict = errors.New("connection cannot be revoked")
	// ErrInvalidToken indicates that the credential or its account is unavailable.
	ErrInvalidToken = errors.New("a Cat_two MCP connection token is required")
	// ErrInactive indicates that the connection is inactive or has expired.
	ErrInactive = errors.New("this Cat_two MCP connection is inactive or expired; create a new connection token")
)

// Issue creates an active MCP-only connection token and returns its plaintext once.
func Issue(ctx context.Context, db *store.Store, userID int32, label, duration string, now time.Time) (*store.MCPConnection, string, error) {
	label = strings.TrimSpace(label)
	if label == "" || utf8.RuneCountInString(label) > 80 {
		return nil, "", ErrInvalidLabel
	}
	var expiresAt int64
	switch duration {
	case "", "day":
		expiresAt = now.Add(24 * time.Hour).Unix()
	case "permanent":
		// A zero expiration represents a permanent connection.
	default:
		return nil, "", ErrInvalidDuration
	}
	token := tokenPrefix + rand.Text()
	connection := &store.MCPConnection{
		ID:        random.UUID(),
		UserID:    userID,
		Label:     label,
		TokenHash: hashToken(token),
		Status:    store.MCPConnectionApproved,
		CreatedAt: now.Unix(),
		ExpiresAt: expiresAt,
	}
	if err := db.CreateMCPConnection(ctx, connection); err != nil {
		return nil, "", err
	}
	return connection, token, nil
}

// StatusAt reports expiration without changing the stored connection.
func StatusAt(connection *store.MCPConnection, now time.Time) store.MCPConnectionStatus {
	if connection.Status == store.MCPConnectionApproved && connection.ExpiresAt != 0 && connection.ExpiresAt <= now.Unix() {
		return "expired"
	}
	return connection.Status
}

// Revoke disables the owner's active token or retires an old, unapproved connection.
func Revoke(ctx context.Context, db *store.Store, userID int32, id string) error {
	for _, state := range []store.MCPConnectionStatus{store.MCPConnectionApproved, store.MCPConnectionAwaiting, store.MCPConnectionPending} {
		updated, err := db.UpdateMCPConnection(ctx, userID, id, state, store.MCPConnectionRevoked, 0)
		if err != nil || updated {
			return err
		}
	}
	return ErrRevocationConflict
}

// Authenticate resolves an active connection and records its first authenticated POST.
func Authenticate(ctx context.Context, db *store.Store, token, remoteIP string, recordUse bool, now time.Time) (*store.User, error) {
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
	// Earlier versions may have stored credentials that never received the
	// owner's approval. Only tokens already active can authenticate.
	if StatusAt(connection, now) != store.MCPConnectionApproved {
		return nil, ErrInactive
	}
	user, err := db.GetUser(ctx, &store.FindUser{ID: &connection.UserID})
	if err != nil {
		return nil, err
	}
	if user == nil || user.RowStatus != store.Normal {
		return nil, ErrInvalidToken
	}
	if recordUse && connection.RequestedAt == 0 {
		if err := db.RecordMCPConnectionFirstUse(ctx, digest, now.Unix(), remoteIP); err != nil {
			return nil, err
		}
	}
	return user, nil
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
