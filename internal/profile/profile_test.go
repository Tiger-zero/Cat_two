package profile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateInstanceURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{name: "empty", url: ""},
		{name: "canonical https", url: "https://memos.example.com", want: "https://memos.example.com"},
		{name: "trim spaces and trailing slash", url: "  https://memos.example.com/app/  ", want: "https://memos.example.com/app"},
		{name: "http with port", url: "http://localhost:8080/", want: "http://localhost:8080"},
		{name: "missing scheme", url: "memos.example.com", wantErr: true},
		{name: "unsupported scheme", url: "ftp://memos.example.com", wantErr: true},
		{name: "missing host", url: "https:///memos", wantErr: true},
		{name: "credentials", url: "https://user:pass@memos.example.com", wantErr: true},
		{name: "query", url: "https://memos.example.com?tenant=one", wantErr: true},
		{name: "fragment", url: "https://memos.example.com/#top", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &Profile{Data: t.TempDir(), Driver: "sqlite", InstanceURL: c.url}
			err := p.Validate()
			if c.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, p.InstanceURL)
		})
	}
}

func TestValidateDatabaseDriver(t *testing.T) {
	for _, driver := range []string{"", SQLiteDriver} {
		p := &Profile{Driver: driver, Data: t.TempDir()}
		require.NoError(t, p.Validate())
		require.Equal(t, SQLiteDriver, p.Driver)
		require.Equal(t, filepath.Join(p.Data, "memos_prod.db"), p.DSN)
	}
	for _, demo := range []bool{false, true} {
		p := &Profile{Demo: demo, Driver: "postgres", Data: filepath.Join(t.TempDir(), "data")}
		require.ErrorContains(t, p.Validate(), "Cat_two requires SQLite")
		require.NoDirExists(t, p.Data, "invalid database configurations must fail before preparing their data directory")
	}
}

func TestDefaultDataDirPreservesExistingMount(t *testing.T) {
	containerDir := t.TempDir()
	sentinelPath := filepath.Join(containerDir, ".write-test")
	contents := []byte("preexisting hidden file")
	require.NoError(t, os.WriteFile(sentinelPath, contents, 0o600))
	require.Equal(t, containerDir, defaultDataDir(containerDir))
	require.NoError(t, os.Chmod(containerDir, 0o550))
	t.Cleanup(func() { require.NoError(t, os.Chmod(containerDir, 0o750)) })

	resolved := &Profile{Data: defaultDataDir(containerDir)}
	require.NoError(t, resolved.Validate())
	require.Equal(t, containerDir, resolved.Data)
	require.Equal(t, filepath.Join(containerDir, "memos_prod.db"), resolved.DSN)
	got, err := os.ReadFile(sentinelPath)
	require.NoError(t, err)
	require.Equal(t, contents, got)

	require.Equal(t, ".", defaultDataDir(filepath.Join(containerDir, "absent")))
	require.Equal(t, ".", defaultDataDir(sentinelPath))
}
