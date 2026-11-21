package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
)

func twoFactorCipher(serverSecret string) (cipher.AEAD, error) {
	key := hmac.New(sha256.New, []byte(serverSecret))
	key.Write([]byte("cat-two/authenticator-secret/v1"))
	block, err := aes.NewCipher(key.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

// SealTwoFactorSecret encrypts an authenticator key and binds it to its account.
func SealTwoFactorSecret(serverSecret string, userID int32, secret string) (string, error) {
	aead, err := twoFactorCipher(serverSecret)
	if err != nil {
		return "", err
	}
	associatedData := strconv.AppendInt(nil, int64(userID), 10)
	data := aead.Seal(nil, nil, []byte(secret), associatedData)
	return base64.RawURLEncoding.EncodeToString(data), nil
}

// OpenTwoFactorSecret decrypts an authenticator key for its account.
func OpenTwoFactorSecret(serverSecret string, userID int32, encoded string) (string, error) {
	aead, err := twoFactorCipher(serverSecret)
	if err != nil {
		return "", err
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	associatedData := strconv.AppendInt(nil, int64(userID), 10)
	plain, err := aead.Open(nil, nil, data, associatedData)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
