package driver

import (
	"fmt"
	"net"
	"os"
	"log"

	"google.golang.org/grpc"
	"github.com/container-storage-interface/spec/lib/go/csi"
)

// Driver defines the basic CSI driver structure
type Driver struct {
	Name       string
	Version    string
	NodeID     string
	Endpoint   string
	grpcServer *grpc.Server
}

// NewCSIDriver initializes a new Driver instance
func NewCSIDriver(name, version, nodeID, endpoint string) *Driver {
	return &Driver{
		Name:     name,
		Version:  version,
		NodeID:   nodeID,
		Endpoint: endpoint,
	}
}

// Run starts the gRPC server and registers the CSI services
func (d *Driver) Run() error {
	// Create new gRPC server
	d.grpcServer = grpc.NewServer()

	// Register CSI service servers
	csi.RegisterIdentityServer(d.grpcServer, &IdentityServer{Driver: d})
//	csi.RegisterControllerServer(d.grpcServer, &ControllerServer{})
	csi.RegisterNodeServer(d.grpcServer, &NodeServer{})
	// Before starting gRPC server
	if err := os.RemoveAll(d.Endpoint); err != nil {
	    log.Fatalf("failed to remove existing socket: %v", err)
	}

	// Setup socket listener
	listener, err := net.Listen("unix", d.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", d.Endpoint, err)
	}

	fmt.Printf("Starting CSI Driver: %s (version: %s) at %s\n", d.Name, d.Version, d.Endpoint)

	// Serve
	return d.grpcServer.Serve(listener)
}

