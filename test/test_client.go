package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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
		log.Fatalf("Failed to connect to CSI driver: %v", err)
	}
	defer conn.Close()

	client := csi.NewIdentityClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetPluginInfo(ctx, &csi.GetPluginInfoRequest{})
	if err != nil {
		log.Fatalf("GetPluginInfo failed: %v", err)
	}

	fmt.Printf("✅ Plugin Name: %s\n", resp.GetName())
	fmt.Printf("✅ Plugin Version: %s\n", resp.GetVendorVersion())
}

