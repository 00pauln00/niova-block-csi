package driver

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
)


type ControllerServer struct {
	csi.UnimplementedControllerServer
}


func (cs *ControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {

	// Generate a unique volume ID
	volumeID := uuid.New().String()

	requestedSize := req.GetCapacityRange().GetRequiredBytes()
	if requestedSize <= 0 {
		return nil, fmt.Errorf("invalid volume size requested")
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: requestedSize,
            VolumeContext: map[string]string{
                "requestedSize": fmt.Sprintf("%d", requestedSize),
            },
		},
	}, nil
}

func (c *ControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
    fmt.Println("🗑️  DeleteVolume called with volume ID:", req.GetVolumeId())

    return &csi.DeleteVolumeResponse{}, nil
}


func (cs *ControllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
    return &csi.ControllerGetCapabilitiesResponse{
        Capabilities: []*csi.ControllerServiceCapability{
            {
                Type: &csi.ControllerServiceCapability_Rpc{
                    Rpc: &csi.ControllerServiceCapability_RPC{
                        Type: csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
                    },
                },
            },
        },
    }, nil
}

func (cs *ControllerServer) ControllerPublishVolume(
	ctx context.Context,
	req *csi.ControllerPublishVolumeRequest,
) (*csi.ControllerPublishVolumeResponse, error) {

	fmt.Println("ControllerPublishVolume called")
	fmt.Printf("VolumeID: %s, NodeID: %s\n", req.GetVolumeId(), req.GetNodeId())

	// Optionally validate the VolumeId and NodeId
	if req.GetVolumeId() == "" || req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "VolumeId and NodeId are required")
	}

	// If you have per-node logic, you could store the attachment state in memory/disk here
	// For local block devices (ublk), likely nothing is needed

	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (cs *ControllerServer) ControllerUnpublishVolume(
	ctx context.Context,
	req *csi.ControllerUnpublishVolumeRequest,
) (*csi.ControllerUnpublishVolumeResponse, error) {

	fmt.Println("ControllerUnpublishVolume called")
	fmt.Printf("VolumeID: %s, NodeID: %s\n", req.GetVolumeId(), req.GetNodeId())

	// Optional validation
	if req.GetVolumeId() == "" || req.GetNodeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "VolumeId and NodeId are required")
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

