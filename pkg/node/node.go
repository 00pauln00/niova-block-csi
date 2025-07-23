package node

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"github.com/niova-block-csi/pkg/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type NodeServer struct {
	nodeID       string
	node         *types.Node
	ublkManager  *UblkManager
	mountManager *MountManager
	mutex        sync.RWMutex
	caps         []*csi.NodeServiceCapability
}

func NewNodeServer(nodeID string) *NodeServer {
	return &NodeServer{
		nodeID: nodeID,
		node: &types.Node{
			VolMap: make(map[string]*types.NodeVolume),
		},
		ublkManager:  NewUblkManager(),
		mountManager: NewMountManager(),
		caps: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
					},
				},
			},
		},
	}
}

func (ns *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	klog.Infof("NodeStageVolume: called with args %+v", req)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "Staging target path cannot be empty")
	}

	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability cannot be empty")
	}

	volumeID := req.GetVolumeId()
	stagingPath := req.GetStagingTargetPath()
	publishContext := req.GetPublishContext()

	// Extract NISD information from publish context
	nisdIPAddr := publishContext["nisdIPAddr"]
	nisdPortStr := publishContext["nisdPort"]
	devicePath := publishContext["devicePath"]
	volumeSizeStr := publishContext["volumeSize"]
	nisduuid := publishContext["nisdUUID"]

	if nisdIPAddr == "" || nisdPortStr == "" || devicePath == "" {
		return nil, status.Error(codes.InvalidArgument, "Missing required publish context information")
	}

	nisdPort, err := strconv.Atoi(nisdPortStr)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Invalid NISD port: %v", err))
	}

	_, err = strconv.ParseInt(volumeSizeStr, 10, 64)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Invalid volume size: %v", err))
	}

	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	// Check if volume already staged
	if nodeVol, exists := ns.node.VolMap[volumeID]; exists {
		if nodeVol.UblkPath != "" {
			klog.Infof("Volume %s already staged with ublk device %s", volumeID, nodeVol.UblkPath)
			return &csi.NodeStageVolumeResponse{}, nil
		}
	}

	// Create ublk device
	ublkDevicePath, ublkpid, err := ns.ublkManager.CreateUblkDevice(volumeID, nisdIPAddr, nisdPort, devicePath, volumeSizeStr, nisduuid)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to create ublk device: %v", err))
	}

	// Determine filesystem type from volume capability
	fsType := "ext4" // default
	if req.GetVolumeCapability().GetMount() != nil {
		if req.GetVolumeCapability().GetMount().GetFsType() != "" {
			fsType = req.GetVolumeCapability().GetMount().GetFsType()
		}
	}

	// Format and mount the ublk device to staging path
	if err := ns.mountManager.FormatAndMountDevice(ublkDevicePath, stagingPath, fsType); err != nil {
		// Cleanup ublk device on failure
		ns.ublkManager.DeleteUblkDevice(volumeID, ublkDevicePath, ublkpid)
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to mount device: %v", err))
	}

	// Parse volume ID to UUID
	volUUID, err := uuid.Parse(volumeID)
	if err != nil {
		klog.Errorf("Failed to parse volume ID %s: %v", volumeID, err)
		ns.ublkManager.DeleteUblkDevice(volumeID, ublkDevicePath, ublkpid)
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf("Invalid volume ID format: %v", err))
	}

	// Create or update node volume entry
	nodeVolume := &types.NodeVolume{
		VolID: volUUID,
		NisdInfo: types.NisdInfo{
			IPAddr:     nisdIPAddr,
			Port:       nisdPort,
			DevicePath: devicePath,
		},
		NodeInfo:    ns.nodeID,
		UblkPath:    ublkDevicePath,
		UblkPid:     ublkpid,
		Status:      types.VolumeStatusAttached,
		StagingPath: stagingPath,
	}

	ns.node.VolMap[volumeID] = nodeVolume

	klog.Infof("Successfully staged volume %s with ublk device %s at %s", volumeID, ublkDevicePath, stagingPath)
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	klog.Infof("NodeUnstageVolume: called with args %+v", req)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "Staging target path cannot be empty")
	}

	volumeID := req.GetVolumeId()
	stagingPath := req.GetStagingTargetPath()

	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	nodeVol, exists := ns.node.VolMap[volumeID]
	if !exists {
		klog.Warningf("Volume %s not found in node map, considering it already unstaged", volumeID)
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	// Unmount from staging path
	if err := ns.mountManager.Unmount(stagingPath); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to unmount staging path: %v", err))
	}

	// Delete ublk device if it exists
	if nodeVol.UblkPath != "" {
		if err := ns.ublkManager.DeleteUblkDevice(volumeID, nodeVol.UblkPath, nodeVol.UblkPid); err != nil {
			klog.Errorf("Failed to delete ublk device %s: %v", nodeVol.UblkPath, err)
		}
	}

	// Clean up staging path
	if err := ns.mountManager.CleanupMountPoint(stagingPath); err != nil {
		klog.Warningf("Failed to cleanup staging path %s: %v", stagingPath, err)
	}

	// Remove from node map
	delete(ns.node.VolMap, volumeID)

	klog.Infof("Successfully unstaged volume %s", volumeID)
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.Infof("NodePublishVolume: called with args %+v", req)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "Target path cannot be empty")
	}

	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "Staging target path cannot be empty")
	}

	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()
	stagingPath := req.GetStagingTargetPath()

	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	nodeVol, exists := ns.node.VolMap[volumeID]
	if !exists {
		return nil, status.Error(codes.FailedPrecondition, fmt.Sprintf("Volume %s not staged", volumeID))
	}

	// Check if volume capability is block or filesystem
	if req.GetVolumeCapability().GetBlock() != nil {
		// Block volume - bind mount the ublk device directly
		if err := ns.mountManager.BindMount(nodeVol.UblkPath, targetPath); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to bind mount block device: %v", err))
		}
	} else {
		// Filesystem volume - bind mount from staging path
		if err := ns.mountManager.BindMount(stagingPath, targetPath); err != nil {
			return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to bind mount filesystem: %v", err))
		}
	}

	// Update node volume with target path
	nodeVol.TargetPath = targetPath

	klog.Infof("Successfully published volume %s to %s", volumeID, targetPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.Infof("NodeUnpublishVolume: called with args %+v", req)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "Target path cannot be empty")
	}

	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()

	ns.mutex.Lock()
	defer ns.mutex.Unlock()

	nodeVol, exists := ns.node.VolMap[volumeID]
	if !exists {
		klog.Warningf("Volume %s not found in node map, considering it already unpublished", volumeID)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	// Unmount from target path
	if err := ns.mountManager.Unmount(targetPath); err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to unmount target path: %v", err))
	}

	// Clean up target path
	if err := ns.mountManager.CleanupMountPoint(targetPath); err != nil {
		klog.Warningf("Failed to cleanup target path %s: %v", targetPath, err)
	}

	// Clear target path from node volume
	nodeVol.TargetPath = ""

	klog.Infof("Successfully unpublished volume %s from %s", volumeID, targetPath)
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (ns *NodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	klog.Infof("NodeGetVolumeStats: called with args %+v", req)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	if req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume path cannot be empty")
	}

	volumePath := req.GetVolumePath()

	// Check if path exists
	if _, err := os.Stat(volumePath); err != nil {
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Volume path %s not found", volumePath))
	}

	// Get filesystem stats
	// For now, return basic stats - can be enhanced with actual filesystem usage
	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Unit:  csi.VolumeUsage_BYTES,
				Total: 0, // TODO: Get actual filesystem stats
				Used:  0,
			},
		},
	}, nil
}

func (ns *NodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "NodeExpandVolume is not implemented")
}

func (ns *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: ns.caps,
	}, nil
}

func (ns *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: ns.nodeID,
	}, nil
}
