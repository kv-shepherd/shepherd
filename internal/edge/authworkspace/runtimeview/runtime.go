package runtimeview

import (
	"errors"
	"strings"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/internal/api/generated"
	admincontract "kv-shepherd.io/shepherd/internal/provider/admincontract"
	adminglobal "kv-shepherd.io/shepherd/internal/provider/adminglobal"
	runtimecontract "kv-shepherd.io/shepherd/internal/provider/runtimecontract"
)

var ErrAuthProviderAdapterNotFound = errors.New("auth provider adapter not found")

func BuildRuntimeDescriptor(row *ent.AuthProvider) (generated.AuthProviderRuntimeDescriptor, error) {
	adapter := adminglobal.Resolve(row.AuthType)
	if adapter == nil {
		return generated.AuthProviderRuntimeDescriptor{}, ErrAuthProviderAdapterNotFound
	}
	runtimeDescriptor, supportsRedirect, supportsCredentials := AuthRuntimeDescriptorForAdapter(adapter, row.Name)
	return generated.AuthProviderRuntimeDescriptor{
		Supported:             supportsRedirect || supportsCredentials,
		DisplayName:           runtimeDescriptor.DisplayName,
		Description:           runtimeDescriptor.Description,
		SupportsRedirect:      supportsRedirect,
		SupportsCredentials:   supportsCredentials,
		RequiresPublicBaseUrl: supportsRedirect,
		LoginModes:            AuthLoginModesToAPI(runtimeDescriptor.LoginModes),
	}, nil
}

func BuildLoginProvider(row *ent.AuthProvider) (generated.LoginAuthProvider, bool) {
	adapter := adminglobal.Resolve(row.AuthType)
	if adapter == nil {
		return generated.LoginAuthProvider{}, false
	}
	runtimeDescriptor, hasRedirectRuntime, hasCredentialRuntime := AuthRuntimeDescriptorForAdapter(adapter, row.Name)
	if !hasRedirectRuntime && !hasCredentialRuntime {
		return generated.LoginAuthProvider{}, false
	}
	return generated.LoginAuthProvider{
		Id:          row.ID,
		Name:        row.Name,
		AuthType:    row.AuthType,
		DisplayName: strings.TrimSpace(runtimeDescriptor.DisplayName),
		Description: strings.TrimSpace(runtimeDescriptor.Description),
		LoginModes:  AuthLoginModesToAPI(runtimeDescriptor.LoginModes),
	}, true
}

func AuthLoginModesToAPI(modes []runtimecontract.AuthLoginMode) []generated.AuthLoginMode {
	if len(modes) == 0 {
		return nil
	}
	items := make([]generated.AuthLoginMode, 0, len(modes))
	for _, mode := range modes {
		items = append(items, generated.AuthLoginMode{
			Key:           strings.TrimSpace(mode.Key),
			DisplayName:   strings.TrimSpace(mode.DisplayName),
			Description:   strings.TrimSpace(mode.Description),
			Interaction:   generated.AuthLoginModeInteraction(mode.Interaction),
			RequestSchema: mode.RequestSchema,
			Default:       mode.Default,
		})
	}
	return items
}

func AuthRuntimeDescriptorForAdapter(
	adapter admincontract.AuthProviderAdminAdapter,
	fallbackDisplayName string,
) (runtimecontract.AuthRuntimeDescriptor, bool, bool) {
	_, hasRedirectRuntime := adapter.(runtimecontract.AuthRuntimeCapability)
	_, hasCredentialRuntime := adapter.(runtimecontract.AuthCredentialCapability)
	descriptor := runtimecontract.AuthRuntimeDescriptor{
		DisplayName: fallbackDisplayName,
	}
	if describer, ok := adapter.(runtimecontract.AuthRuntimeDescriber); ok {
		descriptor = describer.DescribeRuntimeAuth()
	}
	if strings.TrimSpace(descriptor.DisplayName) == "" {
		descriptor.DisplayName = fallbackDisplayName
	}
	return descriptor, hasRedirectRuntime, hasCredentialRuntime
}
