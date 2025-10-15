Niova CSI Docker Quick Start

This repository contains Docker images for the Niova CSI Controller and Node.

1. Prerequisites

Docker >= 20.10

Kubernetes cluster (optional)

Niova CSI binaries: niova-block-csi-controller and niova-block-csi-node

2. Build Docker Image

Use the build-time argument BINARY to select the binary.

# Controller
docker build --build-arg BINARY=controller -t niova-csi-controller .

# Node
docker build --build-arg BINARY=node -t niova-csi-node .

3. Run Locally

Pass arguments to the binary directly:

# Run controller
docker run --rm niova-csi-controller \
  -driver-name=csi.niova.com \
  -version=0.1.0 \
  -endpoint=unix:///csi/custom.sock

# Run node
docker run --rm niova-csi-node \
  -driver-name=csi.niova.com \
  -version=0.1.0 \
  -endpoint=unix:///csi/custom.sock

4. Kubernetes Deployment

Set the binary using BINARY env and pass args:

containers:
  - name: niova-csi
    image: niova-csi-controller:latest
    env:
      - name: BINARY
        value: "controller"   # Use "node" for node binary
    args:
      - "-driver-name=csi.niova.com"
      - "-version=0.1.0"
      - "-endpoint=unix:///csi/custom.sock"


Apply:

kubectl apply -f niova-csi-deployment.yaml

5. Update Binaries

Replace updated binary in opt/niova/.

Rebuild with the appropriate BINARY:

docker build --build-arg BINARY=controller -t niova-csi-controller .


Notes:

ENTRYPOINT runs the selected binary automatically.

Runtime arguments (args: or CLI) are forwarded to the binary.

No separate entrypoint script required.
