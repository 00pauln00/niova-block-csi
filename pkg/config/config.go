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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type ConfigManager struct {
	CpConfigPath string
	Controller   *types.Controller
	Mutex        sync.RWMutex
	K8sClient    *kubernetes.Clientset
}

func NewConfigManager(cpConfigPath string) *ConfigManager {
	return &ConfigManager{
		CpConfigPath: cpConfigPath,
		Controller:   &types.Controller{},
	}
}

func NewNiovaController() (*kubernetes.Clientset, error) {
    // Use in-cluster config — works automatically when running inside Kubernetes
    config, err := rest.InClusterConfig()
    if err != nil {
        return nil, fmt.Errorf("failed to load in-cluster config: %v", err)
    }

    // Create the clientset
    clientset, err := kubernetes.NewForConfig(config)
    if err != nil {
        return nil, fmt.Errorf("failed to create Kubernetes clientset: %v", err)
    }

    return clientset, nil
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
	klog.Infof("Loging in the user with env variables %s and %s", os.Getenv("NIOVA_BLOCK_CP_AUTH_USERNAME"), os.Getenv("NIOVA_BLOCK_CP_AUTH_SECRET"))
	token, err := cm.Controller.UserClient.Login(os.Getenv("NIOVA_BLOCK_CP_AUTH_USERNAME"), os.Getenv("NIOVA_BLOCK_CP_AUTH_SECRET"))
	klog.Infof("returned values are: %v and %v", token, err)
	if err != nil {
		klog.Errorf("Failed to Login admin user", err)
		return err
	}
	klog.Infof("userlogin done")
	cm.Controller.Usertoken = token.AccessToken
	cm.Controller.Cpclient.SetToken(token.AccessToken)
	klog.Infof("login token is set")
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

func toFD(v int) (ctlplfl.FD, error) {
    fd := ctlplfl.FD(v)

    switch fd {
    case ctlplfl.FD_ANY,
        ctlplfl.FD_PDU,
        ctlplfl.FD_RACK,
        ctlplfl.FD_HV,
        ctlplfl.FD_DEVICE,
        ctlplfl.FD_PARTITION:
        return fd, nil
    default:
        return ctlplfl.FD_ANY, fmt.Errorf("invalid ctlplfl.FD value: %d", v)
    }
}

func (cm *ConfigManager) AllocVdev(requiredSize int64, filter int, entityID string) (string, error) {
	cm.Mutex.RLock()
	defer cm.Mutex.RUnlock()
	fd, err := toFD(filter)
	if err != nil {
		return "", err
	}
	klog.Infof("fd filters are %d and %s", filter, entityID)
	// TODO: NumReplica should be passed from PVC file.
	Vdev := &ctlplfl.VdevReq{
		Vdev: &ctlplfl.VdevCfg{
			Size:       requiredSize,
			NumReplica: 1,
		},
		Filter: ctlplfl.Filter{
			ID: entityID,
                        Type: fd,
                },
	}
	klog.Infof("Create vdev of size", Vdev.Vdev.Size)
	klog.Infof("vdevreq is %v", Vdev)
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
		continue
	}
	return "", fmt.Errorf("Failed to create vdev after retry")
}

func (cm *ConfigManager) RemoveVolume(volumeID string) (string, error) {
	Req := &ctlplfl.DeleteVdevReq{
		ID:        volumeID,
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
		continue
	}
	return "", fmt.Errorf("failed to delete vdev after retry")
}

func (cm *ConfigManager) GetVolume(volumeID string) (ctlplfl.VdevCfg, error) {
	vdevreq := &ctlplfl.GetReq{
		ID:        volumeID,
	}
	for i := 0; i < types.MAX_RETRY; i++ {
		vdevcfg, err := cm.Controller.Cpclient.GetVdevCfg(vdevreq)
		if err == nil {
			return vdevcfg, nil
		}
		if exp := cm.VerifyTokenExpiryAndReLogin(err); exp != nil {
			return ctlplfl.VdevCfg{}, fmt.Errorf("Failed to relogin with error %v", err)
		}
		continue
	}
	return ctlplfl.VdevCfg{}, fmt.Errorf("failed to Get Volume after retry")
}

func (cm *ConfigManager) ListVolumes() ([]ctlplfl.VdevCfg, error) {
	Req := &ctlplfl.GetReq{
		GetAll:    true,
	}
	for i := 0; i < types.MAX_RETRY; i++ {
		vdevcfgs, err := cm.Controller.Cpclient.GetVdevCfgs(Req)
		if err == nil {
			return vdevcfgs, nil
		}
		if exp := cm.VerifyTokenExpiryAndReLogin(err); exp != nil {
			return []ctlplfl.VdevCfg{}, fmt.Errorf("Failed to relogin with error %v", err)
		}
		continue
	}
	return []ctlplfl.VdevCfg{}, fmt.Errorf("Failed to list volumes after retry")
}
