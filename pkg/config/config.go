package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	cpClient "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	ctlplfl "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/lib"
	userClient "github.com/00pauln00/niova-mdsvc/controlplane/user/client"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
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
		Controller:   &types.Controller{},
	}
}

func StartAuthClient(raftuuid, raftconfig string) (*userClient.Client, func()) {
	cfg := userClient.Config{
		AppUUID:          uuid.New().String(),
		RaftUUID:         raftuuid,
		GossipConfigPath: raftconfig,
	}

	c, tearDown := userClient.New(cfg)
	return c, tearDown
}

func (cm *ConfigManager) InitNiovaClient(c *cpClient.CliCFuncs, u *userClient.Client) error {
	if cm == nil {
		return fmt.Errorf("ConfigManager is nil")
	}
	if c == nil {
		return fmt.Errorf("CP client Cannot be Nil")
	}
	if u == nil {
		return fmt.Errorf("User client Cannot be Nil")
	}
	if cm.Controller == nil {
		cm.Controller = &types.Controller{}
	}
	cm.Controller.Cpclient = c
	cm.Controller.UserClient = u
	return nil
}

func (cm *ConfigManager) UserLogin() error {
	klog.Infof("Loging in the user with env-var %s  value %s is applied from environment", types.NiovaUserName, os.Getenv(types.NiovaUserName))
	klog.Infof("Loging in the user with env-var %s value %s is applied from environment", types.NiovaUserSecret, os.Getenv(types.NiovaUserSecret))
	token, err := cm.Controller.UserClient.Login(os.Getenv(types.NiovaUserName), os.Getenv(types.NiovaUserSecret))
	if err != nil {
		klog.Errorf("Failed to Login admin user", err)
		return err
	}
	cm.Controller.Usertoken = token.AccessToken
	cm.Controller.Cpclient.SetToken(token.AccessToken)
	return nil
}

func (cm *ConfigManager) VerifyTokenExpiryAndReLogin(exp error) error {
	if errors.Is(exp, jwt.ErrTokenExpired) || strings.Contains(exp.Error(), "token is expired") {
		err := cm.UserLogin()
		if err != nil {
			return err
		}
	} else {
		klog.Errorf("Token Verification failed with different error: %v", exp)
		return exp
	}
	return nil
}

func (cm *ConfigManager) GetController() *types.Controller {
	cm.Mutex.Lock()
	defer cm.Mutex.Unlock()
	return cm.Controller
}

func parseFDType(s string) ctlplfl.FD {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pdu":
		return ctlplfl.FD_PDU
	case "rack":
		return ctlplfl.FD_RACK
	case "hv", "hypervisor":
		return ctlplfl.FD_HV
	case "device":
		return ctlplfl.FD_DEVICE
	case "partition":
		return ctlplfl.FD_PARTITION
	default:
		return ctlplfl.FD_ANY
	}
}

func (cm *ConfigManager) RetryAuth(fn func() error) error {
	for i := 0; i < types.MAX_RETRY; i++ {
		err := fn()
		if err == nil {
			return nil
		}

		if exp := cm.VerifyTokenExpiryAndReLogin(err); exp != nil {
			return fmt.Errorf("failed to relogin: %w", err)
		}
	}

	return fmt.Errorf("operation failed after %d retries", types.MAX_RETRY)
}

func (cm *ConfigManager) AllocVdev(requiredSize int64, filter, entityID, pfsId string) (string, error) {
	cm.Mutex.RLock()
	defer cm.Mutex.RUnlock()
	klog.Infof("Allocate vdev with failure domain: %s", entityID)
	// TODO: NumReplica should be passed from PVC file.
	Vdev := &ctlplfl.VdevReq{
		Vdev: &ctlplfl.VdevConfig{
			Size:       requiredSize,
			DataBlkCnt: 1,
			PFSID:      pfsId,
		},
		Filter: ctlplfl.Filter{
			ID:   entityID,
			Type: parseFDType(filter),
		},
	}
	klog.Infof("Create vdev of size", Vdev.Vdev.Size)
	var resp *ctlplfl.ResponseXML
	err := cm.RetryAuth(func() error {
		var err error
		resp, err = cm.Controller.Cpclient.CreateVdev(Vdev)
		return err
	})

	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (cm *ConfigManager) RemoveVolume(volumeID string) (string, error) {
	Req := &ctlplfl.DeleteVdevReq{
		ID: volumeID,
	}
	klog.Infof("Delete Vdev ID:", volumeID)
	// check if token expired
	var resp *ctlplfl.ResponseXML
	err := cm.RetryAuth(func() error {
		var err error
		resp, err = cm.Controller.Cpclient.DeleteVdev(Req)
		return err
	})

	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (cm *ConfigManager) GetVolume(volumeID string) (ctlplfl.VdevConfig, error) {
	vdevreq := &ctlplfl.GetReq{
		ID: volumeID,
	}
	var vdevcfg ctlplfl.VdevConfig
	err := cm.RetryAuth(func() error {
		var err error
		vdevcfg, err = cm.Controller.Cpclient.GetVdevConfig(vdevreq)
		return err
	})
	if err != nil {
		return ctlplfl.VdevConfig{}, err
	}
	return vdevcfg, nil
}

func (cm *ConfigManager) ListVolumes() ([]ctlplfl.VdevConfig, error) {
	Req := &ctlplfl.GetReq{
		GetAll: true,
	}
	var vdevcfgs []ctlplfl.VdevConfig
	err := cm.RetryAuth(func() error {
		var err error
		vdevcfgs, err = cm.Controller.Cpclient.GetVdevConfigs(Req)
		return err
	})
	if err != nil {
		return []ctlplfl.VdevConfig{}, err
	}
	return vdevcfgs, nil
}
