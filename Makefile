# Makefile for niova-block-csi driver

# Variables
DRIVER_NAME = niova-block-csi
VERSION = v1.0.0
REGISTRY ?= docker.io/niova
IMAGE_TAG ?= $(VERSION)

# Go related variables
GO_VERSION = 1.21
GOPATH ?= $(shell go env GOPATH)
GOOS ?= linux
GOARCH ?= amd64

# Binary names
CONTROLLER_BINARY = niova-block-csi-controller
NODE_BINARY = niova-block-csi-node

# Build directories
BUILD_DIR = build
BIN_DIR = $(BUILD_DIR)/bin
DOCKER_DIR = $(BUILD_DIR)/docker

# Docker images
CONTROLLER_IMAGE = $(REGISTRY)/$(DRIVER_NAME)-controller:$(IMAGE_TAG)
NODE_IMAGE = $(REGISTRY)/$(DRIVER_NAME)-node:$(IMAGE_TAG)

.PHONY: all build controller node test clean docker-build docker-push deploy undeploy

# Default target
all: build

# Build both controller and node binaries
build: controller node

# Build controller binary
controller:
	@echo "Building controller binary..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-o $(BIN_DIR)/$(CONTROLLER_BINARY) \
		./cmd/controller

# Build node binary
node:
	@echo "Building node binary..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build \
		-o $(BIN_DIR)/$(NODE_BINARY) \
		./cmd/node

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	docker rmi $(CONTROLLER_IMAGE) $(NODE_IMAGE) 2>/dev/null || true

# Show help
help:
	@echo "Available targets:"
	@echo "  build            - Build both controller and node binaries"
	@echo "  controller       - Build controller binary"
	@echo "  node             - Build node binary"
	@echo "  help             - Show this help message"

# Version information
version:
	@echo "Driver: $(DRIVER_NAME)"
	@echo "Version: $(VERSION)"
	@echo "Registry: $(REGISTRY)"
	@echo "Controller Image: $(CONTROLLER_IMAGE)"
	@echo "Node Image: $(NODE_IMAGE)"
