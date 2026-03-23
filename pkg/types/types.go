package types

import (
	cpClient "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	"github.com/google/uuid"
)

type VolumeStatus string

const (
	SrcCP                             = "control-plane"
	BLOCK_MODE                        = "block"
	MOUNT_MODE                        = "mount"
	VolumeStatusCreated  VolumeStatus = "created"
	VolumeStatusAttached VolumeStatus = "attached"
	VolumeStatusDetached VolumeStatus = "detached"
	VolumeStatusDeleted  VolumeStatus = "deleted"
)

type Controller struct {
	Cpclient *cpClient.CliCFuncs
}

type NodeVolume struct {
	VolID       uuid.UUID `yaml:"volumeID" json:"volumeID"`
	UblkPath    string    `yaml:"ublkPath" json:"ublkPath"`
	UblkPid     int       `yaml:"ublkPid" json:"ublkPid"`
	VolumeMode  string    `yaml:"volumeMode" json:"volumeMode"`
	StagingPath string    `yaml:"stagingPath" json:"stagingPath"`
	TargetPath  string    `yaml:"targetPath" json:"targetPath"`
}

type Node struct {
	VolMap map[string]*NodeVolume `yaml:"volMap" json:"volMap"`
}
