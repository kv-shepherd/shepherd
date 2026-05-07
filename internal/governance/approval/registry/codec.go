package registry

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	ProtectedSigningKeyMask = "__SHEPHERD_PROTECTED_FIELD_SET__"
	signingKeyEncPrefix     = "approval-signing-key:v1:"
)

var (
	ErrSigningKeyEncryptionKeyMissing = errors.New("external approval signing key encryption key is not configured")
	ErrSigningKeyCiphertext           = errors.New("external approval signing key ciphertext is invalid")
	ErrSigningKeyDecrypt              = errors.New("external approval signing key decryption failed")
	ErrSigningKeyKeyMismatch          = errors.New("external approval signing key was encrypted with an unavailable key")
)

type SigningKeyCodec struct {
	encryptionKey []byte
}

func NewSigningKeyCodec(encryptionKey []byte) *SigningKeyCodec {
	return &SigningKeyCodec{encryptionKey: encryptionKey}
}

func (c *SigningKeyCodec) KeyID() string {
	if len(c.encryptionKey) == 0 {
		return ""
	}
	sum := sha256.Sum256(c.encryptionKey)
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func (c *SigningKeyCodec) EncryptForStorage(plain string) (ciphertext, keyID string, err error) {
	value := strings.TrimSpace(plain)
	if value == "" {
		return "", "", nil
	}
	if len(c.encryptionKey) == 0 {
		return "", "", ErrSigningKeyEncryptionKeyMissing
	}
	block, err := aes.NewCipher(c.encryptionKey)
	if err != nil {
		return "", "", fmt.Errorf("create external approval signing key cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", "", fmt.Errorf("create external approval signing key aead: %w", err)
	}
	encrypted := aead.Seal(nil, nil, []byte(value), nil)
	return signingKeyEncPrefix + base64.RawURLEncoding.EncodeToString(encrypted), c.KeyID(), nil
}

func (c *SigningKeyCodec) DecryptForUse(ciphertext, keyID string) (string, error) {
	stored := strings.TrimSpace(ciphertext)
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, signingKeyEncPrefix) {
		return stored, nil
	}
	if len(c.encryptionKey) == 0 {
		return "", ErrSigningKeyEncryptionKeyMissing
	}
	if expected := strings.TrimSpace(keyID); expected != "" && expected != c.KeyID() {
		return "", fmt.Errorf("%w: stored=%s current=%s", ErrSigningKeyKeyMismatch, expected, c.KeyID())
	}
	raw := strings.TrimPrefix(stored, signingKeyEncPrefix)
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return "", errors.Join(ErrSigningKeyCiphertext, err)
	}
	block, err := aes.NewCipher(c.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("create external approval signing key cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("create external approval signing key aead: %w", err)
	}
	plain, err := aead.Open(nil, nil, data, nil)
	if err != nil {
		return "", errors.Join(ErrSigningKeyDecrypt, err)
	}
	return string(plain), nil
}

func isProtectedSigningKeyMask(value string) bool {
	return strings.TrimSpace(value) == ProtectedSigningKeyMask
}
