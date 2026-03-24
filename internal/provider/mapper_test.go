package provider

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"

	"kv-shepherd.io/shepherd/internal/domain"
)

func TestMapVMStatus_UsesPrintableStatusMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		printable kubevirtv1.VirtualMachinePrintableStatus
		want      domain.VMStatus
	}{
		{
			name:      "starting_maps_to_starting",
			printable: kubevirtv1.VirtualMachineStatusStarting,
			want:      domain.VMStatusStarting,
		},
		{
			name:      "waiting_for_volume_binding_maps_to_pending",
			printable: kubevirtv1.VirtualMachineStatusWaitingForVolumeBinding,
			want:      domain.VMStatusPending,
		},
		{
			name:      "waiting_for_receiver_maps_to_pending",
			printable: kubevirtv1.VirtualMachineStatusWaitingForReceiver,
			want:      domain.VMStatusPending,
		},
		{
			name:      "crashloop_maps_to_failed",
			printable: kubevirtv1.VirtualMachineStatusCrashLoopBackOff,
			want:      domain.VMStatusFailed,
		},
		{
			name:      "err_image_pull_maps_to_failed",
			printable: kubevirtv1.VirtualMachineStatusErrImagePull,
			want:      domain.VMStatusFailed,
		},
		{
			name:      "unknown_maps_to_unknown",
			printable: kubevirtv1.VirtualMachineStatusUnknown,
			want:      domain.VMStatusUnknown,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vm := &kubevirtv1.VirtualMachine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vm-1",
					Namespace: "ns-1",
				},
				Status: kubevirtv1.VirtualMachineStatus{
					PrintableStatus: tc.printable,
				},
			}

			if got := mapVMStatus(vm, nil); got != tc.want {
				t.Fatalf("mapVMStatus(printable=%s) = %s, want %s", tc.printable, got, tc.want)
			}
		})
	}
}

func TestMapVMStatus_FallbackRunStrategyHaltedMapsToStopped(t *testing.T) {
	t.Parallel()

	strategy := kubevirtv1.RunStrategyHalted
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vm-1",
			Namespace: "ns-1",
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: &strategy,
		},
	}

	if got := mapVMStatus(vm, nil); got != domain.VMStatusStopped {
		t.Fatalf("mapVMStatus(runStrategy=Halted) = %s, want %s", got, domain.VMStatusStopped)
	}
}
