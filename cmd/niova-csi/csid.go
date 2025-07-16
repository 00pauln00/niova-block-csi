package main

import (
	"flag"
	"log"

	"niova-csi/driver"
)

func main() {
	// Define CLI flags
	driverName := flag.String("d", "csi.niova.com", "Name of the CSI driver")
	version := flag.String("v", "0.1.0", "Driver version")
	nodeID := flag.String("nid", "dummy-node-id", "Node ID")
	endpoint := flag.String("ep", "/tmp/csi.sock", "CSI endpoint (unix domain socket path)")

	flag.Parse()

	// Initialize and run the CSI driver
	d := driver.NewCSIDriver(*driverName, *version, *nodeID, *endpoint)
	if err := d.Run(); err != nil {
		log.Fatalf("Driver failed: %v", err)
	}
}
