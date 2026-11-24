package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalSmokeBuildStagesSourceAndPreservesFrontend(t *testing.T) {
	for _, command := range []string{"bash", "jq", "tar"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s is required for the smoke script", command)
		}
	}

	repo := t.TempDir()
	write := func(name, contents string) {
		t.Helper()
		file := filepath.Join(repo, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
		require.NoError(t, os.WriteFile(file, []byte(contents), 0o600))
	}
	for _, name := range []string{"release_smoke_test.sh", "release_version.sh", "Dockerfile"} {
		contents, err := os.ReadFile(name)
		require.NoError(t, err)
		write("scripts/"+name, string(contents))
	}
	write(".gitignore", "server/frontend/dist/\nignored.txt\n")
	write("web/package.json", "{}")
	write("app.go", "committed source")
	write("deleted.txt", "old source")
	write("server/frontend/dist/index.html", "original index")
	write("server/frontend/dist/assets/original.js", "original script")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=Smoke Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false"}, args...)...)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
	}
	git("init", "-b", "main")
	git("add", ".")
	git("add", "-f", "server/frontend/dist/index.html")
	git("commit", "-m", "fixtures")
	write("app.go", "modified source")
	write("new source.txt", "untracked source")
	write("ignored.txt", "ignored")
	require.NoError(t, os.Remove(filepath.Join(repo, "deleted.txt")))

	mockBin := t.TempDir()
	writeExecutable(t, filepath.Join(mockBin, "pnpm"), `#!/bin/sh
set -eu
[ "$PWD" = "$SMOKE_TEST_REPO/web" ]
[ "$6" = --outDir ]
[ "$7" != "$SMOKE_TEST_REPO/server/frontend/dist" ]
mkdir -p "$7/assets"
printf 'new index' > "$7/index.html"
printf 'new script' > "$7/assets/new.js"
`)
	writeExecutable(t, filepath.Join(mockBin, "docker"), `#!/bin/sh
set -eu
case "$1" in
  info) exit 0 ;;
  build)
    for context do :; done
    [ "$context" != "$SMOKE_TEST_REPO" ]
    [ "$3" = "$context/scripts/Dockerfile" ]
    [ "$(cat "$context/app.go")" = 'modified source' ]
    [ "$(cat "$context/new source.txt")" = 'untracked source' ]
    [ "$(cat "$context/server/frontend/dist/index.html")" = 'new index' ]
    [ "$(cat "$context/server/frontend/dist/assets/new.js")" = 'new script' ]
    [ ! -e "$context/deleted.txt" ]
    [ ! -e "$context/ignored.txt" ]
    [ ! -e "$context/web" ]
    [ ! -e "$context/server/frontend/dist/assets/original.js" ]
    printf 'staged build verified\n'
    exit 42 ;;
  *) exit 1 ;;
esac
`)
	cmd := exec.Command("bash", filepath.Join(repo, "scripts/release_smoke_test.sh"))
	cmd.Env = append(os.Environ(), "PATH="+mockBin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"SMOKE_TEST_REPO="+repo, "MEMOS_SMOKE_CANDIDATE_IMAGE=", "MEMOS_SMOKE_PREVIOUS_IMAGE=", "MEMOS_SMOKE_KEEP_RESOURCES=")
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr, string(output))
	require.Equal(t, 42, exitErr.ExitCode(), string(output))
	require.Contains(t, string(output), "staged build verified")
	for name, want := range map[string]string{
		"server/frontend/dist/index.html":         "original index",
		"server/frontend/dist/assets/original.js": "original script",
	} {
		contents, err := os.ReadFile(filepath.Join(repo, name))
		require.NoError(t, err)
		require.Equal(t, want, string(contents))
	}
}
