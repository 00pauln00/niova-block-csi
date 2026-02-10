package types

import (
	"time"

	cpClient "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	"github.com/google/uuid"
)

type VolumeStatus string

const (
	SrcCP                             = "control-plane"
	VolumeStatusCreated  VolumeStatus = "created"
	VolumeStatusAttached VolumeStatus = "attached"
	VolumeStatusDetached VolumeStatus = "detached"
	VolumeStatusDeleted  VolumeStatus = "deleted"
)

type Vdev struct {
	VolMap map[string]*Volume `yaml:"volMap" json:"volMap"`
}

type Volume struct {
	VolID      string       `yaml:"volumeID" json:"volumeID"`
	Size       int64        `yaml:"volumeSize" json:"volumeSize"`
	Path       string       `yaml:"volumePath" json:"volumePath"`
	NodeName   string       `yaml:"nodeName" json:"nodeName"`
	Status     VolumeStatus `yaml:"status" json:"status"`
	VolumeMode string       `yaml:"volumeMode" json:"volumeMode"`
	CreatedAt  time.Time    `yaml:"createdAt" json:"createdAt"`
	AttachedAt *time.Time   `yaml:"attachedAt,omitempty" json:"attachedAt,omitempty"`
}

type Controller struct {
	VdevMap  map[string]*Vdev `yaml:"nisdMap" json:"nisdMap"`
	Cpclient *cpClient.CliCFuncs
}

type NodeVolume struct {
	VolID       uuid.UUID    `yaml:"volumeID" json:"volumeID"`
	NodeInfo    string       `yaml:"nodeInfo" json:"nodeInfo"`
	UblkPath    string       `yaml:"ublkPath" json:"ublkPath"`
	UblkPid     int          `yaml:"ublkPid" json:"ublkPid"`
	Status      VolumeStatus `yaml:"status" json:"status"`
	VolumeMode  string       `yaml:"volumeMode" json:"volumeMode"`
	StagingPath string       `yaml:"stagingPath" json:"stagingPath"`
	TargetPath  string       `yaml:"targetPath" json:"targetPath"`
}

type Node struct {
	VolMap map[string]*NodeVolume `yaml:"volMap" json:"volMap"`
}

type VolumeTrackingFile struct {
	Volumes []*Volume `yaml:"volumes" json:"volumes"`
}
