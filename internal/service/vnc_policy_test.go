package service

import (
	"testing"
	"time"

	entvm "kv-shepherd.io/shepherd/ent/vm"
)

func TestEvaluateVNCRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		vmStatus          entvm.Status
		hasPermission     bool
		hasPendingRequest bool
		requireApproval   bool
		want              VNCDecision
	}{
		{
			name:            "direct access allowed when approval not required",
			vmStatus:        entvm.StatusRUNNING,
			hasPermission:   true,
			requireApproval: false,
			want: VNCDecision{
				Allowed:         true,
				RequireApproval: false,
			},
		},
		{
			name:            "approval required path is surfaced",
			vmStatus:        entvm.StatusRUNNING,
			hasPermission:   true,
			requireApproval: true,
			want: VNCDecision{
				Allowed:         true,
				RequireApproval: true,
			},
		},
		{
			name:              "prod duplicate pending rejected",
			vmStatus:          entvm.StatusRUNNING,
			hasPermission:     true,
			hasPendingRequest: true,
			requireApproval:   true,
			want: VNCDecision{
				RejectCode: "DUPLICATE_PENDING_VNC_REQUEST",
			},
		},
		{
			name:            "vm not running rejected",
			vmStatus:        entvm.StatusSTOPPED,
			hasPermission:   true,
			requireApproval: false,
			want: VNCDecision{
				RejectCode: "VM_NOT_RUNNING",
			},
		},
		{
			name:            "missing vnc permission rejected",
			vmStatus:        entvm.StatusRUNNING,
			requireApproval: false,
			want:            VNCDecision{RejectCode: "FORBIDDEN"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := EvaluateVNCRequest(tc.vmStatus, tc.hasPermission, tc.hasPendingRequest, tc.requireApproval)
			if got != tc.want {
				t.Fatalf("EvaluateVNCRequest() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestBuildVNCTokenClaims(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)

	t.Run("uses default ttl and single use", func(t *testing.T) {
		t.Parallel()

		claims := BuildVNCTokenClaims(now, 0, "user-1", "vm-1", "cluster-a", "ns-a", "jti-1")
		if !claims.SingleUse {
			t.Fatal("SingleUse = false, want true")
		}
		if got, want := claims.ExpiresAt, now.Add(DefaultVNCTokenTTL); !got.Equal(want) {
			t.Fatalf("ExpiresAt = %s, want %s", got, want)
		}
	})

	t.Run("uses explicit ttl", func(t *testing.T) {
		t.Parallel()

		claims := BuildVNCTokenClaims(now, 30*time.Minute, "user-1", "vm-1", "cluster-a", "ns-a", "jti-1")
		if got, want := claims.ExpiresAt, now.Add(30*time.Minute); !got.Equal(want) {
			t.Fatalf("ExpiresAt = %s, want %s", got, want)
		}
	})
}
