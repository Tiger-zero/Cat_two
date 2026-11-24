package store

import (
	"context"
	"database/sql"
)

// Driver is an interface for store driver.
// It contains all methods that store database driver should implement.
type Driver interface {
	GetDB() *sql.DB
	Close() error

	IsInitialized(ctx context.Context) (bool, error)

	// GetDatabaseSize returns the database size in bytes, or -1 if unavailable.
	// A non-nil error indicates a hard failure; -1 with nil error means the
	// driver cannot report a size from the underlying database.
	GetDatabaseSize(ctx context.Context) (int64, error)

	// Attachment model related methods.
	CreateAttachment(ctx context.Context, create *Attachment) (*Attachment, error)
	ListAttachments(ctx context.Context, find *FindAttachment) ([]*Attachment, error)
	GetAttachmentStorageUsage(ctx context.Context, creatorID int32) (int64, error)
	UpdateAttachment(ctx context.Context, update *UpdateAttachment) error
	ListLegacyAttachmentIDs(ctx context.Context, afterID int32, limit int) (*sql.Rows, error)
	LocalizeAttachment(ctx context.Context, original *Attachment, reference, payload string) (bool, error)
	DeleteAttachment(ctx context.Context, delete *DeleteAttachment) error
	DeleteAttachments(ctx context.Context, deletes []*DeleteAttachment) error
	DeleteAttachmentsWithPolicy(ctx context.Context, policy *AttachmentDeletionPolicy, attachmentIDs []int32) error
	ApplyMemoMutation(ctx context.Context, mutation *MemoMutation) error
	ListMemoTrashAttachmentCleanup(ctx context.Context, afterID int32, nowTs int64, limit int) ([]*Attachment, error)
	ClaimMemoTrashAttachmentCleanup(ctx context.Context, attachmentID int32, nowTs, retryTs int64) (bool, error)
	CompleteMemoTrashAttachmentCleanup(ctx context.Context, attachmentID int32, retryTs int64) error
	ListLocalAttachmentStorageMatches(ctx context.Context, basename, suffixPattern string, replacingID int32, includePending bool) (*sql.Rows, error)

	// Memo model related methods.
	CreateMemo(ctx context.Context, create *Memo) (*Memo, error)
	ListMemos(ctx context.Context, find *FindMemo) ([]*Memo, error)
	// QueryActiveMemoTags returns one JSON tags column per active top-level memo of the active owner.
	QueryActiveMemoTags(ctx context.Context, creatorID int32) (*sql.Rows, error)
	UpdateMemo(ctx context.Context, update *UpdateMemo) error
	RefreshMemoDerivedPayload(ctx context.Context, refresh *RefreshMemoDerivedPayload) error
	DeleteMemo(ctx context.Context, delete *DeleteMemo) error
	DeleteMemoWithPolicy(ctx context.Context, delete *DeleteMemoWithPolicy) (*DeleteMemoWithPolicyResult, error)
	ApplyMemoTrash(ctx context.Context, mutation *MemoTrashMutation) ([]*Attachment, error)
	// CountUserMemos counts the memos a user created, comments included, in every row status and Space.
	CountUserMemos(ctx context.Context, creatorID int32) (int64, error)

	// MemoRelation model related methods.
	UpsertMemoRelation(ctx context.Context, create *MemoRelation) (*MemoRelation, error)
	ListMemoRelations(ctx context.Context, find *FindMemoRelation) ([]*MemoRelation, error)
	DeleteMemoRelation(ctx context.Context, delete *DeleteMemoRelation) error

	// InstanceSetting model related methods.
	// CreateInstanceSettingIfNotExists atomically creates the setting when its name is absent and reports whether it inserted the row.
	CreateInstanceSettingIfNotExists(ctx context.Context, create *InstanceSetting) (bool, error)
	// UpdateInstanceSettingIfUnchanged replaces the value only when the stored value still equals oldValue, and reports whether it did.
	UpdateInstanceSettingIfUnchanged(ctx context.Context, name, oldValue, newValue string) (bool, error)
	// GetInstanceSettingTx reads one setting within tx, locking its row where the database supports row locks, or returns nil when it is absent.
	GetInstanceSettingTx(ctx context.Context, tx *sql.Tx, name string) (*InstanceSetting, error)
	// CreateInstanceSettingIfNotExistsTx is CreateInstanceSettingIfNotExists within tx.
	CreateInstanceSettingIfNotExistsTx(ctx context.Context, tx *sql.Tx, create *InstanceSetting) (bool, error)
	// UpdateInstanceSettingIfUnchangedTx is UpdateInstanceSettingIfUnchanged within tx.
	UpdateInstanceSettingIfUnchangedTx(ctx context.Context, tx *sql.Tx, name, oldValue, newValue string) (bool, error)
	UpsertInstanceSetting(ctx context.Context, upsert *InstanceSetting) (*InstanceSetting, error)
	// UpsertInstanceSettingTx writes the setting within tx, for callers that must commit it together with other statements.
	UpsertInstanceSettingTx(ctx context.Context, tx *sql.Tx, upsert *InstanceSetting) error
	ListInstanceSettings(ctx context.Context, find *FindInstanceSetting) ([]*InstanceSetting, error)
	DeleteInstanceSetting(ctx context.Context, delete *DeleteInstanceSetting) error
	DeleteInstanceSettingTx(ctx context.Context, tx *sql.Tx, delete *DeleteInstanceSetting) error

	// User model related methods.
	CreateUser(ctx context.Context, create *User) (*User, error)
	UpdateUser(ctx context.Context, update *UpdateUser) (*User, error)
	ListUsers(ctx context.Context, find *FindUser) ([]*User, error)
	// CreateUserTx, UpdateUserTx, and ListUsersTx are the same methods within tx.
	CreateUserTx(ctx context.Context, tx *sql.Tx, create *User) (*User, error)
	UpdateUserTx(ctx context.Context, tx *sql.Tx, update *UpdateUser) (*User, error)
	ListUsersTx(ctx context.Context, tx *sql.Tx, find *FindUser) ([]*User, error)
	DeleteUser(ctx context.Context, delete *DeleteUser) (*DeleteUserResult, error)
	// GetUserTwoFactor reads the persisted factor state, locking it in a transaction.
	GetUserTwoFactor(ctx context.Context, tx *sql.Tx, userID int32) (UserTwoFactor, error)
	UpsertUserTwoFactorTx(ctx context.Context, tx *sql.Tx, userID int32, state UserTwoFactor) error

	// UserSetting model related methods.
	UpsertUserSetting(ctx context.Context, upsert *UserSetting) (*UserSetting, error)
	// CompareAndSetUserSetting writes a setting only if its value has not changed.
	// A nil previous value matches an absent row. A nil transaction uses the database directly.
	CompareAndSetUserSetting(ctx context.Context, tx *sql.Tx, setting *UserSetting, previous *string) (bool, error)
	// ListUserSettings uses the database directly if tx is nil.
	ListUserSettings(ctx context.Context, tx *sql.Tx, find *FindUserSetting) ([]*UserSetting, error)
	DeleteUserSettings(ctx context.Context, delete *DeleteUserSetting) error
	// GetUserByPATHash returns the token matching tokenHash with its owner ID, or
	// (nil, nil) when no stored token matches. An error means the lookup itself
	// failed, including a stored row that cannot be decoded.
	GetUserByPATHash(ctx context.Context, tokenHash string) (*PATQueryResult, error)

	// Persistent login attempts and manual IP blocks.
	IsLoginIPBlocked(ctx context.Context, ip string) (bool, error)
	RecordLoginIPFailure(ctx context.Context, ip string, limit int, nowTs int64) (count int, blocked bool, err error)
	ClearLoginIPFailures(ctx context.Context, ip string) (blocked bool, err error)
	ListBlockedLoginIPs(ctx context.Context, afterIP string, limit int) ([]*BlockedLoginIP, error)
	UnblockLoginIP(ctx context.Context, ip string) (bool, error)

	// MCP connection requests and authorization states.
	CreateMCPConnection(ctx context.Context, connection *MCPConnection) error
	ListMCPConnections(ctx context.Context, userID int32) ([]*MCPConnection, error)
	GetMCPConnectionByHash(ctx context.Context, tokenHash string) (*MCPConnection, RowStatus, error)
	RecordMCPConnectionFirstUse(ctx context.Context, tokenHash string, requestedAt int64, remoteIP string) error
	UpdateMCPConnection(ctx context.Context, userID int32, id string, from MCPConnectionStatus, to MCPConnectionStatus, expiresAt int64) (bool, error)
	DeleteMCPConnection(ctx context.Context, userID int32, id string) (bool, error)

	// IsRetryableTransactionError reports whether err is a serialization or locking conflict that running the transaction again can resolve.
	IsRetryableTransactionError(err error) bool

	// Inbox model related methods.
	CreateInbox(ctx context.Context, create *Inbox) (*Inbox, error)
	ListInboxes(ctx context.Context, find *FindInbox) ([]*Inbox, error)
	UpdateInbox(ctx context.Context, update *UpdateInbox) (*Inbox, error)
	DeleteInbox(ctx context.Context, delete *DeleteInbox) error

	// Reaction model related methods.
	UpsertReaction(ctx context.Context, create *Reaction) (*Reaction, error)
	ListReactions(ctx context.Context, find *FindReaction) ([]*Reaction, error)
	GetReaction(ctx context.Context, find *FindReaction) (*Reaction, error)
	DeleteReaction(ctx context.Context, delete *DeleteReaction) error

	// MemoShare model related methods.
	CreateMemoShare(ctx context.Context, create *MemoShare) (*MemoShare, error)
	ListMemoShares(ctx context.Context, find *FindMemoShare) ([]*MemoShare, error)
	GetMemoShare(ctx context.Context, find *FindMemoShare) (*MemoShare, error)
	DeleteMemoShare(ctx context.Context, delete *DeleteMemoShare) error
}
