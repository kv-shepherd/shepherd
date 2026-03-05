package jobs

import (
	"errors"
	"fmt"
	"testing"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsIdempotentPowerConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		err       error
		want      bool
	}{
		{
			name:      "start already running",
			operation: "start",
			err:       errors.New("Operation cannot be fulfilled: VM is already running"),
			want:      true,
		},
		{
			name:      "stop already stopped",
			operation: "stop",
			err:       errors.New("Operation cannot be fulfilled: VM is already stopped"),
			want:      true,
		},
		{
			name:      "stop not running",
			operation: "stop",
			err:       errors.New("Operation cannot be fulfilled: VM is not running"),
			want:      true,
		},
		{
			name:      "restart not running is not idempotent",
			operation: "restart",
			err:       errors.New("Operation cannot be fulfilled: VM is not running"),
			want:      false,
		},
		{
			name:      "start generic error",
			operation: "start",
			err:       errors.New("connection refused"),
			want:      false,
		},
		{
			name:      "start manual start unsupported is idempotent",
			operation: "start",
			err: errors.New(
				"Operation cannot be fulfilled on virtualmachine.kubevirt.io \"vm-1\": Always does not support manual start requests",
			),
			want: true,
		},
		{
			name:      "stop manual stop unsupported is idempotent",
			operation: "stop",
			err: errors.New(
				"Operation cannot be fulfilled on virtualmachine.kubevirt.io \"vm-1\": Halted does not support manual stop requests",
			),
			want: true,
		},
		{
			name:      "nil error",
			operation: "start",
			err:       nil,
			want:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isIdempotentPowerConflict(tt.operation, tt.err)
			if got != tt.want {
				t.Fatalf("isIdempotentPowerConflict(%q, %v) = %v, want %v", tt.operation, tt.err, got, tt.want)
			}
		})
	}
}

func TestIsTerminalPowerError(t *testing.T) {
	t.Parallel()

	notFoundErr := k8serrors.NewNotFound(
		schema.GroupResource{Group: "kubevirt.io", Resource: "virtualmachine"},
		"vm-stopped",
	)

	tests := []struct {
		name      string
		operation string
		err       error
		want      bool
	}{
		{
			name:      "k8s status notfound",
			operation: "start",
			err:       notFoundErr,
			want:      true,
		},
		{
			name:      "wrapped k8s status notfound",
			operation: "start",
			err:       fmt.Errorf("start vm failed: %w", notFoundErr),
			want:      true,
		},
		{
			name:      "message fallback virtualmachine not found",
			operation: "start",
			err:       errors.New("virtualmachine.kubevirt.io \"vm-stopped\" not found"),
			want:      true,
		},
		{
			name:      "restart halted state conflict",
			operation: "restart",
			err:       errors.New("Operation cannot be fulfilled on virtualmachine.kubevirt.io \"vm-1\": VM is not running: Halted"),
			want:      true,
		},
		{
			name:      "restart runstrategy halted manual restart unsupported",
			operation: "restart",
			err:       errors.New("Operation cannot be fulfilled on virtualmachine.kubevirt.io \"vm-1\": RunStategy Halted does not support manual restart requests"),
			want:      true,
		},
		{
			name:      "cluster missing is not target vm notfound",
			operation: "start",
			err:       errors.New("get client for cluster demo: cluster demo not found"),
			want:      false,
		},
		{
			name:      "transient network error",
			operation: "restart",
			err:       errors.New("dial tcp 10.0.0.1:443: i/o timeout"),
			want:      false,
		},
		{
			name:      "start not running should not be terminal",
			operation: "start",
			err:       errors.New("Operation cannot be fulfilled: VM is not running"),
			want:      false,
		},
		{
			name:      "nil error",
			operation: "restart",
			err:       nil,
			want:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isTerminalPowerError(tt.operation, tt.err)
			if got != tt.want {
				t.Fatalf("isTerminalPowerError(%q, %v) = %v, want %v", tt.operation, tt.err, got, tt.want)
			}
		})
	}
}
