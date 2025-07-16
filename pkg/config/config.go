package config

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/niova-block-csi/pkg/types"
	"gopkg.in/yaml.v2"
	"k8s.io/klog/v2"
)

type ConfigManager struct {
	nisdConfigPath     string
	volumeTrackingPath string
	controller         *types.Controller
	mutex              sync.RWMutex
}

func NewConfigManager(nisdConfigPath, volumeTrackingPath string) *ConfigManager {
	return &ConfigManager{
		nisdConfigPath:     nisdConfigPath,
		volumeTrackingPath: volumeTrackingPath,
		controller: &types.Controller{
			NisdMap: make(map[string]*types.Nisd),
		},
	}
}

func (cm *ConfigManager) LoadNisdConfig() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	data, err := ioutil.ReadFile(cm.nisdConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read NISD config file: %v", err)
	}

	var config types.NisdConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse NISD config: %v", err)
	}

	cm.controller.NisdMap = make(map[string]*types.Nisd)
	for _, nisdInfo := range config.Nisds {
		// Validate device path exists
		if _, err := os.Stat(nisdInfo.DevicePath); err != nil {
			klog.Warningf("Device path %s for NISD %s does not exist or is not accessible: %v",
				nisdInfo.DevicePath, nisdInfo.UUID.String(), err)
		}

		nisd := &types.Nisd{
			Info:   *nisdInfo,
			VolMap: make(map[string]*types.Volume),
		}
		cm.controller.NisdMap[nisdInfo.UUID.String()] = nisd
	}

	klog.Infof("Loaded %d NISD backends from config", len(config.Nisds))
	return nil
}

func (cm *ConfigManager) GetController() *types.Controller {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.controller
}

func (cm *ConfigManager) FindNisdWithSpace(requiredSize int64) (*types.Nisd, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	for _, nisd := range cm.controller.NisdMap {
		if nisd.Info.AvailableSize >= requiredSize {
			return nisd, nil
		}
	}
	return nil, fmt.Errorf("no NISD with sufficient space (%d bytes) found", requiredSize)
}

func (cm *ConfigManager) UpdateNisdAvailableSize(nisdUUID string, sizeChange int64) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	nisd, exists := cm.controller.NisdMap[nisdUUID]
	if !exists {
		return fmt.Errorf("NISD with UUID %s not found", nisdUUID)
	}

	nisd.Info.AvailableSize += sizeChange
	if nisd.Info.AvailableSize < 0 {
		nisd.Info.AvailableSize = 0
	}

	return nil
}

func (cm *ConfigManager) AddVolume(volume *types.Volume) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	nisd, exists := cm.controller.NisdMap[volume.NisdInfo.UUID.String()]
	if !exists {
		return fmt.Errorf("NISD with UUID %s not found", volume.NisdInfo.UUID.String())
	}

	nisd.VolMap[volume.VolID.String()] = volume
	return cm.saveVolumeTracking()
}

func (cm *ConfigManager) RemoveVolume(volumeID string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for _, nisd := range cm.controller.NisdMap {
		if vol, exists := nisd.VolMap[volumeID]; exists {
			delete(nisd.VolMap, volumeID)
			cm.UpdateNisdAvailableSize(vol.NisdInfo.UUID.String(), vol.Size)
			return cm.saveVolumeTracking()
		}
	}
	return fmt.Errorf("volume %s not found", volumeID)
}

func (cm *ConfigManager) GetVolume(volumeID string) (*types.Volume, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	for _, nisd := range cm.controller.NisdMap {
		if vol, exists := nisd.VolMap[volumeID]; exists {
			return vol, nil
		}
	}
	return nil, fmt.Errorf("volume %s not found", volumeID)
}

func (cm *ConfigManager) UpdateVolumeStatus(volumeID string, status types.VolumeStatus, nodeName string) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	for _, nisd := range cm.controller.NisdMap {
		if vol, exists := nisd.VolMap[volumeID]; exists {
			vol.Status = status
			vol.NodeName = nodeName

			now := time.Now()
			if status == types.VolumeStatusAttached {
				vol.AttachedAt = &now
			} else if status == types.VolumeStatusDetached {
				vol.AttachedAt = nil
			}

			return cm.saveVolumeTracking()
		}
	}
	return fmt.Errorf("volume %s not found", volumeID)
}

func (cm *ConfigManager) saveVolumeTracking() error {
	var allVolumes []*types.Volume

	for _, nisd := range cm.controller.NisdMap {
		for _, volume := range nisd.VolMap {
			allVolumes = append(allVolumes, volume)
		}
	}

	trackingFile := &types.VolumeTrackingFile{
		Volumes: allVolumes,
	}

	data, err := yaml.Marshal(trackingFile)
	if err != nil {
		return fmt.Errorf("failed to marshal volume tracking data: %v", err)
	}

	if err := os.MkdirAll(filepath.Dir(cm.volumeTrackingPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for volume tracking file: %v", err)
	}

	if err := ioutil.WriteFile(cm.volumeTrackingPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write volume tracking file: %v", err)
	}

	return nil
}

func (cm *ConfigManager) LoadVolumeTracking() error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if _, err := os.Stat(cm.volumeTrackingPath); os.IsNotExist(err) {
		klog.Info("Volume tracking file does not exist, starting fresh")
		return nil
	}

	data, err := ioutil.ReadFile(cm.volumeTrackingPath)
	if err != nil {
		return fmt.Errorf("failed to read volume tracking file: %v", err)
	}

	var trackingFile types.VolumeTrackingFile
	if err := yaml.Unmarshal(data, &trackingFile); err != nil {
		return fmt.Errorf("failed to parse volume tracking file: %v", err)
	}

	for _, volume := range trackingFile.Volumes {
		if nisd, exists := cm.controller.NisdMap[volume.NisdInfo.UUID.String()]; exists {
			nisd.VolMap[volume.VolID.String()] = volume
		}
	}

	klog.Infof("Loaded %d volumes from tracking file", len(trackingFile.Volumes))
	return nil
}
