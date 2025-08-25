package types

import (
	cpClient "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	"github.com/google/uuid"
	"time"
)

type VolumeStatus string

const (
	VolumeStatusCreated  VolumeStatus = "created"
	VolumeStatusAttached VolumeStatus = "attached"
	VolumeStatusDetached VolumeStatus = "detached"
	VolumeStatusDeleted  VolumeStatus = "deleted"
)

type NisdInfo struct {
	UUID   uuid.UUID `yaml:"uuid" json:"uuid"`
	IPAddr string    `yaml:"ipaddr" json:"ipaddr"`
	Port   int       `yaml:"port" json:"port"`
	// DevicePath    string    `yaml:"devicePath" json:"devicePath"`
	// TotalSize     int64     `yaml:"totalSize" json:"totalSize"`
	// AvailableSize int64     `yaml:"availableSize" json:"availableSize"`
}

type VolumeReq struct {
	VolumeName string
	VolumeSize int64
}

type Nisd struct {
	Info   NisdInfo           `yaml:"info" json:"info"`
	VolMap map[string]*Volume `yaml:"volMap" json:"volMap"`
}

type Volume struct {
	VolID      uuid.UUID    `yaml:"volumeID" json:"volumeID"`
	NisdInfo   NisdInfo     `yaml:"nisdInfo" json:"nisdInfo"`
	Size       int64        `yaml:"volumeSize" json:"volumeSize"`
	Path       string       `yaml:"volumePath" json:"volumePath"`
	NodeName   string       `yaml:"nodeName" json:"nodeName"`
	Status     VolumeStatus `yaml:"status" json:"status"`
	CreatedAt  time.Time    `yaml:"createdAt" json:"createdAt"`
	AttachedAt *time.Time   `yaml:"attachedAt,omitempty" json:"attachedAt,omitempty"`
}

type Controller struct {
	NisdMap  map[string]*Nisd `yaml:"nisdMap" json:"nisdMap"`
	cpclient *cpClient.CliCFuncs
}

type NodeVolume struct {
	VolID       uuid.UUID    `yaml:"volumeID" json:"volumeID"`
	NisdInfo    NisdInfo     `yaml:"nisdInfo" json:"nisdInfo"`
	NodeInfo    string       `yaml:"nodeInfo" json:"nodeInfo"`
	UblkPath    string       `yaml:"ublkPath" json:"ublkPath"`
	UblkPid     int          `yaml:"ublkPid" json:"ublkPid"`
	Status      VolumeStatus `yaml:"status" json:"status"`
	StagingPath string       `yaml:"stagingPath" json:"stagingPath"`
	TargetPath  string       `yaml:"targetPath" json:"targetPath"`
}

type Node struct {
	VolMap map[string]*NodeVolume `yaml:"volMap" json:"volMap"`
}

type VolumeTrackingFile struct {
	Volumes []*Volume `yaml:"volumes" json:"volumes"`
}

type NisdConfig struct {
	Nisds []*NisdInfo `yaml:"nisds" json:"nisds"`
}
