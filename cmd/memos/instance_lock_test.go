package main

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/usememos/memos/internal/profile"
)

func TestSQLiteInstanceLockRejectsSharedDataOrDatabase(t *testing.T) {
	data, otherData := t.TempDir(), t.TempDir()
	original := &profile.Profile{Data: data, Driver: "sqlite", DSN: filepath.Join(data, "a #.db")}
	ctx := t.Context()
	release, err := lockInstance(ctx, original)
	require.NoError(t, err)
	defer release()

	for _, conflicting := range []*profile.Profile{
		original,
		{Data: data, Driver: "sqlite", DSN: filepath.Join(data, "b.db")},
		{Data: otherData, Driver: "sqlite", DSN: original.DSN},
		{Data: otherData, Driver: "sqlite", DSN: original.DSN + "?fixtureTag=example"},
		{Data: otherData, Driver: "sqlite", DSN: "file:" + (&url.URL{Path: filepath.ToSlash(original.DSN)}).EscapedPath()},
	} {
		_, err := lockInstance(ctx, conflicting)
		require.ErrorContains(t, err, "cannot lock Cat_two instance")
	}
	// A failed second acquisition must also release the other data directory.
	safe := &profile.Profile{Data: otherData, Driver: "sqlite", DSN: filepath.Join(otherData, "b.db")}
	unlockOther, err := lockInstance(ctx, safe)
	require.NoError(t, err)
	defer unlockOther()
	release()
	reopen, err := lockInstance(ctx, original)
	require.NoError(t, err)
	defer reopen()
}
