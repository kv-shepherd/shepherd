package provider

import runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"

// ExternalCohort is the provider-agnostic external organization shape.
type ExternalCohort = runtimecontract.ExternalCohort

// AuthProfileAttributes stores display-only external profile metadata.
type AuthProfileAttributes = runtimecontract.AuthProfileAttributes

type AuthInteractionType = runtimecontract.AuthInteractionType

const (
	AuthInteractionRedirect    = runtimecontract.AuthInteractionRedirect
	AuthInteractionCredentials = runtimecontract.AuthInteractionCredentials
)

// AuthResult is the canonical runtime auth result consumed by core.
type AuthResult = runtimecontract.AuthResult

// AuthStartRequest carries core-owned login parameters into the provider.
type AuthStartRequest = runtimecontract.AuthStartRequest

// AuthStartResponse carries the provider-owned redirect URL back to core.
type AuthStartResponse = runtimecontract.AuthStartResponse

// AuthStartError indicates a provider-owned login-start validation failure.
type AuthStartError = runtimecontract.AuthStartError

// NewAuthStartError constructs a structured login-start error.
func NewAuthStartError(code, message string) error {
	return runtimecontract.NewAuthStartError(code, message)
}

// AuthCallbackRequest is the opaque callback envelope forwarded to providers.
type AuthCallbackRequest = runtimecontract.AuthCallbackRequest

// AuthCredentialRequest is the opaque credential envelope forwarded to providers.
type AuthCredentialRequest = runtimecontract.AuthCredentialRequest

// AuthLoginMode describes one provider-owned login entrypoint.
type AuthLoginMode = runtimecontract.AuthLoginMode

// AuthRuntimeDescriptor exposes public runtime metadata for login UX.
type AuthRuntimeDescriptor = runtimecontract.AuthRuntimeDescriptor

// AuthRuntimeCapability is an optional auth-provider runtime extension.
type AuthRuntimeCapability = runtimecontract.AuthRuntimeCapability

// AuthCallbackOriginDescriber exposes exact callback-submitting origins.
type AuthCallbackOriginDescriber = runtimecontract.AuthCallbackOriginDescriber

// AuthCredentialCapability is an optional auth-provider runtime extension for
// direct credential submission flows.
type AuthCredentialCapability = runtimecontract.AuthCredentialCapability

// AuthRuntimeDescriber exposes public runtime metadata when supported.
type AuthRuntimeDescriber = runtimecontract.AuthRuntimeDescriber

// AuthCredentialError indicates a provider-owned credential-login failure.
type AuthCredentialError = runtimecontract.AuthCredentialError

// NewAuthCredentialError constructs a structured credential-login error.
func NewAuthCredentialError(code, message string) error {
	return runtimecontract.NewAuthCredentialError(code, message)
}
