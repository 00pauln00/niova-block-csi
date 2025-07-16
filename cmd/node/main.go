package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/niova-block-csi/pkg/driver"
	"k8s.io/klog/v2"
)

var (
	endpoint   = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/niova-block-csi/csi.sock", "CSI endpoint")
	nodeID     = flag.String("node-id", "", "Node ID")
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

	klog.Infof("Starting CSI node plugin for driver %s version %s", *driverName, *version)
	klog.Infof("Node ID: %s", *nodeID)
	klog.Infof("Endpoint: %s", *endpoint)

	// Create CSI driver
	csiDriver := driver.NewCSIDriver(*driverName, *version, *nodeID, *endpoint, nil)

	// Setup node server
	csiDriver.SetupNodeServer()

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

	klog.Info("CSI node plugin stopped")
}
