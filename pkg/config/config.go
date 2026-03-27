package config

import (
	"errors"
	"fmt"
	"os"
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

func NewUserClient(raftuuid, raftconfig string) (*userClient.Client, func()) {
	cfg := userClient.Config{
		AppUUID:          uuid.New().String(),
		RaftUUID:         raftuuid,
		GossipConfigPath: raftconfig,
	}

	c, tearDown := userClient.New(cfg)
	return c, tearDown
}

func (cm *ConfigManager) LoadCpClient(c *cpClient.CliCFuncs, u *userClient.Client) error {
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
	token, err := cm.Controller.UserClient.Login(os.Getenv("NIOVA_BLOCK_CP_AUTH_USERNAME"), os.Getenv("NIOVA_BLOCK_CP_AUTH_SECRET"))
	if err != nil {
		klog.Errorf("Failed to Login admin user", err)
		return err
	}
	cm.Controller.Usertoken = token.AccessToken
	return nil
}

func (cm *ConfigManager) VerifyTokenExpiryAndReLogin(exp error) error {
	if errors.Is(exp, jwt.ErrTokenExpired) {
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

func (cm *ConfigManager) AllocVdev(requiredSize int64) (string, error) {
	cm.Mutex.RLock()
	defer cm.Mutex.RUnlock()
	// TODO: NumReplica should be passed from PVC file.
	Vdev := &ctlplfl.VdevReq{
		Vdev: &ctlplfl.VdevCfg{
			Size:       requiredSize,
			NumReplica: 1,
		},
		UserToken: cm.Controller.Usertoken,
	}
	klog.Infof("Create vdev of size", Vdev.Vdev.Size)
	for i := 0; i < types.MAX_RETRY; i++ {  // max 1 retry
		resp, err := cm.Controller.Cpclient.CreateVdev(Vdev)
		if err == nil {
			klog.Infof("Created Vdev of UUID :%+v", resp.ID)
			return resp.ID, nil
		}
		if exp := cm.VerifyTokenExpiryAndReLogin(err); exp != nil {
			klog.Errorf("nisd is not allocated", err)
			return "", fmt.Errorf("failed to relogin with error %w", err)
		}
		Vdev.UserToken = cm.Controller.Usertoken
		continue
	}
	return "", fmt.Errorf("Failed to create vdev after retry")
}

func (cm *ConfigManager) RemoveVolume(volumeID string) (string, error) {
	Req := &ctlplfl.DeleteVdevReq{
		ID:        volumeID,
		UserToken: cm.Controller.Usertoken,
	}
	klog.Infof("Delete vdev of size", volumeID)
	// check if token expired
	for i := 0; i < types.MAX_RETRY; i++ {  // max 1 retry
		resp, err := cm.Controller.Cpclient.DeleteVdev(Req)
		if err == nil {
			return resp.ID, nil
		}
		if exp := cm.VerifyTokenExpiryAndReLogin(err); exp != nil {
			return "", fmt.Errorf("Failed to relogin with error %v", err)
		}
		// update token and retry
		Req.UserToken = cm.Controller.Usertoken
		continue
	}
	return "", fmt.Errorf("failed to delete vdev after retry")
}

func (cm *ConfigManager) GetVolume(volumeID string) (ctlplfl.VdevCfg, error) {
	vdevreq := &ctlplfl.GetReq{
		ID:        volumeID,
		UserToken: cm.Controller.Usertoken,
	}
	for i := 0; i < types.MAX_RETRY; i++ {
		vdevcfg, err := cm.Controller.Cpclient.GetVdevCfg(vdevreq)
		if err == nil {
			return vdevcfg, nil
		}
		if exp := cm.VerifyTokenExpiryAndReLogin(err); exp != nil {
			return ctlplfl.VdevCfg{}, fmt.Errorf("Failed to relogin with error %v", err)
		}
		vdevreq.UserToken = cm.Controller.Usertoken
		continue
	}
	return ctlplfl.VdevCfg{}, fmt.Errorf("failed to Get Volume after retry")
}

func (cm *ConfigManager) ListVolumes() ([]ctlplfl.VdevCfg, error) {
	Req := &ctlplfl.GetReq{
		GetAll:    true,
		UserToken: cm.Controller.Usertoken,
	}
	for i := 0; i < types.MAX_RETRY; i++ {
		vdevcfgs, err := cm.Controller.Cpclient.GetVdevCfgs(Req)
		if err == nil {
			return vdevcfgs, nil
		}
		if exp := cm.VerifyTokenExpiryAndReLogin(err); exp != nil {
			return []ctlplfl.VdevCfg{}, fmt.Errorf("Failed to relogin with error %v", err)
		}
		Req.UserToken = cm.Controller.Usertoken
		continue
	}
	return []ctlplfl.VdevCfg{}, fmt.Errorf("Failed to list volumes after retry")
}
