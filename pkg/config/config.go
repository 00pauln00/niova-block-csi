package config

import (
	"fmt"
	"sync"

	cpClient "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	"github.com/niova-block-csi/pkg/types"
	"k8s.io/klog/v2"
)

type ConfigManager struct {
	CpConfigPath string
	Controller   *types.Controller
	Mutex        sync.RWMutex
}

func NewConfigManager(cpConfigPath string) *ConfigManager {
	return &ConfigManager{
		CpConfigPath: cpConfigPath,
	}
}

func (cm *ConfigManager) LoadCpClient(c *cpClient.CliCFuncs) error {
	if c == nil {
		return fmt.Errorf("CP client Cannot be Nil")
	}
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
	// TODO: NumReplica should be passed from PVC file.
	Vdev := &ctlplfl.VdevReq{
		Vdev: &ctlplfl.VdevCfg{
			Size:       requiredSize,
			NumReplica: 1,
		},
	}
	klog.Infof("Create vdev of size", Vdev.Vdev.Size)
	resp, err := cm.Controller.Cpclient.CreateVdev(Vdev)
	if err != nil {
		klog.Infof("nisd is not allocated", err)
		return "", fmt.Errorf("failed to get nisd %w", err)
	}
	klog.Infof("Created Vdev of UUID :%+v", resp.ID)

	return resp.ID, nil
}
func (cm *ConfigManager) RemoveVolume(volumeID string) error {
	/*TODO: Delete the Vdev from CP*/
	return nil
}

func (cm *ConfigManager) GetVolume(volumeID string) (string, error) {
	/*TODO: Get the Volume configuration from CP*/
	return "", fmt.Errorf("Need to get details from CP")
}
