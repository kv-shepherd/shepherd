package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"kv-shepherd.io/shepherd/ent"
)

// ErrAuthProviderGenerationChanged reports that an operation authenticated or
// fetched data with an auth-provider configuration that is no longer current.
var ErrAuthProviderGenerationChanged = errors.New("auth provider generation changed")

// AuthProviderGeneration is a stable, secret-free snapshot of the provider
// fields that determine authentication and directory-sync behavior.
type AuthProviderGeneration struct {
	providerID   string
	authType     string
	updatedAt    time.Time
	configDigest [sha256.Size]byte
}

const authProviderGenerationBindingContext = "shepherd:auth-provider-generation:v1"

// CaptureAuthProviderGeneration binds an operation to the exact provider
// configuration it used for external I/O. The digest avoids retaining another
// copy of decrypted or encrypted provider credentials in memory.
func CaptureAuthProviderGeneration(row *ent.AuthProvider) (AuthProviderGeneration, error) {
	if row == nil {
		return AuthProviderGeneration{}, fmt.Errorf("capture auth provider generation: provider is required")
	}
	encodedConfig, err := json.Marshal(row.Config)
	if err != nil {
		return AuthProviderGeneration{}, fmt.Errorf("capture auth provider %q generation: encode config: %w", row.ID, err)
	}
	return AuthProviderGeneration{
		providerID: row.ID,
		authType:   row.AuthType,
		// PostgreSQL stores timestamptz with microsecond precision. Ent returns
		// the pre-insert time value from Save, so normalize here before comparing
		// it with a row reloaded from PostgreSQL.
		updatedAt:    row.UpdatedAt.UTC().Truncate(time.Microsecond),
		configDigest: sha256.Sum256(encodedConfig),
	}, nil
}

// StateBinding returns a keyed, opaque binding for carrying this generation in
// a browser-visible signed state token. Keying the binding prevents the token
// payload from becoming an oracle for guessing provider configuration values.
func (generation AuthProviderGeneration) StateBinding(key []byte) (string, error) {
	if len(key) == 0 {
		return "", fmt.Errorf("bind auth provider generation: key is required")
	}
	payload, err := json.Marshal(struct {
		ProviderID   string            `json:"provider_id"`
		AuthType     string            `json:"auth_type"`
		UpdatedAt    int64             `json:"updated_at_unix_micro"`
		ConfigDigest [sha256.Size]byte `json:"config_digest"`
	}{
		ProviderID:   generation.providerID,
		AuthType:     generation.authType,
		UpdatedAt:    generation.updatedAt.UnixMicro(),
		ConfigDigest: generation.configDigest,
	})
	if err != nil {
		return "", fmt.Errorf("bind auth provider generation: encode payload: %w", err)
	}

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(authProviderGenerationBindingContext))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// ValidateStateBinding rejects a state token that was issued for another
// provider generation. hmac.Equal keeps comparison timing independent of the
// matching prefix when a validly signed token reaches this boundary.
func (generation AuthProviderGeneration) ValidateStateBinding(key []byte, binding string) error {
	expected, err := generation.StateBinding(key)
	if err != nil {
		return err
	}
	if !hmac.Equal([]byte(expected), []byte(binding)) {
		return fmt.Errorf("%w for %q", ErrAuthProviderGenerationChanged, generation.providerID)
	}
	return nil
}

// Validate rejects a freshly loaded row when any generation field changed.
// Callers must invoke this while holding LockAuthProviderMutation so the
// comparison and dependent write have one serialization point.
func (generation AuthProviderGeneration) Validate(row *ent.AuthProvider) error {
	current, err := CaptureAuthProviderGeneration(row)
	if err != nil {
		return err
	}
	if generation.providerID != current.providerID ||
		generation.authType != current.authType ||
		!generation.updatedAt.Equal(current.updatedAt) ||
		generation.configDigest != current.configDigest {
		return fmt.Errorf("%w for %q", ErrAuthProviderGenerationChanged, current.providerID)
	}
	return nil
}
