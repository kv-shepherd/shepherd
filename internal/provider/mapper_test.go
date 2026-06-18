package provider

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtv1 "kubevirt.io/api/core/v1"
)

func TestKubeVirtMapper_MapVM_MapsPrimaryIPAddress(t *testing.T) {
	t.Parallel()

	mapper := NewKubeVirtMapper()
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: v1.ObjectMeta{
			Name:      "vm-1",
			Namespace: "team-a",
		},
	}
	vmi := &kubevirtv1.VirtualMachineInstance{
		Status: kubevirtv1.VirtualMachineInstanceStatus{
			NodeName: "worker-a",
			GuestOSInfo: kubevirtv1.VirtualMachineInstanceGuestOSInfo{
				PrettyName: "Ubuntu 24.04.2 LTS",
				VersionID:  "24.04",
				ID:         "ubuntu",
			},
			Interfaces: []kubevirtv1.VirtualMachineInstanceNetworkInterface{
				{
					Name: "default",
					IP:   "10.0.0.18",
					IPs:  []string{"10.0.0.18", "fd00::18"},
				},
			},
		},
	}

	got, err := mapper.MapVM(vm, vmi)
	if err != nil {
		t.Fatalf("MapVM() error = %v", err)
	}
	if got.IPAddress != "10.0.0.18" {
		t.Fatalf("IPAddress = %q, want %q", got.IPAddress, "10.0.0.18")
	}
	if got.NodeName != "worker-a" {
		t.Fatalf("NodeName = %q, want %q", got.NodeName, "worker-a")
	}
	if got.OSName != "Ubuntu 24.04.2 LTS" {
		t.Fatalf("OSName = %q, want %q", got.OSName, "Ubuntu 24.04.2 LTS")
	}
	if got.OSVersion != "24.04" {
		t.Fatalf("OSVersion = %q, want %q", got.OSVersion, "24.04")
	}
	if got.OSFamily != "linux" {
		t.Fatalf("OSFamily = %q, want %q", got.OSFamily, "linux")
	}
}

func TestKubeVirtMapper_MapVM_MapsConsoleCapabilityDefaultsAndOverrides(t *testing.T) {
	t.Parallel()

	mapper := NewKubeVirtMapper()
	graphicsDisabled := false
	serialDisabled := false
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: v1.ObjectMeta{
			Name:      "vm-console",
			Namespace: "team-a",
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Devices: kubevirtv1.Devices{
							AutoattachGraphicsDevice: &graphicsDisabled,
							AutoattachSerialConsole:  &serialDisabled,
						},
					},
				},
			},
		},
	}

	got, err := mapper.MapVM(vm, nil)
	if err != nil {
		t.Fatalf("MapVM() error = %v", err)
	}
	if got.Spec.AutoattachGraphicsDevice {
		t.Fatal("AutoattachGraphicsDevice = true, want false")
	}
	if got.Spec.AutoattachSerialConsole {
		t.Fatal("AutoattachSerialConsole = true, want false")
	}

	defaultVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: v1.ObjectMeta{
			Name:      "vm-default-console",
			Namespace: "team-a",
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{},
				},
			},
		},
	}

	defaultGot, err := mapper.MapVM(defaultVM, nil)
	if err != nil {
		t.Fatalf("MapVM() default error = %v", err)
	}
	if !defaultGot.Spec.AutoattachGraphicsDevice {
		t.Fatal("default AutoattachGraphicsDevice = false, want true")
	}
	if !defaultGot.Spec.AutoattachSerialConsole {
		t.Fatal("default AutoattachSerialConsole = false, want true")
	}
}

func TestKubeVirtMapper_MapVM_MapsHugepagesPageSize(t *testing.T) {
	t.Parallel()

	mapper := NewKubeVirtMapper()
	guest := resource.MustParse("8Gi")
	vm := &kubevirtv1.VirtualMachine{
		ObjectMeta: v1.ObjectMeta{
			Name:      "vm-hugepages",
			Namespace: "team-a",
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{
					Domain: kubevirtv1.DomainSpec{
						Memory: &kubevirtv1.Memory{
							Guest: &guest,
							Hugepages: &kubevirtv1.Hugepages{
								PageSize: "2Mi",
							},
						},
					},
				},
			},
		},
	}

	got, err := mapper.MapVM(vm, nil)
	if err != nil {
		t.Fatalf("MapVM() error = %v", err)
	}
	if got.Spec.HugepagesPageSize != "2Mi" {
		t.Fatalf("HugepagesPageSize = %q, want 2Mi", got.Spec.HugepagesPageSize)
	}
}
