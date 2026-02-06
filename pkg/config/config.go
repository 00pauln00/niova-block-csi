
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
	CpConfigPath     string
	Controller         *types.Controller
	Mutex              sync.RWMutex
}

func NewConfigManager(cpConfigPath string) *ConfigManager {
	return &ConfigManager{
		CpConfigPath:     cpConfigPath,
		Controller: &types.Controller{
			VdevMap: make(map[string]*types.Vdev),
		},
	}
}

func (cm *ConfigManager) LoadCpClient(c *cpClient.CliCFuncs) error {
	cm.Controller.VdevMap = make(map[string]*types.Vdev)
	cm.Controller.Cpclient = c
	return nil
}


func (cm *ConfigManager) GetController() *types.Controller {
	cm.Mutex.Lock()
	defer cm.Mutex.Unlock()
	return cm.Controller
}

func (cm *ConfigManager) AllocVdev(requiredSize int64) (string, error) {
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


func (cm *ConfigManager) AddVolumeLocked(volume *types.Volume) error {
	vdev, exists := cm.Controller.VdevMap[volume.VolID]
	if !exists {
		return fmt.Errorf("vdev %s not initialized", volume.VolID)
	}
	vdev.VolMap[volume.VolID] = volume
	return nil
}


func (cm *ConfigManager) RemoveVolume(volumeID string) error {

	for _, vdev := range cm.Controller.VdevMap {
		if _, exists := vdev.VolMap[volumeID]; exists {
			delete(vdev.VolMap, volumeID)
			return nil
		}
	}
	return fmt.Errorf("volume %s not found", volumeID)
}

func (cm *ConfigManager) GetVolume(volumeID string) (*types.Volume, error) {
	vdev, exists := cm.Controller.VdevMap[volumeID]
	if !exists {
		return nil, fmt.Errorf("volume %s not found", volumeID)
	}
	vol, exists := vdev.VolMap[volumeID]
	if !exists {
		return nil, fmt.Errorf("volume %s not found", volumeID)
	}
	return vol, nil
}

func (cm *ConfigManager) UpdateVolumeStatus(volumeID string, status types.VolumeStatus, nodeName string) error {

	for _, nisd := range cm.Controller.VdevMap {
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

