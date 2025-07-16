package driver

import (
	"context"
	"fmt"
	"io/ioutil"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc/status"
)

// Global configuration variables
var (
	NisdConfigPath  = "/etc/niova/NisdConfig.yaml"
	ublkQueueSize   = "128"
	ublkBlockSize   = "1048576"
	ublkBinaryPath  = "/usr/local/bin/niova-ublk"
	ldLibraryPath   = "/usr/local/lib"
	workingDir      = "/var/niova"
)

type ublkInfo struct {
	UblkUUID string
	UblkSize int64
	UblkPath string
}

type nisdInfo struct {
	NisdUUID       string               `yaml:"nisd_uuid"`
	RemainingSpace int64                `yaml:"remaining_space"`
	UblkMap        map[string]*ublkInfo `yaml:"-"`
}

type NisdConfig struct {
	Nisds []nisdInfo `yaml:"nisds"`
}

type NodeServer struct {
	csi.UnimplementedNodeServer
}

func LoadNisdConfig() (map[string]*nisdInfo, error) {
        data, err := ioutil.ReadFile(NisdConfigPath)
        if err != nil {
                return nil, fmt.Errorf("failed to read config file: %v", err)
        }

        var cfg NisdConfig
        if err := yaml.Unmarshal(data, &cfg); err != nil {
                return nil, fmt.Errorf("failed to parse config file: %v", err)
        }
        nisdMap := make(map[string]*nisdInfo)
        for _, nisd := range cfg.Nisds {
                // Initialize the UblkMap if needed
                if nisd.UblkMap == nil {
                        nisd.UblkMap = make(map[string]*ublkInfo)
                }
                nisdCopy := nisd // avoid referencing loop var
                nisdMap[nisd.NisdUUID] = &nisdCopy
        }

        return nisdMap, nil
}

func lsblkDevices() ([]string, error) {
        out, err := exec.Command("lsblk", "-n", "-o", "NAME,MOUNTPOINT").Output()
        if err != nil {
                return nil, err
        }
        if err != nil {
                return nil, err
        }
        fmt.Println(" output of lsblk: ", out)
        lines := strings.Split(string(out), "\n")
        var devices []string
        for _, line := range lines {
                fields := strings.Fields(line)
                if len(fields) == 0 {
                        continue
                }

                name := fields[0]
                mountpoint := ""
                if len(fields) > 1 {
                        mountpoint = fields[1]
                }

                if strings.HasPrefix(name, "ublkb") && mountpoint == "" {
                        fmt.Println(" only spec devices: /dev/%s", name)
                        devices = append(devices, "/dev/"+name)
                }
        }
        return devices, nil
}

func SaveNisdConfig(nisdMap map[string]*nisdInfo) error {
        config := struct {
                NISDs []nisdInfo `yaml:"nisds"`
        }{}

        for _, nisd := range nisdMap {
                config.NISDs = append(config.NISDs, *nisd)
        }

        data, err := yaml.Marshal(&config)
        if err != nil {
                return fmt.Errorf("failed to marshal updated config: %w", err)
        }

        if err := os.WriteFile(NisdConfigPath, data, 0644); err != nil {
                return fmt.Errorf("failed to write config to %s: %w", NisdConfigPath, err)
        }
        return nil
}

func (n *NodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
        volumeID := req.GetVolumeId()
        stagingPath := req.GetStagingTargetPath()
        volCtx := req.GetVolumeContext()

        fmt.Printf("NodeStageVolume called for volumeID=%s, stagingPath=%s\n", volumeID, stagingPath)
    
        requiredSizeStr := volCtx["requestedSize"]
	    if requiredSizeStr == "" {
		    return nil, status.Errorf(codes.InvalidArgument, "missing requiredBytes in VolumeContext")
    	}
	    requiredSize, err := strconv.ParseInt(requiredSizeStr, 10, 64)
    	if err != nil || requiredSize <= 0 {
	    	return nil, status.Errorf(codes.InvalidArgument, "invalid requiredBytes: %s", requiredSizeStr)
    	}

	    nisdMap, err := LoadNisdConfig()
    	if err != nil {
	    	return nil, fmt.Errorf("failed to reload config: %v", err)
    	}

	    var selectedNisd *nisdInfo
    	for _, nisd := range nisdMap {
	    	if nisd.RemainingSpace >= requiredSize {
		    	selectedNisd = nisd
			    break
    		}
	    }
    	if selectedNisd == nil {
	    	return nil, status.Errorf(codes.ResourceExhausted, "no NISD has enough space")
    	}

	    beforeDevices, err := lsblkDevices()
    	if err != nil {
	    	return nil, status.Errorf(codes.Internal, "failed to list devices before start: %v", err)
    	}

	    cmd := exec.Command(ublkBinaryPath,
		    "-s", fmt.Sprintf("%d", requiredSize),
    		"-t", selectedNisd.NisdUUID,
	    	"-v", volumeID,
    		"-u", volumeID,
    		"-q", ublkQueueSize,
	    	"-b", ublkBlockSize,
    	)
	    cmd.Env = append(cmd.Env, fmt.Sprintf("LD_LIBRARY_PATH=%s", ldLibraryPath))
    	cmd.Dir = workingDir
	    if err := cmd.Start(); err != nil {
		    return nil, status.Errorf(codes.Internal, "failed to start ublk: %v", err)
    	}
        fmt.Println("cmd value is:", cmd)
        fmt.Printf("ublk process started with PID %d\n", cmd.Process.Pid)

	    var ublkPath string
    	existing := make(map[string]bool)
    	for _, dev := range beforeDevices {
	    	existing[dev] = true
    	}
        fmt.Println(" previous lsblk devices: ", beforeDevices)
        fmt.Println(" existing map: ", existing)

	    for {
		    time.Sleep(1 * time.Second)
    		afterDevices, err := lsblkDevices()
	    	if err != nil {
		    	return nil, fmt.Errorf("failed to list devices after start: %v", err)
    		}
	    	for _, dev := range afterDevices {
		    	if !existing[dev] {
			    	ublkPath = dev
				    break
    			}   
	    	}
            fmt.Println(" after ublk lsblk: ", afterDevices)
            fmt.Println(" any new device created: ", ublkPath)
		    if ublkPath != "" {
    			break
	    	}
    	}
	    if ublkPath == "" {
		    return nil, status.Errorf(codes.DeadlineExceeded, "timeout waiting for ublk device to appear")
        }

        // Create staging directory
        if _, err := os.Stat(stagingPath); os.IsNotExist(err) {
            if err := os.MkdirAll(stagingPath, 0750); err != nil {
                return nil, status.Errorf(codes.Internal, "failed to create staging path: %v", err)
            }
        }

        // Format if not already formatted
        blkid := exec.Command("blkid", ublkPath)
        if err := blkid.Run(); err != nil {
            fmt.Printf("No filesystem found. Creating ext4 on %s\n", ublkPath)
            mkfs := exec.Command("mkfs.ext4", ublkPath)
            if err := mkfs.Run(); err != nil {
                return nil, status.Errorf(codes.Internal, "mkfs.ext4 failed: %v", err)
            }
        }

        // Mount the device to the staging path
        mountCmd := exec.Command("mount", ublkPath, stagingPath)
        if err := mountCmd.Run(); err != nil {
            return nil, status.Errorf(codes.Internal, "mount failed: %v", err)
        }

        fmt.Printf("Staged device %s to %s\n", ublkPath, stagingPath)
        return &csi.NodeStageVolumeResponse{}, nil
}

func (n *NodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
    	volumeID := req.GetVolumeId()
        stagingPath := req.GetStagingTargetPath()
        fmt.Printf("NodeUnstageVolume called for volumeID=%s, stagingPath=%s\n", volumeID, stagingPath)
         // Unmount the staging path
        umountCmd := exec.Command("umount", stagingPath)
        if err := umountCmd.Run(); err != nil {
            return nil, status.Errorf(codes.Internal, "failed to unmount staging path: %v", err)
        }

        fmt.Printf("Successfully unmounted staging path: %s\n", stagingPath)
        return &csi.NodeUnstageVolumeResponse{}, nil
}

func (n *NodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	    fmt.Printf("NodePublishVolume called for volume ID:%s\n", req.GetVolumeId())
    	volumeID := req.GetVolumeId()
	    targetPath := req.GetTargetPath()
    	stagingPath := req.GetStagingTargetPath()
	    fmt.Printf("NodePublishVolume: volumeID=%s, stagingPath=%s, targetPath=%s\n", volumeID, stagingPath, targetPath)

        // Create the target path if not exists
        if _, err := os.Stat(targetPath); os.IsNotExist(err) {
        	if err := os.MkdirAll(targetPath, 0750); err != nil {
	            return nil, status.Errorf(codes.Internal, "failed to create target path: %v", err)
        	}
    	}
    
        // Bind mount from staging path to target
        mountCmd := exec.Command("mount", "--bind", stagingPath, targetPath)
        if err := mountCmd.Run(); err != nil {
	        return nil, status.Errorf(codes.Internal, "bind mount failed: %v", err)
	    }

        fmt.Printf("Bind mounted %s to %s\n", stagingPath, targetPath)
    	return &csi.NodePublishVolumeResponse{}, nil
}

func (n *NodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
        fmt.Println("NodeUnpublishVolume called for volume ID:", req.GetVolumeId())
        targetPath := req.GetTargetPath()
        // Unmount the bind mount
        umountCmd := exec.Command("umount", targetPath)
        if err := umountCmd.Run(); err != nil {
            return nil, status.Errorf(codes.Internal, "failed to unmount bind mount: %v", err)
        }

        fmt.Printf("Successfully unmounted bind mount: %s\n", targetPath)

        // Optionally cleanup directory
        if err := os.RemoveAll(targetPath); err != nil {
            fmt.Printf("Warning: failed to remove target path %s: %v\n", targetPath, err)
        }
        return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (n *NodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
    // Replace with actual node ID if needed
        return &csi.NodeGetInfoResponse{
            NodeId: "niova-node",
        }, nil
}

func (n *NodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
        return &csi.NodeGetCapabilitiesResponse{
            Capabilities: []*csi.NodeServiceCapability{
                {
                    Type: &csi.NodeServiceCapability_Rpc{
                        Rpc: &csi.NodeServiceCapability_RPC{
                            Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
                        },
                    },
                },
            },
        }, nil
}


