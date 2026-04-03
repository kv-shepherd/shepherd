package runtimecontract

import "context"

// ExternalCohort is the provider-agnostic external organization shape.
type ExternalCohort struct {
	Kind        string `json:"kind"`
	Key         string `json:"key"`
	DisplayName string `json:"display_name,omitempty"`
}

// AuthProfileAttributes stores display-only external profile metadata.
type AuthProfileAttributes map[string]interface{}

type AuthDirectoryAuthorityMode string

const (
	AuthDirectoryAuthorityAuthoritative AuthDirectoryAuthorityMode = "authoritative"
	AuthDirectoryAuthorityLoginOnly     AuthDirectoryAuthorityMode = "login_only"
)

type AuthInteractionType string

const (
	AuthInteractionRedirect    AuthInteractionType = "redirect"
	AuthInteractionCredentials AuthInteractionType = "credentials"
)

// AuthResult is the canonical runtime auth result consumed by core.
type AuthResult struct {
	ExternalID         string                     `json:"external_id"`
	Username           string                     `json:"username"`
	DisplayName        string                     `json:"display_name"`
	Email              string                     `json:"email,omitempty"`
	Enabled            bool                       `json:"enabled"`
	Cohorts            []ExternalCohort           `json:"cohorts,omitempty"`
	ProfileAttributes  AuthProfileAttributes      `json:"profile_attributes,omitempty"`
	DirectoryAuthority AuthDirectoryAuthorityMode `json:"directory_authority,omitempty"`
}

// AuthStartRequest carries core-owned login parameters into the provider.
type AuthStartRequest struct {
	LoginMode   string `json:"login_mode,omitempty"`
	ReturnTo    string `json:"return_to,omitempty"`
	CallbackURL string `json:"callback_url,omitempty"`
	State       string `json:"state,omitempty"`
	UserAgent   string `json:"user_agent,omitempty"`
}

// AuthStartResponse carries the provider-owned redirect URL back to core.
type AuthStartResponse struct {
	RedirectURL string `json:"redirect_url,omitempty"`
}

// AuthStartError indicates a provider-owned login-start validation failure.
type AuthStartError struct {
	Code    string
	Message string
}

func (e *AuthStartError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// NewAuthStartError constructs a structured login-start error.
func NewAuthStartError(code, message string) error {
	return &AuthStartError{
		Code:    code,
		Message: message,
	}
}

// AuthCallbackRequest is the opaque callback envelope forwarded to providers.
type AuthCallbackRequest struct {
	Method     string              `json:"method,omitempty"`
	Query      map[string][]string `json:"query,omitempty"`
	Form       map[string][]string `json:"form,omitempty"`
	Header     map[string][]string `json:"header,omitempty"`
	RemoteAddr string              `json:"remote_addr,omitempty"`
}

// AuthCredentialRequest is the opaque credential envelope forwarded to providers.
type AuthCredentialRequest struct {
	LoginMode   string                 `json:"login_mode,omitempty"`
	Credentials map[string]interface{} `json:"credentials,omitempty"`
	UserAgent   string                 `json:"user_agent,omitempty"`
}

// AuthLoginMode describes one provider-owned login entrypoint.
type AuthLoginMode struct {
	Key           string                 `json:"key"`
	DisplayName   string                 `json:"display_name"`
	Description   string                 `json:"description,omitempty"`
	Interaction   AuthInteractionType    `json:"interaction,omitempty"`
	RequestSchema map[string]interface{} `json:"request_schema,omitempty"`
	Default       bool                   `json:"default,omitempty"`
}

// AuthRuntimeDescriptor exposes public runtime metadata for login UX.
type AuthRuntimeDescriptor struct {
	DisplayName string          `json:"display_name"`
	Description string          `json:"description,omitempty"`
	LoginModes  []AuthLoginMode `json:"login_modes,omitempty"`
}

// AuthRuntimeCapability is an optional auth-provider runtime extension.
type AuthRuntimeCapability interface {
	StartLogin(ctx context.Context, config map[string]interface{}, req AuthStartRequest) (*AuthStartResponse, error)
	CompleteLogin(ctx context.Context, config map[string]interface{}, req AuthCallbackRequest) (*AuthResult, error)
}

// AuthCredentialCapability is an optional auth-provider runtime extension for
// direct credential submission flows.
type AuthCredentialCapability interface {
	AuthenticateCredentials(ctx context.Context, config map[string]interface{}, req AuthCredentialRequest) (*AuthResult, error)
}

// AuthRuntimeDescriber exposes public runtime metadata when supported.
type AuthRuntimeDescriber interface {
	DescribeRuntimeAuth() AuthRuntimeDescriptor
}

// AuthCredentialError indicates a provider-owned credential-login failure.
type AuthCredentialError struct {
	Code    string
	Message string
}

func (e *AuthCredentialError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// NewAuthCredentialError constructs a structured credential-login error.
func NewAuthCredentialError(code, message string) error {
	return &AuthCredentialError{
		Code:    code,
		Message: message,
	}
}
