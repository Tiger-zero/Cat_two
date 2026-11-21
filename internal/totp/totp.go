// Package totp implements RFC 6238 profiles used by authenticator apps.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"time"
)

const (
	periodSeconds = 30
	secretLength  = 20
)

var encoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a random 160-bit Base32 key without padding.
func GenerateSecret() (string, error) {
	var key [secretLength]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", err
	}
	return encoding.EncodeToString(key[:]), nil
}

// ProvisioningURI formats the account and key for an authenticator app.
func ProvisioningURI(issuer, account, secret string) string {
	values := url.Values{
		"algorithm": {"SHA1"},
		"digits":    {"6"},
		"issuer":    {issuer},
		"period":    {"30"},
		"secret":    {secret},
	}
	u := url.URL{Scheme: "otpauth", Host: "totp", Path: "/" + issuer + ":" + account, RawQuery: values.Encode()}
	return u.String()
}

// Match reports the newest matching step in the current or adjacent windows.
// A step that has already been used cannot match.
func Match(secret, code string, now time.Time, lastUsedStep int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	key, err := encoding.DecodeString(secret)
	if err != nil || len(key) != secretLength {
		return 0, false
	}

	currentStep := now.Unix() / periodSeconds
	for step := currentStep + 1; step >= currentStep-1 && step > lastUsedStep; step-- {
		value := codeAt(key, step)
		if subtle.ConstantTimeCompare([]byte(value), []byte(code)) == 1 {
			return step, true
		}
	}
	return 0, false
}

func codeAt(key []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 0x0f
	number := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", number%1_000_000)
}
