package store

import (
	stderrors "errors"
	"strings"

	"github.com/pkg/errors"
)

// ErrEmailTaken reports that another user already holds the email address.
var ErrEmailTaken = stderrors.New("email is already in use")

// ErrUsernameTaken reports that another user already holds the username.
var ErrUsernameTaken = stderrors.New("username is already in use")

// classifyUserUniqueViolation maps a SQLite error raised by a write to the
// user table onto one of the typed sentinels. It returns nil
// when the error is not a unique-constraint violation, so callers can fall
// through to their ordinary error handling.
func classifyUserUniqueViolation(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	_, target, found := strings.Cut(message, "UNIQUE constraint failed: ")
	if !found {
		return nil
	}
	switch {
	case strings.Contains(target, "email"):
		return errors.Wrap(ErrEmailTaken, message)
	case strings.Contains(target, "username"):
		return errors.Wrap(ErrUsernameTaken, message)
	}
	return nil
}
