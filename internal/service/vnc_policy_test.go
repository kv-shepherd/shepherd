package service

import (
	"testing"
	"time"

	"kv-shepherd.io/shepherd/ent/namespaceregistry"
	entvm "kv-shepherd.io/shepherd/ent/vm"
)

func TestEvaluateVNCRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		env               namespaceregistry.Environment
		vmStatus          entvm.Status
		hasPermission     bool
		hasPendingRequest bool
		want              VNCDecision
	}{
		{
			name:          "test environment allows direct access",
			env:           namespaceregistry.EnvironmentTest,
			vmStatus:      entvm.StatusRUNNING,
			hasPermission: true,
			want: VNCDecision{
				Allowed:         true,
				RequireApproval: false,
			},
		},
		{
			name:          "prod environment requires approval",
			env:           namespaceregistry.EnvironmentProd,
			vmStatus:      entvm.StatusRUNNING,
			hasPermission: true,
			want: VNCDecision{
				Allowed:         true,
				RequireApproval: true,
			},
		},
		{
			name:              "prod duplicate pending rejected",
			env:               namespaceregistry.EnvironmentProd,
			vmStatus:          entvm.StatusRUNNING,
			hasPermission:     true,
			hasPendingRequest: true,
			want: VNCDecision{
				RejectCode: "DUPLICATE_PENDING_VNC_REQUEST",
			},
		},
		{
			name:          "vm not running rejected",
			env:           namespaceregistry.EnvironmentTest,
			vmStatus:      entvm.StatusSTOPPED,
			hasPermission: true,
			want: VNCDecision{
				RejectCode: "VM_NOT_RUNNING",
			},
		},
		{
			name:     "missing vnc permission rejected",
			env:      namespaceregistry.EnvironmentTest,
			vmStatus: entvm.StatusRUNNING,
			want:     VNCDecision{RejectCode: "FORBIDDEN"},
		},
		{
			name:          "unsupported environment rejected",
			env:           namespaceregistry.Environment("staging"),
			vmStatus:      entvm.StatusRUNNING,
			hasPermission: true,
			want: VNCDecision{
				RejectCode: "UNSUPPORTED_NAMESPACE_ENVIRONMENT",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := EvaluateVNCRequest(tc.env, tc.vmStatus, tc.hasPermission, tc.hasPendingRequest)
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
