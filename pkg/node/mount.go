package node

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
)

// Supported filesystem types
const (
	FilesystemTypeExt4    = "ext4"
	FilesystemTypeExt3    = "ext3"
	FilesystemTypeXFS     = "xfs"
	FilesystemTypeUnknown = "unknown"
)

type MountManager struct {
	mounter mount.Interface
}

func NewMountManager() *MountManager {
	return &MountManager{
		mounter: mount.New(""),
	}
}

func (mm *MountManager) FormatAndMountDevice(devicePath, targetPath, requestedFsType string) error {
	klog.Infof("Processing device %s for mount to %s with requested filesystem %s", devicePath, targetPath, requestedFsType)

	// Create target directory if it doesn't exist
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create target directory %s: %v", targetPath, err)
	}

	// Validate filesystem compatibility and get the actual filesystem type to use
	if err := mm.validateFilesystemCompatibility(devicePath, requestedFsType); err != nil {
		return fmt.Errorf("filesystem compatibility check failed: %v", err)
	}

	// Get existing filesystem type (if any)
	existingFsType, err := mm.getExistingFilesystemType(devicePath)
	if err != nil {
		return fmt.Errorf("failed to detect existing filesystem: %v", err)
	}

	// Determine the filesystem type to use for mounting
	mountFsType := requestedFsType
	needsFormatting := false

	if existingFsType == "" {
		// No existing filesystem - format with requested type
		needsFormatting = true
		klog.Infof("Device %s has no filesystem, will format with %s", devicePath, requestedFsType)
	} else {
		// Existing filesystem found - use it and preserve data
		mountFsType = existingFsType
		klog.Infof("Device %s has existing %s filesystem, preserving data", devicePath, existingFsType)

		if existingFsType != requestedFsType {
			klog.Warningf("Using existing filesystem %s instead of requested %s to preserve data", existingFsType, requestedFsType)
		}
	}

	// Format device only if needed
	if needsFormatting {
		klog.Infof("Formatting device %s with filesystem %s", devicePath, requestedFsType)
		if err := mm.formatDevice(devicePath, requestedFsType); err != nil {
			return fmt.Errorf("failed to format device %s: %v", devicePath, err)
		}
	}

	// Check if already mounted
	mounted, err := mm.mounter.IsMountPoint(targetPath)
	if err != nil {
		return fmt.Errorf("failed to check if target is mounted: %v", err)
	}

	if mounted {
		klog.Infof("Device %s is already mounted at %s", devicePath, targetPath)
		return nil
	}

	// Mount the device with the appropriate filesystem type
	klog.Infof("Mounting device %s to %s with filesystem type %s", devicePath, targetPath, mountFsType)
	if err := mm.mounter.Mount(devicePath, targetPath, mountFsType, []string{}); err != nil {
		return fmt.Errorf("failed to mount device %s to %s: %v", devicePath, targetPath, err)
	}

	if needsFormatting {
		klog.Infof("Successfully formatted and mounted device %s to %s with %s filesystem", devicePath, targetPath, mountFsType)
	} else {
		klog.Infof("Successfully mounted existing %s filesystem from device %s to %s", mountFsType, devicePath, targetPath)
	}

	return nil
}

func (mm *MountManager) BindMount(sourcePath, targetPath string) error {
	klog.Infof("Bind mounting %s to %s", sourcePath, targetPath)

	// Create target directory if it doesn't exist
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return fmt.Errorf("failed to create target directory %s: %v", targetPath, err)
	}

	// Check if already mounted
	mounted, err := mm.mounter.IsMountPoint(targetPath)
	if err != nil {
		return fmt.Errorf("failed to check if target is mounted: %v", err)
	}

	if mounted {
		klog.Infof("Path %s is already mounted at %s", sourcePath, targetPath)
		return nil
	}

	// Perform bind mount
	if err := mm.mounter.Mount(sourcePath, targetPath, "", []string{"bind"}); err != nil {
		return fmt.Errorf("failed to bind mount %s to %s: %v", sourcePath, targetPath, err)
	}

	klog.Infof("Successfully bind mounted %s to %s", sourcePath, targetPath)
	return nil
}

func (mm *MountManager) Unmount(targetPath string) error {
	klog.Infof("Unmounting %s", targetPath)

	// Check if mounted
	mounted, err := mm.mounter.IsMountPoint(targetPath)
	if err != nil {
		return fmt.Errorf("failed to check if target is mounted: %v", err)
	}

	if !mounted {
		klog.Infof("Path %s is not mounted", targetPath)
		return nil
	}

	// Unmount
	if err := mm.mounter.Unmount(targetPath); err != nil {
		return fmt.Errorf("failed to unmount %s: %v", targetPath, err)
	}

	klog.Infof("Successfully unmounted %s", targetPath)
	return nil
}

func (mm *MountManager) CleanupMountPoint(targetPath string) error {
	klog.Infof("Cleaning up mount point %s", targetPath)

	// Unmount if mounted
	if err := mm.Unmount(targetPath); err != nil {
		return fmt.Errorf("failed to unmount during cleanup: %v", err)
	}

	// Remove directory if empty
	if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		klog.Warningf("Failed to remove directory %s: %v", targetPath, err)
	}

	return nil
}

// getExistingFilesystemType detects the filesystem type on a device
func (mm *MountManager) getExistingFilesystemType(devicePath string) (string, error) {
	klog.V(4).Infof("Probing filesystem type for device %s", devicePath)

	// Use blkid to detect filesystem type
	cmd := exec.Command("blkid", "-o", "value", "-s", "TYPE", devicePath)
	output, err := cmd.CombinedOutput()

	if err != nil {
		// blkid returns non-zero if no filesystem is found
		if strings.Contains(string(output), "No such device") {
			return "", fmt.Errorf("device %s not found", devicePath)
		}
		// No filesystem found - this is normal for unformatted devices
		klog.V(4).Infof("No filesystem detected on device %s", devicePath)
		return "", nil
	}

	fsType := strings.TrimSpace(string(output))
	if fsType == "" {
		klog.V(4).Infof("Empty filesystem type detected on device %s", devicePath)
		return "", nil
	}

	klog.Infof("Detected existing filesystem type %s on device %s", fsType, devicePath)
	return fsType, nil
}

// isDeviceFormatted checks if device has any filesystem
func (mm *MountManager) isDeviceFormatted(devicePath string) (bool, error) {
	fsType, err := mm.getExistingFilesystemType(devicePath)
	if err != nil {
		return false, err
	}
	return fsType != "", nil
}

// validateFilesystemCompatibility checks if existing filesystem is compatible with requested type
func (mm *MountManager) validateFilesystemCompatibility(devicePath, requestedFsType string) error {
	// Validate that requested filesystem type is supported
	if !mm.isSupportedFilesystem(requestedFsType) {
		return fmt.Errorf("unsupported filesystem type requested: %s", requestedFsType)
	}

	existingFsType, err := mm.getExistingFilesystemType(devicePath)
	if err != nil {
		return fmt.Errorf("failed to detect existing filesystem: %v", err)
	}

	// If no existing filesystem, formatting is needed
	if existingFsType == "" {
		klog.Infof("Device %s has no filesystem, will format with %s", devicePath, requestedFsType)
		return nil
	}

	// Validate that existing filesystem is supported
	if !mm.isSupportedFilesystem(existingFsType) {
		return fmt.Errorf("existing filesystem type %s on device %s is not supported", existingFsType, devicePath)
	}

	// Check if existing filesystem matches requested type
	if existingFsType == requestedFsType {
		klog.Infof("Device %s already has compatible filesystem %s, preserving data", devicePath, existingFsType)
		return nil
	}

	// Filesystem type mismatch - log warning but proceed with existing filesystem to preserve data
	klog.Warningf("Device %s has filesystem %s but %s was requested. Preserving existing data with %s filesystem",
		devicePath, existingFsType, requestedFsType, existingFsType)

	return nil
}

// isSupportedFilesystem checks if the filesystem type is supported
func (mm *MountManager) isSupportedFilesystem(fsType string) bool {
	switch fsType {
	case FilesystemTypeExt4, FilesystemTypeExt3, FilesystemTypeXFS:
		return true
	default:
		return false
	}
}

// probeFilesystemInfo returns comprehensive filesystem information
func (mm *MountManager) probeFilesystemInfo(devicePath string) (map[string]string, error) {
	klog.V(4).Infof("Probing detailed filesystem info for device %s", devicePath)

	// Use blkid to get all filesystem information
	cmd := exec.Command("blkid", "-o", "export", devicePath)
	output, err := cmd.CombinedOutput()

	if err != nil {
		if strings.Contains(string(output), "No such device") {
			return nil, fmt.Errorf("device %s not found", devicePath)
		}
		// No filesystem found
		return map[string]string{}, nil
	}

	// Parse blkid output
	info := make(map[string]string)
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			info[key] = value
		}
	}

	klog.V(4).Infof("Filesystem info for device %s: %+v", devicePath, info)
	return info, nil
}

func (mm *MountManager) formatDevice(devicePath, fsType string) error {
	klog.Infof("Formatting device %s with filesystem %s", devicePath, fsType)

	// Validate filesystem type before formatting
	if !mm.isSupportedFilesystem(fsType) {
		return fmt.Errorf("unsupported filesystem type: %s", fsType)
	}

	var cmd *exec.Cmd
	switch fsType {
	case FilesystemTypeExt4:
		cmd = exec.Command("mkfs.ext4", "-F", devicePath)
	case FilesystemTypeXFS:
		cmd = exec.Command("mkfs.xfs", "-f", devicePath)
	case FilesystemTypeExt3:
		cmd = exec.Command("mkfs.ext3", "-F", devicePath)
	default:
		return fmt.Errorf("unsupported filesystem type: %s", fsType)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to format device: %v, output: %s", err, string(output))
	}

	klog.Infof("Successfully formatted device %s with %s", devicePath, fsType)
	return nil
}

func (mm *MountManager) GetMountInfo(targetPath string) (map[string]string, error) {
	mountPoints, err := mm.mounter.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list mount points: %v", err)
	}

	// Clean target path
	targetPath = filepath.Clean(targetPath)

	for _, mp := range mountPoints {
		if filepath.Clean(mp.Path) == targetPath {
			return map[string]string{
				"device":  mp.Device,
				"path":    mp.Path,
				"type":    mp.Type,
				"options": strings.Join(mp.Opts, ","),
			}, nil
		}
	}

	return nil, fmt.Errorf("mount point %s not found", targetPath)
}

func (mm *MountManager) IsPathMounted(targetPath string) (bool, error) {
	return mm.mounter.IsMountPoint(targetPath)
}
