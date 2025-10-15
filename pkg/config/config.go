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
	nisdConfigPath     string
	controller         *types.Controller
	Mutex              sync.RWMutex
}

func NewConfigManager(nisdConfigPath string) *ConfigManager {
	return &ConfigManager{
		nisdConfigPath:     nisdConfigPath,
		controller: &types.Controller{
			NisdMap: make(map[string]*types.Nisd),
		},
	}
}

func (cm *ConfigManager) LoadCpClient(c *cpClient.CliCFuncs) error {
	cm.controller.NisdMap = make(map[string]*types.Nisd)
	cm.controller.Cpclient = c
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
	return cm.controller
}

func (cm *ConfigManager) FindNisdWithSpace(requiredSize int64) (*types.Nisd, string, error) {
	cm.Mutex.RLock()
	defer cm.Mutex.RUnlock()

	Vdev := ctlplfl.Vdev{
		Size: requiredSize,
	}
	klog.Infof("calling create vdev request with size", Vdev.Size)
	err := cm.controller.Cpclient.CreateVdev(&Vdev)
	if err != nil {
		klog.Infof("nisd is not allocated", err)
		return nil, "", fmt.Errorf("failed to get nisd %w", err)
	}
	klog.Infof("reponse from control plane :%+v", Vdev)

	for _, nisdChunk := range Vdev.NisdToChkMap {
		klog.Infof("allocated nisd for vdev :%+v", nisdChunk)
		var emptyNisd ctlplfl.Nisd
		if nisdChunk.Nisd == emptyNisd {
			continue
		}

		nisd := &types.Nisd{
			Info:   nisdChunk.Nisd,
			VolMap: make(map[string]*types.Volume), // initialize empty map
		}

		return nisd, Vdev.VdevID, nil
	}

	return nil, "", fmt.Errorf("no NISD found in controlplane response for size %d", requiredSize)

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
	nisd, exists := cm.controller.NisdMap[volume.NisdInfo.ID]
	if !exists {
		nisd = &types.Nisd{
			Info:   volume.NisdInfo,
			VolMap: make(map[string]*types.Volume),
		}
		cm.controller.NisdMap[volume.NisdInfo.ID] = nisd
	}
	if nisd.VolMap == nil {
		nisd.VolMap = make(map[string]*types.Volume)
	}
	nisd.VolMap[volume.VolID] = volume
	return nil
}


func (cm *ConfigManager) RemoveVolume(volumeID string) error {

	for _, nisd := range cm.controller.NisdMap {
		if vol, exists := nisd.VolMap[volumeID]; exists {
			delete(nisd.VolMap, volumeID)
			cm.UpdateNisdAvailableSizeLocked(vol.NisdInfo.ID, vol.Size)
			return nil
		}
	}
	return fmt.Errorf("volume %s not found", volumeID)
}

func (cm *ConfigManager) GetVolume(volumeID string) (*types.Volume, error) {

	for _, nisd := range cm.controller.NisdMap {
		if vol, exists := nisd.VolMap[volumeID]; exists {
			return vol, nil
		}
	}
	return nil, fmt.Errorf("volume %s not found", volumeID)
}

func (cm *ConfigManager) UpdateVolumeStatus(volumeID string, status types.VolumeStatus, nodeName string) error {

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

			return nil
		}
	}
	return fmt.Errorf("volume %s not found", volumeID)
}

