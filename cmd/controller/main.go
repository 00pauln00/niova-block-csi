package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/niova-block-csi/pkg/config"
	"github.com/niova-block-csi/pkg/driver"
	"k8s.io/klog/v2"
)

var (
	endpoint           = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/niova-block-csi/csi.sock", "CSI endpoint")
	nodeID             = flag.String("node-id", "", "Node ID")
	nisdConfigPath     = flag.String("nisd-config", "/var/lib/niova-csi/nisd-config.yml", "Path to NISD configuration file")
	volumeTrackingPath = flag.String("volume-tracking", "/var/lib/niova-csi/volumes.yml", "Path to volume tracking file")
	driverName         = flag.String("driver-name", "niova-block-csi", "Name of the CSI driver")
	version            = flag.String("version", "v1.0.0", "Version of the CSI driver")
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
	klog.Infof("NISD config path: %s", *nisdConfigPath)
	klog.Infof("Volume tracking path: %s", *volumeTrackingPath)

	// Create config manager
	configManager := config.NewConfigManager(*nisdConfigPath, *volumeTrackingPath)

	// Load NISD configuration
	if err := configManager.LoadNisdConfig(); err != nil {
		klog.Fatalf("Failed to load NISD configuration: %v", err)
	}

	// Load existing volume tracking
	if err := configManager.LoadVolumeTracking(); err != nil {
		klog.Fatalf("Failed to load volume tracking: %v", err)
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

	klog.Info("CSI controller stopped")
}
