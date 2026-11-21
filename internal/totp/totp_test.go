package totp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRFC6238AndReplayWindow(t *testing.T) {
	// RFC 6238 Appendix B's SHA-1 test secret and timestamps, with each
	// eight-digit answer truncated to the six digits used by authenticators.
	secret := encoding.EncodeToString([]byte("12345678901234567890"))
	for _, tt := range []struct {
		unix int64
		code string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	} {
		now := time.Unix(tt.unix, 0)
		step := tt.unix / periodSeconds
		got, matched := Match(secret, tt.code, now, step-1)
		require.True(t, matched, "time %d", tt.unix)
		require.Equal(t, step, got)

		_, matched = Match(secret, tt.code, now, step)
		require.False(t, matched, "replayed step %d", step)
		_, matched = Match(secret, tt.code, now.Add(30*time.Second), step-1)
		require.True(t, matched, "adjacent step %d", step)
		_, matched = Match(secret, tt.code, now.Add(60*time.Second), step-1)
		require.False(t, matched, "expired step %d", step)
	}
}
