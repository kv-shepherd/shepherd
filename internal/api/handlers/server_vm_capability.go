package handlers

// server_vm_capability.go — P2-B Capability Warning helpers (ADR-0014 Layer 3)
//
// Non-blocking capability warnings are emitted via X-Capability-Warning HTTP response header
// when a VM creation request selects an instance size with hardware requirements (GPU / SR-IOV /
// HugePages) that no currently-HEALTHY cluster can satisfy.
//
// Design choices:
//   - Warning is NEVER blocking: the 202 response is always returned if the usecase succeeds.
//   - Reads from DB (Cluster.enabled_features persisted by health checker, P1-C).
//   - GPU/SRIOV/HUGEPAGES requirements are mapped to KubeVirt feature gate names via a static
//     table. These are the canonical feature gate keys used in `enabled_features`.
//   - Cost: at most 2 DB reads per POST /vms/request (1 InstanceSize GET + 1 Cluster query).
//     Cluster result is not cached — health data changes frequently; DB is the source of truth.

import (
	"context"
	"strings"

	"go.uber.org/zap"

	entcluster "kv-shepherd.io/shepherd/ent/cluster"
	entinstancesize "kv-shepherd.io/shepherd/ent/instancesize"
	"kv-shepherd.io/shepherd/internal/pkg/logger"
	"kv-shepherd.io/shepherd/internal/provider/capabilityutil"
)

// featureGateForRequiresGPU is the KubeVirt feature gate that must be in enabled_features
// for GPU passthrough to work. Mapped from InstanceSize.requires_gpu.
// See: https://kubevirt.io/user-guide/virtual_machines/dedicated_cpu_resources/
const featureGateForRequiresGPU = "GPU"

// featureGateForRequiresSRIOV is the KubeVirt feature gate for SR-IOV network passthrough.
// Mapped from InstanceSize.requires_sriov.
const featureGateForRequiresSRIOV = "SRIOV"

// featureGateForRequiresHugePages is the KubeVirt feature gate for HugePages memory backing.
// Mapped from InstanceSize.requires_hugepages.
const featureGateForRequiresHugePages = "HugePages"

// resolveCapabilityWarning returns a non-empty warning message if the given instance size
// requires hardware capabilities (GPU, SR-IOV, HugePages) that no HEALTHY cluster currently
// reports in its enabled_features.
//
// Returns "" if:
//   - The instance size has no special hardware requirements (most common case = fast path)
//   - At least one HEALTHY cluster satisfies all requirements
//   - Any DB read fails (fail-open: warning is suppressed, not propagated as error)
//
// The warning string format is human-readable and intended for frontend display.
// Machine-readable details are available in the response body's ticket status.
func (s *Server) resolveCapabilityWarning(ctx context.Context, instanceSizeID string) string {
	// Fast-path: load the instance size and check if it has any hardware requirements.
	sz, err := s.client.InstanceSize.
		Query().
		Where(entinstancesize.IDEQ(instanceSizeID)).
		Only(ctx)
	if err != nil {
		// Fail-open: if we cannot read the instance size, suppress warning entirely.
		logger.Debug("capability warning: failed to load instance size, skipping warning",
			zap.String("instance_size_id", instanceSizeID),
			zap.Error(err),
		)
		return ""
	}

	// Build the list of KubeVirt feature gates required by this instance size.
	var required []string
	if sz.RequiresGpu {
		required = append(required, featureGateForRequiresGPU)
	}
	if sz.RequiresSriov {
		required = append(required, featureGateForRequiresSRIOV)
	}
	if sz.RequiresHugepages {
		required = append(required, featureGateForRequiresHugePages)
	}

	// Fast-path: no hardware requirements → no warning needed.
	if len(required) == 0 {
		return ""
	}

	// Load all HEALTHY clusters and check if any satisfies all requirements.
	clusters, err := s.client.Cluster.
		Query().
		Where(entcluster.StatusEQ(entcluster.StatusHEALTHY)).
		All(ctx)
	if err != nil {
		// Fail-open: do not emit warning if cluster query fails.
		logger.Debug("capability warning: failed to query healthy clusters, skipping warning",
			zap.Error(err),
		)
		return ""
	}

	for _, cl := range clusters {
		if capabilityutil.HasAllCapabilities(cl.EnabledFeatures, required) {
			return "" // At least one cluster can handle this → no warning
		}
	}

	// No cluster satisfies all requirements → emit warning.
	return "No HEALTHY cluster currently has all required features: " + strings.Join(required, ", ") +
		". Your request has been accepted but may fail at approval time. " +
		"Contact your cluster administrator to enable the required feature gates."
}
