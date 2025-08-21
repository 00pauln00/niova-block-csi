package main

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/klog/v2"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
)

func main() {
	conn, err := grpc.Dial(
		"unix:////var/lib/kubelet/plugins/csi.niova.com/csi.sock",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second),
	)
	if err != nil {
		fmt.Errorf("Failed to connect to CSI driver: %v", err)
	}
	defer conn.Close()

	client := csi.NewIdentityClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetPluginInfo(ctx, &csi.GetPluginInfoRequest{})
	if err != nil {
		fmt.Errorf("GetPluginInfo failed: %v", err)
	}

	klog.Infof("Plugin Name: %s", resp.GetName())
	klog.Infof("Plugin Version: %s", resp.GetVendorVersion())
}
