package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/niova-block-csi/pkg/config"
	"github.com/niova-block-csi/pkg/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

type ControllerServer struct {
	config *config.ConfigManager
	caps   []*csi.ControllerServiceCapability
}

func NewControllerServer(configManager *config.ConfigManager) *ControllerServer {
	return &ControllerServer{
		config: configManager,
		caps: []*csi.ControllerServiceCapability{
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
					},
				},
			},
			{
				Type: &csi.ControllerServiceCapability_Rpc{
					Rpc: &csi.ControllerServiceCapability_RPC{
						Type: csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
					},
				},
			},
		},
	}
}

func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.Infof("CreateVolume: called with args %+v", req)

	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume name cannot be empty")
	}

	if req.GetCapacityRange() == nil {
		return nil, status.Error(codes.InvalidArgument, "Capacity range cannot be empty")
	}

	volumeSize := req.GetCapacityRange().GetRequiredBytes()
	if volumeSize == 0 {
		volumeSize = 1024 * 1024 * 1024 // 1GB default
	}
	caps := req.GetVolumeCapabilities()
	if len(caps) == 0 {
		return nil, status.Error(codes.InvalidArgument, "VolumeCapabilities missing")
	}
	cap := caps[0]
	if cap.GetMount() == nil && cap.GetBlock() == nil {
		return nil, status.Error(codes.InvalidArgument, "Unsupported volume capability")
	}
	volumeMode := "mount"
	if cap.GetBlock() != nil {
		volumeMode = "block"
	}
	// Allocate Vdev of required size
	volumeID, err := cs.config.AllocVdev(volumeSize)
	if err != nil {
		klog.Errorf("Failed to Allocate Vdev with error : %v", err)
		return nil, status.Error(codes.ResourceExhausted, err.Error())
	}
	klog.Infof("Allocated vdevid is : %s", volumeID)
	// Create volume structure
	volume := &types.Volume{
		VolID:      volumeID,
		Size:       volumeSize,
		Status:     types.VolumeStatusCreated,
		VolumeMode: volumeMode,
		CreatedAt:  time.Now(),
	}
	cs.config.Mutex.Lock()
	if _, exists := cs.config.Controller.VdevMap[volumeID]; !exists {
		cs.config.Controller.VdevMap[volumeID] = &types.Vdev{
			VolMap: make(map[string]*types.Volume),
		}
	}
	// Add volume to config manager
	if err := cs.config.AddVolumeLocked(volume); err != nil {
		klog.Errorf("Failed to add volume to config: %v", err)
		cs.config.Mutex.Unlock()
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to create volume: %v", err))
	}

	cs.config.Mutex.Unlock()

	klog.Infof("Created volume %s of size %d bytes on NISD %s", volumeID, volumeSize)

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: volumeSize,
		},
	}, nil
}

func (cs *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	cs.config.Mutex.Lock()
	defer cs.config.Mutex.Unlock()
	klog.Infof("DeleteVolume: called with args %+v", req)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	volumeID := req.GetVolumeId()

	// Get volume info
	volume, err := cs.config.GetVolume(volumeID)
	if err != nil {
		klog.Warningf("Volume %s not found, considering it already deleted", volumeID)
		return &csi.DeleteVolumeResponse{}, nil
	}

	// Remove volume from config
	if err := cs.config.RemoveVolume(volumeID); err != nil {
		klog.Errorf("Failed to remove volume from config: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to delete volume: %v", err))
	}

	klog.Infof("Deleted volume %s of size %d bytes", volumeID, volume.Size)

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *ControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	klog.Infof("ControllerPublishVolume: called with args %+v", req)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	if req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Node ID cannot be empty")
	}

	volumeID := req.GetVolumeId()
	nodeID := req.GetNodeId()

	// Get volume info
	cs.config.Mutex.Lock()
	volume, err := cs.config.GetVolume(volumeID)
	if err != nil {
		klog.Errorf("Volume %s not found: %v", volumeID, err)
		cs.config.Mutex.Unlock()
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Volume %s not found", volumeID))
	}

	// Update volume status to attached
	if err := cs.config.UpdateVolumeStatus(volumeID, types.VolumeStatusAttached, nodeID); err != nil {
		klog.Errorf("Failed to update volume status: %v", err)
		cs.config.Mutex.Unlock()
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to attach volume: %v", err))
	}
	cs.config.Mutex.Unlock()
	klog.Infof("Published volume %s to node %s", volumeID, nodeID)

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{
			"volumeID":   volume.VolID,
			"volumeSize": fmt.Sprintf("%d", volume.Size),
		},
	}, nil
}

func (cs *ControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	klog.Infof("ControllerUnpublishVolume: called with args %+v", req)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	volumeID := req.GetVolumeId()
	cs.config.Mutex.Lock()
	// Check if volume exists
	_, err := cs.config.GetVolume(volumeID)
	if err != nil {
		klog.Warningf("Volume %s not found, considering it already detached", volumeID)
		cs.config.Mutex.Unlock()
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	// Update volume status to detached
	if err := cs.config.UpdateVolumeStatus(volumeID, types.VolumeStatusDetached, ""); err != nil {
		klog.Errorf("Failed to update volume status: %v", err)
		cs.config.Mutex.Unlock()
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to detach volume: %v", err))
	}
	cs.config.Mutex.Unlock()

	klog.Infof("Unpublished volume %s", volumeID)

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (cs *ControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	klog.Infof("ValidateVolumeCapabilities: called with args %+v", req)

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "Volume ID cannot be empty")
	}

	if req.GetVolumeCapabilities() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities cannot be empty")
	}

	// Check if volume exists
	cs.config.Mutex.Lock()
	vol, err := cs.config.GetVolume(req.GetVolumeId())
	if err != nil {
		cs.config.Mutex.Unlock()
		return nil, status.Error(codes.NotFound, fmt.Sprintf("Volume %s not found", req.GetVolumeId()))
	}
	cs.config.Mutex.Unlock()
	for _, cap := range req.GetVolumeCapabilities() {
		if cap.GetBlock() != nil && vol.VolumeMode != "block" {
			return &csi.ValidateVolumeCapabilitiesResponse{}, nil
		}
		if cap.GetMount() != nil && vol.VolumeMode != "mount" {
			return &csi.ValidateVolumeCapabilitiesResponse{}, nil
		}
	}
	// For now, we support all requested capabilities
	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}, nil
}

func (cs *ControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	klog.Infof("ListVolumes: called with args %+v", req)

	var entries []*csi.ListVolumesResponse_Entry

	controller := cs.config.GetController()
	for _, vdev := range controller.VdevMap {
		for _, volume := range vdev.VolMap {
			entries = append(entries, &csi.ListVolumesResponse_Entry{
				Volume: &csi.Volume{
					VolumeId:      volume.VolID,
					CapacityBytes: volume.Size,
					VolumeContext: map[string]string{
						"status":   string(volume.Status),
						"nodeName": volume.NodeName,
					},
				},
			})
		}
	}

	return &csi.ListVolumesResponse{
		Entries: entries,
	}, nil
}

func (cs *ControllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	klog.Infof("GetCapacity: called with args %+v", req)

	var totalCapacity int64
	return &csi.GetCapacityResponse{
		AvailableCapacity: totalCapacity,
	}, nil
}

func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: cs.caps,
	}, nil
}

func (cs *ControllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CreateSnapshot is not implemented")
}

func (cs *ControllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "DeleteSnapshot is not implemented")
}

func (cs *ControllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ListSnapshots is not implemented")
}

func (cs *ControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerExpandVolume is not implemented")
}

func (cs *ControllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "ControllerGetVolume is not implemented")
}
