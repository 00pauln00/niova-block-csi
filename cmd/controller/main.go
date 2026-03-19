package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	cpClient "github.com/00pauln00/niova-mdsvc/controlplane/ctlplanefuncs/client"
	"github.com/google/uuid"
	"github.com/niova-block-csi/pkg/config"
	"github.com/niova-block-csi/pkg/driver"
	"k8s.io/klog/v2"
)

var (
	endpoint   = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/niova-block-csi/csi.sock", "CSI endpoint")
	raftID     = flag.String("r", "", "pass the raft uuid")
	nodeID     = flag.String("node-id", "", "Node ID")
	ConfigPath = flag.String("configpath", "./gossipNodes", "Path to gossip configuration file")
	driverName = flag.String("driver-name", "niova-block-csi", "Name of the CSI driver")
	version    = flag.String("version", "v1.0.0", "Version of the CSI driver")
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	if *nodeID == "" {
		hostname, err := os.Hostname()
		if err != nil {
			klog.Fatalf("Failed to get hostname: %v", err)
		}
		*nodeID = hostname
	}

	klog.Infof("Starting CSI controller for driver %s version %s", *driverName, *version)
	klog.Infof("Node ID: %s", *nodeID)
	klog.Infof("Endpoint: %s", *endpoint)
	klog.Infof("ControlPlane config path: %s", *ConfigPath)
	klog.Infof("Raft ID: %s", *raftID)

	// Create config manager
	configManager := config.NewConfigManager(*ConfigPath)

	c := cpClient.InitCliCFuncs(uuid.New().String(), *raftID, *ConfigPath)
	u, td := config.NewUserClient(*raftID, *ConfigPath)
	if u == nil {
		klog.Fatalf("Failed to initialize user client")
	}
	defer td()
	if err := configManager.LoadCpClient(c, u); err != nil {
		klog.Errorf("Failed to load CP configuration: %v", err)
		os.Exit(-1)
	}
	klog.Infof("connection with control plane is sucessful %v", c)

	err := configManager.UserLogin()
	if err != nil {
		klog.Fatalf("Failed to Login admin user", err)
	}

	// Create CSI driver
	csiDriver := driver.NewCSIDriver(*driverName, *version, *nodeID, *endpoint, configManager)

	// Setup controller server
	csiDriver.SetupControllerServer()

	// Setup identity server
	csiDriver.SetupIdentityServer()

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		klog.Infof("Received signal %s, shutting down", sig)
		cancel()
	}()

	// Start the driver
	if err := csiDriver.Run(ctx); err != nil {
		klog.Fatalf("Failed to run CSI driver: %v", err)
	}

	klog.Infof("CSI controller stopped")
}
