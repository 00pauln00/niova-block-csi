package driver

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/niova-block-csi/pkg/config"
	"github.com/niova-block-csi/pkg/controller"
	"github.com/niova-block-csi/pkg/node"
	"google.golang.org/grpc"
	"k8s.io/klog/v2"
)

type CSIDriver struct {
	name          string
	version       string
	nodeID        string
	endpoint      string
	configManager *config.ConfigManager

	controllerServer *controller.ControllerServer
	nodeServer       *node.NodeServer
	identityServer   *IdentityServer

	server *grpc.Server
}

func NewCSIDriver(name, version, nodeID, endpoint string, configManager *config.ConfigManager) *CSIDriver {
	return &CSIDriver{
		name:          name,
		version:       version,
		nodeID:        nodeID,
		endpoint:      endpoint,
		configManager: configManager,
	}
}

func (d *CSIDriver) SetupControllerServer() {
	klog.Info("Setting up controller server")
	d.controllerServer = controller.NewControllerServer(d.configManager)
}

func (d *CSIDriver) SetupNodeServer() {
	klog.Info("Setting up node server")
	d.nodeServer = node.NewNodeServer(d.nodeID)
}

func (d *CSIDriver) SetupIdentityServer() {
	klog.Info("Setting up identity server")
	d.identityServer = NewIdentityServer(d.name, d.version)
}

func (d *CSIDriver) Run(ctx context.Context) error {
	klog.Infof("Starting CSI driver %s version %s", d.name, d.version)

	// Setup identity server (always required)
	d.SetupIdentityServer()

	// Parse endpoint
	proto, addr, err := parseEndpoint(d.endpoint)
	if err != nil {
		return fmt.Errorf("failed to parse endpoint: %v", err)
	}

	// Remove socket file if it exists
	if proto == "unix" {
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove socket file: %v", err)
		}

		// Create directory for socket file
		if err := os.MkdirAll(filepath.Dir(addr), 0755); err != nil {
			return fmt.Errorf("failed to create socket directory: %v", err)
		}
	}

	// Create listener
	listener, err := net.Listen(proto, addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s://%s: %v", proto, addr, err)
	}

	// Create gRPC server
	d.server = grpc.NewServer()

	// Register identity server
	csi.RegisterIdentityServer(d.server, d.identityServer)

	// Register controller server if configured
	if d.controllerServer != nil {
		csi.RegisterControllerServer(d.server, d.controllerServer)
		klog.Info("Controller server registered")
	}

	// Register node server if configured
	if d.nodeServer != nil {
		csi.RegisterNodeServer(d.server, d.nodeServer)
		klog.Info("Node server registered")
	}

	klog.Infof("CSI driver listening on %s://%s", proto, addr)

	// Handle graceful shutdown
	go func() {
		<-ctx.Done()
		klog.Info("Shutting down CSI driver")
		d.server.GracefulStop()
	}()

	// Start serving
	if err := d.server.Serve(listener); err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}

	return nil
}

func parseEndpoint(endpoint string) (string, string, error) {
	parts := strings.SplitN(endpoint, "://", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid endpoint format: %s", endpoint)
	}

	proto := parts[0]
	addr := parts[1]

	switch proto {
	case "unix":
		return proto, addr, nil
	case "tcp":
		return proto, addr, nil
	default:
		return "", "", fmt.Errorf("unsupported protocol: %s", proto)
	}
}

type IdentityServer struct {
	name    string
	version string
}

func NewIdentityServer(name, version string) *IdentityServer {
	return &IdentityServer{
		name:    name,
		version: version,
	}
}

func (is *IdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{
		Name:          is.name,
		VendorVersion: is.version,
	}, nil
}

func (is *IdentityServer) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
					},
				},
			},
		},
	}, nil
}

func (is *IdentityServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{}, nil
}
