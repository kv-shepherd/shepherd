package provider

import "strings"

// vmReferenceNamingProfile defines platform-managed reference names used to
// wire together related KubeVirt spec sections (for example disks <-> volumes
// and interfaces <-> networks).
//
// These names are compatibility-sensitive implementation details, not ordinary
// user-facing business parameters. Keep them centralized and stable. If the
// platform ever needs to migrate to a new naming scheme, introduce a versioned
// profile or an explicit migration path instead of changing these defaults
// in-place.
type vmReferenceNamingProfile struct {
	RootDiskName         string
	DataDiskName         string
	CloudInitDiskName    string
	PrimaryNetworkName   string
	RootDataVolumeSuffix string
}

var defaultVMReferenceNamingProfile = vmReferenceNamingProfile{
	RootDiskName:         "rootfs",
	DataDiskName:         "datadisk",
	CloudInitDiskName:    "cloudinitdisk",
	PrimaryNetworkName:   "default",
	RootDataVolumeSuffix: "-rootfs",
}

// DefaultRootDataVolumeName returns the platform-managed root DataVolume name
// for a VM when using the default naming profile.
func DefaultRootDataVolumeName(vmName string) string {
	name := strings.TrimSpace(vmName)
	if name == "" {
		return ""
	}
	return name + defaultVMReferenceNamingProfile.RootDataVolumeSuffix
}
