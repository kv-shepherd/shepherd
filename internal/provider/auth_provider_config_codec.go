package provider

import configcodec "kv-shepherd.io/shepherd/internal/provider/configcodec"

const AuthProviderProtectedFieldMask = configcodec.AuthProviderProtectedFieldMask

var (
	ErrAuthProviderConfigCodecKeyMissing = configcodec.ErrAuthProviderConfigCodecKeyMissing
	ErrAuthProviderConfigCiphertext      = configcodec.ErrAuthProviderConfigCiphertext
	ErrAuthProviderConfigDecrypt         = configcodec.ErrAuthProviderConfigDecrypt
)

type AuthProviderConfigCodec = configcodec.AuthProviderConfigCodec

func NewAuthProviderConfigCodec(encryptionKey []byte) *AuthProviderConfigCodec {
	return configcodec.NewAuthProviderConfigCodec(encryptionKey)
}
