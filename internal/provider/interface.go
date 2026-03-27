// Package provider defines the infrastructure provider interfaces.
//
// This is the Anti-Corruption Layer between the platform and KubeVirt.
// All provider methods return domain types, not K8s types.
//
// Import Path (ADR-0016): kv-shepherd.io/shepherd/internal/provider
package provider

import infracontract "kv-shepherd.io/shepherd/internal/provider/infracontract"

type InfrastructureProvider = infracontract.InfrastructureProvider
type NamespaceProvisioner = infracontract.NamespaceProvisioner
type ProvisioningQueryProvider = infracontract.ProvisioningQueryProvider
type PVCClonePreflightProvider = infracontract.PVCClonePreflightProvider
type SnapshotProvider = infracontract.SnapshotProvider
type CloneProvider = infracontract.CloneProvider
type MigrationProvider = infracontract.MigrationProvider
type InstanceTypeProvider = infracontract.InstanceTypeProvider
type ConsoleProvider = infracontract.ConsoleProvider
type VMMutationProvider = infracontract.VMMutationProvider
type VNCStreamProvider = infracontract.VNCStreamProvider
type SerialConsoleStreamProvider = infracontract.SerialConsoleStreamProvider
type KubeVirtProvider = infracontract.KubeVirtProvider
type ListOptions = infracontract.ListOptions
type CredentialProvider = infracontract.CredentialProvider
