
package config

import (
	"fmt"
	"sync"
	"time"

	cpClient "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	"github.com/niova-block-csi/pkg/types"
	"k8s.io/klog/v2"
)

type ConfigManager struct {
	NisdConfigPath     string
	Controller         *types.Controller
	Mutex              sync.RWMutex
}

func NewConfigManager(nisdConfigPath string) *ConfigManager {
	return &ConfigManager{
		NisdConfigPath:     nisdConfigPath,
		Controller: &types.Controller{
			NisdMap: make(map[string]*types.Nisd),
		},
	}
}

func (cm *ConfigManager) LoadCpClient(c *cpClient.CliCFuncs) error {
	cm.Controller.NisdMap = make(map[string]*types.Nisd)
	cm.Controller.Cpclient = c
	return nil
}

func (cm *ConfigManager) LoadConfig() error {

	cm.Mutex.Lock()
	defer cm.Mutex.Unlock()
	/*	cm.controller.NisdMap = make(map[string]*types.Nisd)

		data, err := ioutil.ReadFile(cm.nisdConfigPath)
		if err != nil {
			return fmt.Errorf("failed to read NISD config file: %v", err)
		}

		var config types.NisdConfig
		if err := yaml.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("failed to parse NISD config: %v", err)
		}

		for _, nisdInfo := range config.Nisds {
			nisd := &types.Nisd{
				Info:   *nisdInfo,
				VolMap: make(map[string]*types.Volume),
			}
			cm.controller.NisdMap[nisdInfo.UUID.String()] = nisd
		}*/

	return nil
}

func (cm *ConfigManager) GetController() *types.Controller {
	cm.Mutex.Lock()
	defer cm.Mutex.Unlock()
	return cm.Controller
}

func (cm *ConfigManager) FindNisdWithSpace(requiredSize int64) (string, error) {
	cm.Mutex.RLock()
	defer cm.Mutex.RUnlock()

	Vdev := ctlplfl.Vdev{
		Cfg: ctlplfl.VdevCfg {
			Size: requiredSize,
			NumReplica: 1,
		},
	}
	klog.Infof("calling create vdev request with size", Vdev.Cfg.Size)
	resp, err := cm.Controller.Cpclient.CreateVdev(&Vdev)
	if err != nil {
		klog.Infof("nisd is not allocated", err)
		return "", fmt.Errorf("failed to get nisd %w", err)
	}
	klog.Infof("reponse from control plane :%+v", resp.ID)

	return resp.ID, nil
}

func (cm *ConfigManager) UpdateNisdAvailableSizeLocked(nisdUUID string, sizeChange int64) error {

	/*	if cm.configSource == types.SrcNISD {
		nisd, exists := cm.controller.NisdMap[nisdUUID]
		if !exists {
			return fmt.Errorf("NISD with UUID %s not found", nisdUUID)
		}

		nisd.Info.AvailableSize += sizeChange
		if nisd.Info.AvailableSize < 0 {
			nisd.Info.AvailableSize = 0
		}
	}*/
	return nil
}

func (cm *ConfigManager) AddVolumeLocked(volume *types.Volume) error {
	vdev, exists := cm.Controller.NisdMap[volume.VolID]
	if !exists {
		return fmt.Errorf("vdev %s not initialized", volume.VolID)
	}
	vdev.VolMap[volume.VolID] = volume
	return nil
}


func (cm *ConfigManager) RemoveVolume(volumeID string) error {

	for _, nisd := range cm.Controller.NisdMap {
		if vol, exists := nisd.VolMap[volumeID]; exists {
			delete(nisd.VolMap, volumeID)
			cm.UpdateNisdAvailableSizeLocked(volumeID, vol.Size)
			return nil
		}
	}
	return fmt.Errorf("volume %s not found", volumeID)
}

func (cm *ConfigManager) GetVolume(volumeID string) (*types.Volume, error) {
	nisd, exists := cm.Controller.NisdMap[volumeID]
	if !exists {
		return nil, fmt.Errorf("volume %s not found", volumeID)
	}
	vol, exists := nisd.VolMap[volumeID]
	if !exists {
		return nil, fmt.Errorf("volume %s not found", volumeID)
	}
	return vol, nil
}

func (cm *ConfigManager) UpdateVolumeStatus(volumeID string, status types.VolumeStatus, nodeName string) error {

	for _, nisd := range cm.Controller.NisdMap {
		if vol, exists := nisd.VolMap[volumeID]; exists {
			vol.Status = status
			vol.NodeName = nodeName

			now := time.Now()
			if status == types.VolumeStatusAttached {
				vol.AttachedAt = &now
			} else if status == types.VolumeStatusDetached {
				vol.AttachedAt = nil
			}

			return nil
		}
	}
	return fmt.Errorf("volume %s not found", volumeID)
}

