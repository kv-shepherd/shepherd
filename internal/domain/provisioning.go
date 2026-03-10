package domain

import "time"

// ObjectReference identifies a Kubernetes object for event correlation.
type ObjectReference struct {
	Kind      string
	Name      string
	Namespace string
	UID       string
}

// ProvisioningCondition is a normalized condition for CDI-backed boot workflows.
type ProvisioningCondition struct {
	Type               string    `json:"type"`
	Status             string    `json:"status"`
	Reason             string    `json:"reason,omitempty"`
	Message            string    `json:"message,omitempty"`
	LastTransitionTime time.Time `json:"last_transition_time,omitempty"`
}

// ProvisioningEvent summarizes a Kubernetes Event relevant to VM provisioning.
type ProvisioningEvent struct {
	Type          string    `json:"type,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	Message       string    `json:"message,omitempty"`
	Count         int32     `json:"count,omitempty"`
	FirstObserved time.Time `json:"first_observed,omitempty"`
	LastObserved  time.Time `json:"last_observed,omitempty"`
}

// DataVolume models the subset of CDI DataVolume state needed by the platform.
type DataVolume struct {
	Name         string                  `json:"name"`
	Namespace    string                  `json:"namespace"`
	UID          string                  `json:"uid,omitempty"`
	ClaimName    string                  `json:"claim_name,omitempty"`
	Phase        string                  `json:"phase,omitempty"`
	Progress     string                  `json:"progress,omitempty"`
	RestartCount int32                   `json:"restart_count,omitempty"`
	Conditions   []ProvisioningCondition `json:"conditions,omitempty"`
}

// PersistentVolumeClaim models the subset of PVC state needed for observability.
type PersistentVolumeClaim struct {
	Name                  string `json:"name"`
	Namespace             string `json:"namespace"`
	Phase                 string `json:"phase,omitempty"`
	StorageClassName      string `json:"storage_class_name,omitempty"`
	VolumeMode            string `json:"volume_mode,omitempty"`
	RequestedStorageBytes int64  `json:"requested_storage_bytes,omitempty"`
	CapacityBytes         int64  `json:"capacity_bytes,omitempty"`
	CloneType             string `json:"clone_type,omitempty"`
	ClonePhase            string `json:"clone_phase,omitempty"`
	CloneFallbackReason   string `json:"clone_fallback_reason,omitempty"`
}

// StorageClass models the subset of StorageClass state needed for clone-expansion preflight.
type StorageClass struct {
	Name                 string `json:"name"`
	AllowVolumeExpansion bool   `json:"allow_volume_expansion,omitempty"`
}

// StorageProfile models the subset of CDI StorageProfile state needed for
// non-blocking clone advisories.
type StorageProfile struct {
	Name              string `json:"name"`
	CloneStrategy     string `json:"clone_strategy,omitempty"`
	DefaultVolumeMode string `json:"default_volume_mode,omitempty"`
}
