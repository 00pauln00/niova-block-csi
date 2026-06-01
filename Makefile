# Makefile for niova-block-csi driver

# Variables
DRIVER_NAME = niova-block-csi
VERSION = v1.0.0
REGISTRY ?= docker.io/niova
IMAGE_TAG ?= $(VERSION)

# Architecture Detection
# Go uses 'arm64' for the architecture that uname calls 'aarch64'
UNAME_M := $(shell uname -m)
ifeq ($(UNAME_M),aarch64)
    ARCH = arm64
else ifeq ($(UNAME_M),x86_64)
    ARCH = amd64
else
    ARCH = $(shell go env GOARCH)
endif

# Go related variables
GOOS ?= linux
GOARCH ?= $(ARCH)
GOPATH ?= $(shell go env GOPATH)

# Explicitly set the C compiler to ensure CGO doesn't default to x86 toolchains
CC = gcc

# Binary names
CONTROLLER_BINARY = niova-block-csi-controller
NODE_BINARY = niova-block-csi-node

# Build directories
BUILD_DIR ?= /opt/niova
BIN_DIR = $(BUILD_DIR)

.PHONY: all build controller node clean help version

# Default target
all: build

# Build both controller and node binaries
build: controller node

# Build controller binary
controller:
	@echo "Building controller binary for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 \
	GOOS=$(GOOS) \
	GOARCH=$(GOARCH) \
	CC=$(CC) \
	go build -v -o $(BIN_DIR)/$(CONTROLLER_BINARY) ./cmd/controller

# Build node binary
node:
	@echo "Building node binary for $(GOOS)/$(GOARCH)..."
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=1 \
	GOOS=$(GOOS) \
	GOARCH=$(GOARCH) \
	CC=$(CC) \
	go build -v -o $(BIN_DIR)/$(NODE_BINARY) ./cmd/node

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)

# Show help
help:
	@echo "Available targets:"
	@echo "  build            - Build both binaries"
	@echo "  controller       - Build controller binary"
	@echo "  node             - Build node binary"
	@echo "  version          - Show detected arch and version"
	@echo "  clean            - Remove build artifacts"

# Version information
version:
	@echo "Driver:     $(DRIVER_NAME)"
	@echo "Version:    $(VERSION)"
	@echo "Host Arch:  $(UNAME_M)"
	@echo "Go Arch:    $(GOARCH)"
	@echo "Compiler:   $(CC)"
