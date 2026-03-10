package service

import (
	"time"

	entvm "kv-shepherd.io/shepherd/ent/vm"
)

const (
	// Stage 6 baseline token TTL (master-flow.md Stage 6).
	DefaultVNCTokenTTL = 2 * time.Hour
)

// VNCDecision captures Stage 6 request decision outcome.
type VNCDecision struct {
	Allowed         bool
	RequireApproval bool
	RejectCode      string
}

// EvaluateVNCRequest applies the Stage 6 interaction policy after the caller
// has already resolved whether VNC access requires approval.
//
// 1. requires vnc:access permission
// 2. VM must be RUNNING
// 3. no approval required -> direct token issuance
// 4. approval required -> reject duplicate pending requests
func EvaluateVNCRequest(vmStatus entvm.Status, hasPermission, hasPendingRequest, requireApproval bool) VNCDecision {
	if !hasPermission {
		return VNCDecision{RejectCode: "FORBIDDEN"}
	}
	if vmStatus != entvm.StatusRUNNING {
		return VNCDecision{RejectCode: "VM_NOT_RUNNING"}
	}
	if requireApproval {
		if hasPendingRequest {
			return VNCDecision{RejectCode: "DUPLICATE_PENDING_VNC_REQUEST"}
		}
		return VNCDecision{Allowed: true, RequireApproval: true}
	}
	return VNCDecision{Allowed: true, RequireApproval: false}
}

// VNCTokenClaims is the canonical token claim shape used by Stage 6 policy tests.
type VNCTokenClaims struct {
	Subject   string
	VMID      string
	ClusterID string
	Namespace string
	ExpiresAt time.Time
	JTI       string
	SingleUse bool
}

// BuildVNCTokenClaims builds baseline VNC claims with single-use semantics and default TTL.
func BuildVNCTokenClaims(now time.Time, ttl time.Duration, subject, vmID, clusterID, namespace, jti string) VNCTokenClaims {
	if ttl <= 0 {
		ttl = DefaultVNCTokenTTL
	}
	return VNCTokenClaims{
		Subject:   subject,
		VMID:      vmID,
		ClusterID: clusterID,
		Namespace: namespace,
		ExpiresAt: now.Add(ttl).UTC(),
		JTI:       jti,
		SingleUse: true,
	}
}
