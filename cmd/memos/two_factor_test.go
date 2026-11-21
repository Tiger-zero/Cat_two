package main

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/store"
)

func executeTwoFactorCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	cmd := newTwoFactorCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(t.Context())
	return output.String(), err
}

func TestTwoFactorResetReadsDockerDSNFileAndOnlyResetsTarget(t *testing.T) {
	instance, data := newMigratedStore(t)
	ctx := t.Context()
	target, err := instance.CreateUser(ctx, &store.User{Username: "target", Role: store.RoleUser, PasswordHash: "target-password-hash"})
	require.NoError(t, err)
	other, err := instance.CreateUser(ctx, &store.User{Username: "other", Role: store.RoleAdmin, PasswordHash: "other-password-hash"})
	require.NoError(t, err)
	before := store.UserTwoFactor{
		ActiveSecret: "encrypted-active-secret", PendingSecret: "encrypted-pending-secret", PendingExpiresTs: 42,
		LastUsedStep: 15, SessionRevision: "prior-session", FailedAttempts: 2, AttemptWindowTs: 37,
	}
	for _, user := range []*store.User{target, other} {
		require.NoError(t, instance.UpdateUserTwoFactor(ctx, user.ID, &user.PasswordHash, func(state *store.UserTwoFactor) error {
			*state = before
			return nil
		}))
	}

	secretFile := filepath.Join(t.TempDir(), "dsn")
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(data, "memos_prod.db"))}).String()
	require.NoError(t, os.WriteFile(secretFile, []byte(dsn+"\n"), 0o600))
	t.Setenv("MEMOS_DATA", t.TempDir())
	t.Setenv("MEMOS_DRIVER", "sqlite")
	t.Setenv("MEMOS_DEMO", "false")
	t.Setenv("MEMOS_DSN", "")
	t.Setenv("MEMOS_DSN_FILE", secretFile)
	output, err := executeTwoFactorCommand(t, "reset", "--username", target.Username)
	require.NoError(t, err)
	require.Equal(t, "Two-factor authentication reset. Sign in again.\n", output)
	after, err := instance.GetUserTwoFactor(ctx, target.ID)
	require.NoError(t, err)
	require.NotEmpty(t, after.SessionRevision)
	require.NotEqual(t, before.SessionRevision, after.SessionRevision)
	require.Equal(t, store.UserTwoFactor{SessionRevision: after.SessionRevision}, after)
	otherFactor, err := instance.GetUserTwoFactor(ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, before, otherFactor)
	storedUser, err := instance.GetUser(ctx, &store.FindUser{Username: &target.Username})
	require.NoError(t, err)
	require.Equal(t, target.PasswordHash, storedUser.PasswordHash)
}

func TestTwoFactorResetRefusesMissingDatabase(t *testing.T) {
	data := t.TempDir()
	t.Setenv("MEMOS_DATA", data)
	t.Setenv("MEMOS_DRIVER", "sqlite")
	t.Setenv("MEMOS_DSN", "")
	t.Setenv("MEMOS_DSN_FILE", "")
	t.Setenv("MEMOS_DEMO", "false")
	output, err := executeTwoFactorCommand(t, "reset", "--username", "administrator")
	require.ErrorContains(t, err, "cannot open the instance database")
	require.Empty(t, output)
	require.NoFileExists(t, filepath.Join(data, "memos_prod.db"))
}

func TestTwoFactorResetRedactsDatabaseSettings(t *testing.T) {
	t.Setenv("MEMOS_DATA", t.TempDir())
	t.Setenv("MEMOS_DRIVER", "sqlite")
	t.Setenv("MEMOS_DEMO", "false")
	secretFile := filepath.Join(t.TempDir(), "dsn")
	const secretValue = "test-database-value-not-for-output"
	require.NoError(t, os.WriteFile(secretFile, []byte(secretValue+"\n"), 0o600))
	t.Setenv("MEMOS_DSN", "")
	t.Setenv("MEMOS_DSN_FILE", secretFile)
	output, err := executeTwoFactorCommand(t, "reset", "--username", "administrator")
	require.ErrorContains(t, err, "cannot open the instance database")
	require.NotContains(t, err.Error()+output, secretValue)
	require.NotContains(t, err.Error()+output, secretFile)
}
