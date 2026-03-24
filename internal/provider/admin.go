package provider

import admincontract "kv-shepherd.io/shepherd/internal/provider/admincontract"

// AuthProviderTypeDescriptor describes a provider type exposed to admin UI/API.
type AuthProviderTypeDescriptor = admincontract.AuthProviderTypeDescriptor

// AuthProviderSampleField is the normalized sample-field contract exposed by plugins.
type AuthProviderSampleField = admincontract.AuthProviderSampleField

// AuthProviderAdminAdapter defines the plugin contract for auth provider management endpoints.
type AuthProviderAdminAdapter = admincontract.AuthProviderAdminAdapter

// AuthProviderAdminAdapterDescriber is an optional adapter extension for metadata exposure.
type AuthProviderAdminAdapterDescriber = admincontract.AuthProviderAdminAdapterDescriber
