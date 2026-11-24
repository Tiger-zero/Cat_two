package store

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyUserUniqueViolation(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    error
	}{
		{name: "sqlite email", message: "UNIQUE constraint failed: user.email", want: ErrEmailTaken},
		{name: "sqlite username", message: "UNIQUE constraint failed: user.username", want: ErrUsernameTaken},
		{name: "wrapped driver error", message: "failed to create user: UNIQUE constraint failed: user.email", want: ErrEmailTaken},
		{name: "not a unique violation", message: "database is locked", want: nil},
		{name: "unique violation on another table", message: "UNIQUE constraint failed: memo.uid", want: nil},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := classifyUserUniqueViolation(errors.New(test.message))
			if test.want == nil {
				require.NoError(t, got)
				return
			}
			require.ErrorIs(t, got, test.want)
			require.Contains(t, got.Error(), test.message)
		})
	}
	require.NoError(t, classifyUserUniqueViolation(nil))
}
