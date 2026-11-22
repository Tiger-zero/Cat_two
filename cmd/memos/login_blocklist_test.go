package main

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func executeLoginBlocklistCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var output bytes.Buffer
	cmd := newLoginBlocklistCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(t.Context())
	return output.String(), err
}

func TestLoginBlocklistRemoveReadsDockerDSNFileAndClearsOnlyTarget(t *testing.T) {
	instance, data := newMigratedStore(t)
	ctx := t.Context()
	const target, other = "192.0.2.10", "2001:db8::11"
	for _, ip := range []string{target, other} {
		_, blocked, err := instance.RecordLoginIPFailure(ctx, ip, 1)
		require.NoError(t, err)
		require.True(t, blocked)
	}
	secretFile := filepath.Join(t.TempDir(), "dsn")
	dsn := (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(data, "memos_prod.db"))}).String()
	require.NoError(t, os.WriteFile(secretFile, []byte(dsn+"\n"), 0o600))
	t.Setenv("MEMOS_DATA", t.TempDir())
	t.Setenv("MEMOS_DRIVER", "sqlite")
	t.Setenv("MEMOS_DEMO", "false")
	t.Setenv("MEMOS_DSN", "")
	t.Setenv("MEMOS_DSN_FILE", secretFile)

	output, err := executeLoginBlocklistCommand(t, "remove", "--ip", "::ffff:"+target)
	require.NoError(t, err)
	require.Equal(t, "Removed "+target+" from the sign-in blocklist.\n", output)
	blocked, err := instance.IsLoginIPBlocked(ctx, target)
	require.NoError(t, err)
	require.False(t, blocked)
	blocked, err = instance.IsLoginIPBlocked(ctx, other)
	require.NoError(t, err)
	require.True(t, blocked)

	output, err = executeLoginBlocklistCommand(t, "remove", "--ip", target)
	require.NoError(t, err)
	require.Equal(t, "No sign-in block found for "+target+".\n", output)
}

func TestLoginBlocklistRemoveRejectsIPRangesAndScopedAddresses(t *testing.T) {
	for _, ip := range []string{"192.0.2.0/24", "fe80::42%eth0", "host.example.com", "192.0.2.10:5230"} {
		output, err := executeLoginBlocklistCommand(t, "remove", "--ip", ip)
		require.ErrorContains(t, err, "provide one IPv4 or IPv6 address with --ip")
		require.Empty(t, output)
	}
}

func TestLoginBlocklistRemoveRefusesMissingDatabase(t *testing.T) {
	data := t.TempDir()
	t.Setenv("MEMOS_DATA", data)
	t.Setenv("MEMOS_DRIVER", "sqlite")
	t.Setenv("MEMOS_DSN", "")
	t.Setenv("MEMOS_DSN_FILE", "")
	t.Setenv("MEMOS_DEMO", "false")

	output, err := executeLoginBlocklistCommand(t, "remove", "--ip", "192.0.2.10")
	require.ErrorContains(t, err, "cannot open the instance database")
	require.Empty(t, output)
	require.NoFileExists(t, filepath.Join(data, "memos_prod.db"))
}
