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

func (um *UblkManager) CreateUblkDevice(volumeID, nisdIPAddr string, nisdPort int, devicePath, volumesize string, nisdUUID string) (string, int, error) {
	klog.Infof("Creating ublk device for volume %s using NISD %s:%d, device %s",
		volumeID, nisdIPAddr, nisdPort, devicePath)

	beforeublkDevices, err := lsblkDevices()
	if err != nil {
		return "", -1, status.Errorf(codes.Internal, "failed to list devices before start: %v", err)
	}

	nisdtPath := prepareTargetPath(nisdUUID, nisdIPAddr, nisdPort)

	// Command to create niova-ublk device
	// Format: niova-ublk -v <ublk_id> -u <ublk_id> -t tcp:<nisd_uuid>:<nisd_ip>:<nisd_port> -q <queuedepth> -b <bufsize>

	cmd := exec.Command(um.ublkBinary,
		"-s", volumesize,
		"-t", nisdtPath,
		"-v", volumeID,
		"-u", volumeID,
		"-q", QUEUEDEPTH,
		"-b", MAXBUFSIZE,
	)
	cmd.Env = append(cmd.Env,
		fmt.Sprintf("LD_LIBRARY_PATH=%s", ldLibraryPath),
		fmt.Sprintf("NIOVA_BLOCK_TCP_PEER_PORT=%d", nisdPort),
	)
	cmd.Dir = workingDir
	if err := cmd.Start(); err != nil {
		return "", -1, status.Errorf(codes.Internal, "failed to start ublk: %v", err)
	}

	klog.Infof("Executing command: %s", cmd.String())

	ublkDevicePath, err := waitForDevice(beforeublkDevices)
	if err != nil {
		return "", -1, err
	}

	klog.Infof("Successfully created ublk device %s for volume %s", ublkDevicePath, volumeID)
	return ublkDevicePath, cmd.Process.Pid, nil
}

func (um *UblkManager) DeleteUblkDevice(volumeID, ublkDevicePath string, ublkPid int) error {
	klog.Infof("Deleting ublk device %s for volume %s", ublkDevicePath, volumeID)

	err := killByNameIfExists(ublkPid)
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

func (um *UblkManager) IsUblkDeviceActive(ublkDevicePath string) bool {
	if _, err := os.Stat(ublkDevicePath); err != nil {
		return false
	}
	return true
}

func (um *UblkManager) generateUblkID(volumeID string) string {
	// Generate a shorter ID from volume UUID for ublk device
	// Take first 8 characters of volume ID
	if len(volumeID) >= 8 {
		return volumeID[:8]
	}
	return volumeID
}

func (um *UblkManager) extractUblkID(ublkDevicePath string) string {
	// Extract ublk ID from device path like /dev/ublk123 -> 123
	base := filepath.Base(ublkDevicePath)
	if strings.HasPrefix(base, "ublk") {
		return strings.TrimPrefix(base, "ublkb")
	}
	return ""
}

func waitForDevice(beforeublkdevices []string) (string, error) {
	var ublkPath string
	existing := make(map[string]bool)
	for _, dev := range beforeublkdevices {
		existing[dev] = true
	}
	for {
		time.Sleep(1 * time.Second)
		afterDevices, err := lsblkDevices()
		if err != nil {
			return "", fmt.Errorf("failed to list devices after start: %v", err)
		}
		for _, dev := range afterDevices {
			if !existing[dev] {
				ublkPath = dev
				break
			}
		}
		if ublkPath != "" {
			break
		}
	}
	if ublkPath == "" {
		return "", status.Errorf(codes.DeadlineExceeded, "timeout waiting for ublk device to appear")
	}
	return ublkPath, nil
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

func lsblkDevices() ([]string, error) {
	out, err := exec.Command("lsblk", "-n", "-o", "NAME,MOUNTPOINT").Output()
	if err != nil {
		return nil, err
	}
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
			devices = append(devices, "/dev/"+name)
		}
	}
	return devices, nil
}

func killByNameIfExists(pid int) error {
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
	killCmd := exec.Command("kill", "-9", strconv.Itoa(pid))
	if err := killCmd.Run(); err != nil {
		return fmt.Errorf("failed to kill process %d: %v", pid, err)
	}

	klog.Infof("Successfully killed process %d.", pid)
	return nil
}
