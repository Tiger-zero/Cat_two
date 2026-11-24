package scripts

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLegacyInstallerDirectsToCompose(t *testing.T) {
	installDir := filepath.Join(t.TempDir(), "legacy-install")
	for _, tc := range []struct {
		name     string
		args     []string
		exitCode int
	}{
		{name: "direct", exitCode: 1},
		{name: "old flags", args: []string{"--repo", "usememos/memos", "--install-dir", installDir}, exitCode: 1},
		{name: "help", args: []string{"--help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("sh", append([]string{"install.sh"}, tc.args...)...)
			cmd.Env = []string{"PATH=" + t.TempDir()}
			out, err := cmd.CombinedOutput()
			if tc.exitCode == 0 {
				require.NoError(t, err, string(out))
			} else {
				var status *exec.ExitError
				require.ErrorAs(t, err, &status, string(out))
				require.Equal(t, tc.exitCode, status.ExitCode())
			}
			require.Contains(t, string(out), "https://github.com/Tiger-zero/Cat_two/blob/main/compose.yaml")
			require.Contains(t, string(out), "docker compose up -d")
			require.NotContains(t, string(out), "usememos/memos")
			require.NoDirExists(t, installDir)
		})
	}
}
