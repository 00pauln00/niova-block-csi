#!/bin/bash
set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$DIR"

echo "Preparing build context in $DIR/.build_context..."
rm -rf .build_context
mkdir -p .build_context/bin
mkdir -p .build_context/usr/local/bin .build_context/usr/local/lib .build_context/usr/local/include .build_context/usr/local/libexec

# Copy niova-block-csi tools from the system installation directory
if [ -f /opt/niova/bin/niova-block-csi-controller ]; then
    cp /opt/niova/bin/niova-block-csi-controller .build_context/bin/
else
    echo "Error: /opt/niova/bin/niova-block-csi-controller not found. Please run 'make build' first."
    exit 1
fi

if [ -f /opt/niova/bin/niova-block-csi-node ]; then
    cp /opt/niova/bin/niova-block-csi-node .build_context/bin/
else
    echo "Error: /opt/niova/bin/niova-block-csi-node not found. Please run 'make build' first."
    exit 1
fi

# Copy dependencies from system
echo "Copying dependencies from /usr/local..."
cp -R /usr/local/bin/* .build_context/usr/local/bin/ 2>/dev/null || true
cp -R /usr/local/lib/* .build_context/usr/local/lib/ 2>/dev/null || true
cp -R /usr/local/include/* .build_context/usr/local/include/ 2>/dev/null || true
cp -R /usr/local/libexec/* .build_context/usr/local/libexec/ 2>/dev/null || true

echo "Building Docker image for controller..."
docker build -f Dockerfile --build-arg BINARY=controller -t niova-csi-controller .build_context

echo "Building Docker image for node..."
docker build -f Dockerfile --build-arg BINARY=node -t niova-csi-node .build_context

echo "Docker build completed successfully."
