package node

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/niova-block-csi/pkg/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

var (
	QUEUEDEPTH    = "128"
	MAXBUFSIZE    = "1048576"
	ldLibraryPath = "/usr/local/lib"
	workingDir    = "/var/niova"
)

type UblkManager struct {
	ublkBinary string
}

func NewUblkManager() *UblkManager {
	return &UblkManager{
		ublkBinary: "niova-ublk", // Assuming niova-ublk is in PATH
	}
}

// CreateUblkDevice starts a niova-ublk daemon for the given volume.
// Returns (ublkDevicePath, pid, error) where ublkDevicePath is the stable
// /dev/disk/by-uuid/<volumeID> symlink created by 61-niova-ublk.rules.
func (um *UblkManager) CreateUblkDevice(volumeID, volumesize string) (string, int, error) {
	klog.Infof("Creating ublk device for volume %s", volumeID)

	args := []string{
		"-t", "cp",
		"-v", volumeID,
		"-u", volumeID,
		"-q", QUEUEDEPTH,
		"-b", MAXBUFSIZE,
		"-T",
	}

	cmd := exec.Command(um.ublkBinary, args...)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("LD_LIBRARY_PATH=%s", ldLibraryPath),
		fmt.Sprintf("NIOVA_GOSSIP_PATH=%s", os.Getenv(types.NiovaGossipPath)),
		fmt.Sprintf("NIOVA_GOSSIP_KEY=%s", os.Getenv(types.NiovaGossipKey)),
		fmt.Sprintf("NIOVA_BLOCK_CP_AUTH_USERNAME=%s", os.Getenv(types.NiovaUserName)),
		fmt.Sprintf("NIOVA_BLOCK_CP_AUTH_SECRET=%s", os.Getenv(types.NiovaUserSecret)),
		fmt.Sprintf("NIOVA_BLOCK_UBLK_UNIFIED=%s", os.Getenv(types.NiovaUblkUnified)),
		fmt.Sprintf("NIOVA_BLOCK_MDSVC_GET_CHUNKS_LIMIT=%s", os.Getenv(types.NiovaMdsvcChunkLimit)),
	)
	cmd.Dir = workingDir
	if err := cmd.Start(); err != nil {
		return "", -1, status.Errorf(codes.Internal, "failed to start ublk: %v", err)
	}

	klog.Infof("ENV variables %s: %s and %s: %s, %s: %s and %s: %s, %s: %s, %s: %s ", types.NiovaGossipPath, os.Getenv(types.NiovaGossipPath), types.NiovaGossipKey, os.Getenv(types.NiovaGossipKey), types.NiovaUserName, os.Getenv(types.NiovaUserName), types.NiovaUserSecret, os.Getenv(types.NiovaUserSecret), types.NiovaUblkUnified, os.Getenv(types.NiovaUblkUnified), types.NiovaMdsvcChunkLimit, os.Getenv(types.NiovaMdsvcChunkLimit))
	klog.Infof("Executing command: %s", cmd.String())

	ublkDevicePath, err := waitForByUUIDLink(volumeID)
	if err != nil {
		cmd.Process.Kill()
		return "", -1, err
	}

	klog.Infof("Successfully created ublk device %s for volume %s", ublkDevicePath, volumeID)
	return ublkDevicePath, cmd.Process.Pid, nil
}

func (um *UblkManager) DeleteUblkDevice(volumeID, ublkDevicePath string, ublkPid int) error {
	klog.Infof("Deleting ublk device %s for volume %s", ublkDevicePath, volumeID)

	err := killByNameIfExists(ublkPid, ublkDevicePath)
	if err != nil {
		return fmt.Errorf("failed to delete ublk device: %v", err)
	}

	klog.Infof("Successfully deleted ublk device %s for volume %s", ublkDevicePath, volumeID)
	return nil
}

func prepareTargetPath(nisdUUID, nisdIPAddr string, nisdPort int) string {
	// nisduuid := tcp:<nisd_uuid>:<nisd_ip>:<nisd_port>
	var tPath string
	if nisdIPAddr != "" {
		tPath = fmt.Sprintf("tcp:%s:%s:%d", nisdUUID, nisdIPAddr, nisdPort+1)
	} else {
		tPath = fmt.Sprintf("tcp:%s:127.0.0.1:%d", nisdUUID, nisdPort+1)
	}
	return tPath
}

func (um *UblkManager) extractUblkID(ublkDevicePath string) string {
	// Extract ublk ID from device path like /dev/ublk123 -> 123
	base := filepath.Base(ublkDevicePath)
	if strings.HasPrefix(base, "ublk") {
		return strings.TrimPrefix(base, "ublkb")
	}
	return ""
}

// waitForByUUIDLink polls for the /dev/disk/by-uuid/<volumeID> symlink that
// 61-niova-ublk.rules creates in response to niova-ublk's synthetic uevent.
// The symlink appearing is an implicit readiness signal: niova-ublk only fires
// the uevent after niova_ublk_start() completes successfully.
func waitForByUUIDLink(volumeID string) (string, error) {
	path := "/dev/disk/by-uuid/" + volumeID
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(path); err == nil {
			return path, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", status.Errorf(codes.DeadlineExceeded,
		"timed out waiting for udev symlink %s (is 61-niova-ublk.rules installed on the node?)", path)
}

func (um *UblkManager) GetUblkDeviceInfo(ublkDevicePath string) (map[string]string, error) {
	ublkID := um.extractUblkID(ublkDevicePath)
	if ublkID == "" {
		return nil, fmt.Errorf("invalid ublk device path: %s", ublkDevicePath)
	}

	// Command to get ublk device info
	// Format: niova-ublk -i <ublk_id> --info
	cmd := exec.Command(um.ublkBinary,
		"-i", ublkID,
		"--info")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get ublk device info: %v, output: %s", err, string(output))
	}

	// Parse output into key-value pairs
	info := make(map[string]string)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				info[key] = value
			}
		}
	}

	return info, nil
}

func killByNameIfExists(pid int, ublkDevicePath string) error {
	// Step 1: Check if the process exists and is accessible
	err := syscall.Kill(pid, 0)
	if err != nil {
		switch err {
		case syscall.ESRCH:
			klog.Infof("Process %d does not exist.", pid)
			return nil // Not an error — nothing to kill
		case syscall.EPERM:
			klog.Infof("Process %d exists but you don't have permission to kill it.", pid)
			return nil // Not an error — just can't kill it
		default:
			return fmt.Errorf("error checking PID %d: %v", pid, err)
		}
	}

	// Step 2: Kill the process
	klog.Infof("Killing process with PID %d...", pid)
	killCmd := exec.Command("kill", "-15", strconv.Itoa(pid))
	if err := killCmd.Run(); err != nil {
		return fmt.Errorf("failed to kill process %d: %v", pid, err)
	}
	klog.Infof("Deleteing the ublk %s", ublkDevicePath)
	base := filepath.Base(ublkDevicePath)
	id := strings.TrimPrefix(base, "ublkb")
	dublk := exec.Command("ublk", "del", "-n", id)
	if err := dublk.Run(); err != nil {
		return fmt.Errorf("failed to delete ublk %s: %v", ublkDevicePath, err)
	}
	klog.Infof("Successfully killed process %d.", pid)
	return nil
}
