package configcodec

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	admincontract "kv-shepherd.io/shepherd/internal/provider/admincontract"
	adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"
)

const (
	// AuthProviderProtectedFieldMask is the API/UI mask used for stored protected fields.
	AuthProviderProtectedFieldMask = "__SHEPHERD_PROTECTED_FIELD_SET__"
	authProviderConfigEncPrefix    = "enc:v1:"
)

var (
	ErrAuthProviderConfigCodecKeyMissing = errors.New("auth provider config encryption key is not configured")
	ErrAuthProviderConfigCiphertext      = errors.New("auth provider config ciphertext is invalid")
	ErrAuthProviderConfigDecrypt         = errors.New("auth provider config decryption failed")
)

// AuthProviderConfigCodec encrypts/decrypts sensitive auth-provider config fields.
//
// Sensitive fields are discovered from the provider config schema by looking for
// top-level properties with `format: password`.
type AuthProviderConfigCodec struct {
	encryptionKey []byte
}

func NewAuthProviderConfigCodec(encryptionKey []byte) *AuthProviderConfigCodec {
	return &AuthProviderConfigCodec{encryptionKey: encryptionKey}
}

func (c *AuthProviderConfigCodec) EncryptForStorage(authType string, raw map[string]interface{}) (map[string]interface{}, error) {
	if raw == nil {
		return nil, nil
	}
	cloned := cloneConfigMap(raw)
	for field := range authProviderSensitiveFields(authType) {
		value, ok := cloned[field]
		if !ok || value == nil {
			continue
		}
		plain := strings.TrimSpace(fmt.Sprint(value))
		if plain == "" {
			delete(cloned, field)
			continue
		}
		if plain == AuthProviderProtectedFieldMask || strings.HasPrefix(plain, authProviderConfigEncPrefix) {
			continue
		}
		encrypted, err := c.encryptString(plain)
		if err != nil {
			return nil, err
		}
		cloned[field] = encrypted
	}
	return cloned, nil
}

func (c *AuthProviderConfigCodec) DecryptForUse(authType string, stored map[string]interface{}) (map[string]interface{}, error) {
	if stored == nil {
		return nil, nil
	}
	cloned := cloneConfigMap(stored)
	for field := range authProviderSensitiveFields(authType) {
		value, ok := cloned[field]
		if !ok || value == nil {
			continue
		}
		decrypted, err := c.decryptMaybeString(fmt.Sprint(value))
		if err != nil {
			return nil, err
		}
		cloned[field] = decrypted
	}
	return cloned, nil
}

func (c *AuthProviderConfigCodec) SanitizeForAPI(authType string, stored map[string]interface{}) (map[string]interface{}, error) {
	if stored == nil {
		return nil, nil
	}
	cloned := cloneConfigMap(stored)
	for field := range authProviderSensitiveFields(authType) {
		value, ok := cloned[field]
		if !ok || value == nil {
			continue
		}
		decrypted, err := c.decryptMaybeString(fmt.Sprint(value))
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(decrypted) == "" {
			delete(cloned, field)
			continue
		}
		cloned[field] = AuthProviderProtectedFieldMask
	}
	return cloned, nil
}

func (c *AuthProviderConfigCodec) MergeForUpdate(authType string, existingStored, incoming map[string]interface{}) (map[string]interface{}, error) {
	if incoming == nil {
		return cloneConfigMap(existingStored), nil
	}
	merged := cloneConfigMap(existingStored)
	plainExisting, err := c.DecryptForUse(authType, existingStored)
	if err != nil {
		return nil, err
	}

	sensitive := authProviderSensitiveFields(authType)
	for key, raw := range incoming {
		if _, ok := sensitive[key]; !ok {
			merged[key] = raw
			continue
		}

		plain := strings.TrimSpace(fmt.Sprint(raw))
		switch plain {
		case "", AuthProviderProtectedFieldMask:
			if existingValue, ok := existingStored[key]; ok {
				merged[key] = existingValue
			} else {
				delete(merged, key)
			}
		default:
			encrypted, encErr := c.encryptString(plain)
			if encErr != nil {
				return nil, encErr
			}
			merged[key] = encrypted
		}
	}

	for field := range sensitive {
		if _, ok := incoming[field]; ok {
			continue
		}
		if existingValue, ok := existingStored[field]; ok {
			merged[field] = existingValue
			continue
		}
		if plainValue, ok := plainExisting[field]; ok && strings.TrimSpace(fmt.Sprint(plainValue)) != "" {
			encrypted, encErr := c.encryptString(strings.TrimSpace(fmt.Sprint(plainValue)))
			if encErr != nil {
				return nil, encErr
			}
			merged[field] = encrypted
		}
	}

	return merged, nil
}

func authProviderSensitiveFields(authType string) map[string]struct{} {
	adapter := adminglobal.Resolve(authType)
	if adapter == nil {
		return nil
	}
	describer, ok := adapter.(admincontract.AuthProviderAdminAdapterDescriber)
	if !ok {
		return nil
	}
	desc := describer.Describe()
	return schemaSensitiveFields(desc.ConfigSchema)
}

func schemaSensitiveFields(schema map[string]interface{}) map[string]struct{} {
	propertiesRaw, ok := schema["properties"]
	if !ok {
		return nil
	}
	properties, ok := propertiesRaw.(map[string]interface{})
	if !ok {
		return nil
	}
	fields := make(map[string]struct{})
	for key, raw := range properties {
		prop, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(fmt.Sprint(prop["format"])), "password") {
			fields[key] = struct{}{}
		}
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

func cloneConfigMap(raw map[string]interface{}) map[string]interface{} {
	if raw == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(raw))
	for k, v := range raw {
		cloned[k] = v
	}
	return cloned
}

func (c *AuthProviderConfigCodec) encryptString(plain string) (string, error) {
	if strings.TrimSpace(plain) == "" {
		return "", nil
	}
	if len(c.encryptionKey) == 0 {
		return "", ErrAuthProviderConfigCodecKeyMissing
	}
	block, err := aes.NewCipher(c.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("create auth provider config cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("create auth provider config aead: %w", err)
	}
	encrypted := aead.Seal(nil, nil, []byte(plain), nil)
	return authProviderConfigEncPrefix + base64.RawURLEncoding.EncodeToString(encrypted), nil
}

func (c *AuthProviderConfigCodec) decryptMaybeString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, authProviderConfigEncPrefix) {
		return value, nil
	}
	if len(c.encryptionKey) == 0 {
		return "", ErrAuthProviderConfigCodecKeyMissing
	}
	encoded := strings.TrimPrefix(value, authProviderConfigEncPrefix)
	ciphertext, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.Join(ErrAuthProviderConfigCiphertext, err)
	}
	block, err := aes.NewCipher(c.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("create auth provider config cipher: %w", err)
	}
	aead, err := cipher.NewGCMWithRandomNonce(block)
	if err != nil {
		return "", fmt.Errorf("create auth provider config aead: %w", err)
	}
	plaintext, err := aead.Open(nil, nil, ciphertext, nil)
	if err != nil {
		return "", errors.Join(ErrAuthProviderConfigDecrypt, err)
	}
	return string(plaintext), nil
}
