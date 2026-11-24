package testutil

import (
	"crypto/sha256"
	"encoding/hex"
)

// LegacyPATValue builds a historic SQLite setting value for authentication tests.
func LegacyPATValue(token string) string {
	digest := sha256.Sum256([]byte(token))
	return `{"tokens":[{"tokenId":"retired-test-token","tokenHash":"` + hex.EncodeToString(digest[:]) + `"}]}`
}
