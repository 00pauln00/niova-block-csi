package node

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

type UblkManager struct {
	ublkBinary string
}

func NewUblkManager() *UblkManager {
	return &UblkManager{
		ublkBinary: "niova-ublk", // Assuming niova-ublk is in PATH
	}
}

func (um *UblkManager) CreateUblkDevice(volumeID, nisdIPAddr string, nisdPort int, devicePath string) (string, error) {
	klog.Infof("Creating ublk device for volume %s using NISD %s:%d, device %s",
		volumeID, nisdIPAddr, nisdPort, devicePath)

	// Generate a unique ublk device ID based on volume ID
	ublkID := um.generateUblkID(volumeID)

	// Expected ublk device path
	ublkDevicePath := fmt.Sprintf("/dev/ublk%s", ublkID)

	// Command to create niova-ublk device
	// XXX Add the cmd for ublk
	//klog.Infof("Executing command: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to create ublk device: %v, output: %s", err, string(output))
	}

	// Wait for device to be available
	if err := um.waitForDevice(ublkDevicePath, 10*time.Second); err != nil {
		return "", fmt.Errorf("ublk device %s not available after creation: %v", ublkDevicePath, err)
	}

	klog.Infof("Successfully created ublk device %s for volume %s", ublkDevicePath, volumeID)
	return ublkDevicePath, nil
}

func (um *UblkManager) DeleteUblkDevice(volumeID, ublkDevicePath string) error {
	klog.Infof("Deleting ublk device %s for volume %s", ublkDevicePath, volumeID)

	// Extract ublk ID from device path
	ublkID := um.extractUblkID(ublkDevicePath)
	if ublkID == "" {
		return fmt.Errorf("invalid ublk device path: %s", ublkDevicePath)
	}

	// Command to delete niova-ublk device
	// Format: niova-ublk -i <ublk_id> --delete
	cmd := exec.Command(um.ublkBinary,
		"-i", ublkID,
		"--delete")

	klog.Infof("Executing command: %s", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete ublk device: %v, output: %s", err, string(output))
	}

	klog.Infof("Successfully deleted ublk device %s for volume %s", ublkDevicePath, volumeID)
	return nil
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
		return strings.TrimPrefix(base, "ublk")
	}
	return ""
}

func (um *UblkManager) waitForDevice(devicePath string, timeout time.Duration) error {
	start := time.Now()
	for time.Since(start) < timeout {
		if _, err := os.Stat(devicePath); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for device %s", devicePath)
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
