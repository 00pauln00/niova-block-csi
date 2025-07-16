package driver

const (
	DriverName    = "niova-block-csi"
	DriverVersion = "v1.0.0"

	DefaultNisdConfigPath     = "/var/lib/niova-csi/nisd-config.yml"
	DefaultVolumeTrackingPath = "/var/lib/niova-csi/volumes.yml"

	DefaultNisdPort = 8080

	DefaultStagingPath = "/var/lib/kubelet/plugins/kubernetes.io/csi/pv"
	DefaultTargetPath  = "/var/lib/kubelet/pods"

	VolumeCapabilityModeBlock      = "Block"
	VolumeCapabilityModeFilesystem = "Filesystem"

	VolumeCapabilityAccessSingleNodeWriter = "SingleNodeWriter"
	VolumeCapabilityAccessSingleNodeReader = "SingleNodeReader"
	VolumeCapabilityAccessMultiNodeReader  = "MultiNodeReader"
	VolumeCapabilityAccessMultiNodeWriter  = "MultiNodeWriter"

	DefaultVolumeSize = 1024 * 1024 * 1024 // 1GB

	UblkDevicePrefix = "/dev/ublk"
)
