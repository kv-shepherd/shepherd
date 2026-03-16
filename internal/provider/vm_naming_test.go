package provider

import "testing"

func TestDefaultVMReferenceNamingProfile_CompatibilityDefaults(t *testing.T) {
	if defaultVMReferenceNamingProfile.RootDiskName != "rootfs" {
		t.Fatalf("RootDiskName = %q, want %q", defaultVMReferenceNamingProfile.RootDiskName, "rootfs")
	}
	if defaultVMReferenceNamingProfile.DataDiskName != "datadisk" {
		t.Fatalf("DataDiskName = %q, want %q", defaultVMReferenceNamingProfile.DataDiskName, "datadisk")
	}
	if defaultVMReferenceNamingProfile.CloudInitDiskName != "cloudinitdisk" {
		t.Fatalf("CloudInitDiskName = %q, want %q", defaultVMReferenceNamingProfile.CloudInitDiskName, "cloudinitdisk")
	}
	if defaultVMReferenceNamingProfile.PrimaryNetworkName != "default" {
		t.Fatalf("PrimaryNetworkName = %q, want %q", defaultVMReferenceNamingProfile.PrimaryNetworkName, "default")
	}
	if defaultVMReferenceNamingProfile.RootDataVolumeSuffix != "-rootfs" {
		t.Fatalf("RootDataVolumeSuffix = %q, want %q", defaultVMReferenceNamingProfile.RootDataVolumeSuffix, "-rootfs")
	}
}

func TestDefaultRootDataVolumeName(t *testing.T) {
	if got := DefaultRootDataVolumeName("vm-a"); got != "vm-a-rootfs" {
		t.Fatalf("DefaultRootDataVolumeName(vm-a) = %q, want %q", got, "vm-a-rootfs")
	}
	if got := DefaultRootDataVolumeName("   "); got != "" {
		t.Fatalf("DefaultRootDataVolumeName(blank) = %q, want empty", got)
	}
}
