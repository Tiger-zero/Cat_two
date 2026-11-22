package messagecapture

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeWeComCallbackURL(t *testing.T) {
	const base = "https://notes.example.com"
	for input, expected := range map[string]string{
		"  ":                                "",
		" notes.example.com ":               base + WeComCallbackPath,
		"https://Notes.Example.com./":       base + WeComCallbackPath,
		base + WeComCallbackPath:            base + WeComCallbackPath,
		base + "/notes" + WeComCallbackPath: base + "/notes" + WeComCallbackPath,
		"notes.example.com:8443":            base + ":8443" + WeComCallbackPath,
		"HTTPS://notes.example.com:443":     base + ":443" + WeComCallbackPath,
		"http://notes.example.com:8080":     "http://notes.example.com:8080" + WeComCallbackPath,
	} {
		actual, err := NormalizeWeComCallbackURL(input)
		require.NoError(t, err, "input %q", input)
		require.Equal(t, expected, actual, "input %q", input)
	}
	for _, input := range []string{
		"ftp://notes.example.com",
		"https://notes.example.com/a",
		base + WeComCallbackPath + "/",
		base + "/notes/.." + WeComCallbackPath,
		base + "/integrations/%77ecom",
		base + "?",
		base + "#",
		base + "?token=private-input",
		"https://user:private-input@notes.example.com",
		"https://[notes.example.com]",
		"notes.example.com:",
		"notes.example.com:0",
		"notes.example.com:65536",
		"notes.example.com:abc",
		"https://[::1]",
		"https://127.0.0.1",
		"https://192.168.1.2",
		"012.000.000.001",
		"localhost",
		"nas",
		"nas.local",
		"nas.home.arpa",
		"notes..example.com",
		"notes.example-.com",
	} {
		actual, err := NormalizeWeComCallbackURL(input)
		require.ErrorIs(t, err, ErrInvalidConfiguration, "input %q", input)
		require.NotContains(t, err.Error(), "private-input")
		require.Empty(t, actual)
	}
}
