package configcodec

import (
	"context"
	"strings"
	"testing"

	admincontract "kv-shepherd.io/shepherd/internal/provider/admincontract"
	adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"
)

type sensitiveStubAdapter struct{}

func (sensitiveStubAdapter) Type() string                                { return "configcodec-sensitive-stub" }
func (sensitiveStubAdapter) ValidateConfig(map[string]interface{}) error { return nil }
func (sensitiveStubAdapter) TestConnection(context.Context, map[string]interface{}) (ok bool, message string, err error) {
	return true, "ok", nil
}
func (sensitiveStubAdapter) SampleFields(context.Context, map[string]interface{}) ([]admincontract.AuthProviderSampleField, error) {
	return nil, nil
}
func (sensitiveStubAdapter) Describe() admincontract.AuthProviderTypeDescriptor {
	return admincontract.AuthProviderTypeDescriptor{
		Type: "configcodec-sensitive-stub",
		ConfigSchema: map[string]interface{}{
			"properties": map[string]interface{}{
				"client_secret": map[string]interface{}{
					"type":   "string",
					"format": "password",
				},
			},
		},
	}
}

func registerSensitiveStubAdapter(t *testing.T) {
	t.Helper()
	if err := adminglobal.Register(sensitiveStubAdapter{}); err != nil && !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("Register() error = %v", err)
	}
}

func TestEncryptDecryptAndSanitize(t *testing.T) {
	registerSensitiveStubAdapter(t)

	codec := NewAuthProviderConfigCodec([]byte("0123456789abcdef0123456789abcdef"))
	encrypted, err := codec.EncryptForStorage("configcodec-sensitive-stub", map[string]interface{}{
		"client_secret": "secret-value",
		"plain":         "visible",
	})
	if err != nil {
		t.Fatalf("EncryptForStorage() error = %v", err)
	}
	if encrypted["client_secret"] == "secret-value" {
		t.Fatal("client_secret remained plaintext")
	}

	decrypted, err := codec.DecryptForUse("configcodec-sensitive-stub", encrypted)
	if err != nil {
		t.Fatalf("DecryptForUse() error = %v", err)
	}
	if decrypted["client_secret"] != "secret-value" {
		t.Fatalf("decrypted client_secret = %#v, want secret-value", decrypted["client_secret"])
	}

	sanitized, err := codec.SanitizeForAPI("configcodec-sensitive-stub", encrypted)
	if err != nil {
		t.Fatalf("SanitizeForAPI() error = %v", err)
	}
	if sanitized["client_secret"] != AuthProviderProtectedFieldMask {
		t.Fatalf("sanitized client_secret = %#v, want protected mask", sanitized["client_secret"])
	}
}
