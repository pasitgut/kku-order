package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
)

// secretBox encrypts values stored in app_settings with AES-256-GCM. The key
// comes from SETTINGS_ENCRYPTION_KEY (base64 of 32 random bytes).
type secretBox struct {
	aead cipher.AEAD
}

// secretAdditionalData binds a ciphertext to its setting, so it cannot be moved to another row.
var secretAdditionalData = []byte("ocr_api_key")

func newSecretBox(encodedKey string) (*secretBox, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil {
		return nil, errors.New("SETTINGS_ENCRYPTION_KEY must be base64")
	}
	if len(raw) != 32 {
		return nil, errors.New("SETTINGS_ENCRYPTION_KEY must decode to 32 bytes")
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &secretBox{aead: aead}, nil
}

// newSecretBoxFromEnv returns nil when SETTINGS_ENCRYPTION_KEY is missing or invalid.
// Saving a key from the web is then refused, and the env key keeps working.
func newSecretBoxFromEnv() *secretBox {
	box, err := newSecretBox(os.Getenv("SETTINGS_ENCRYPTION_KEY"))
	if err != nil {
		return nil
	}
	return box
}

// seal returns base64(nonce || ciphertext) with a fresh random nonce.
func (b *secretBox) seal(plain string) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := b.aead.Seal(nonce, nonce, []byte(plain), secretAdditionalData)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (b *secretBox) open(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	nonceSize := b.aead.NonceSize()
	if err != nil || len(raw) < nonceSize {
		return "", errors.New("stored secret is not valid ciphertext")
	}
	plain, err := b.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], secretAdditionalData)
	if err != nil {
		return "", errors.New("stored secret failed authentication (wrong SETTINGS_ENCRYPTION_KEY or modified row)")
	}
	return string(plain), nil
}
