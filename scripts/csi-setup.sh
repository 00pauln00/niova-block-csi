g!/bin/bash

set -euo pipefail

WORKDIR=$1
CSI_DIR=$2
DOCKER_REPO=$3
VERSION_TAG=$4

CP_BIN=${WORKDIR}/cp-bin
NB_BIN=${WORKDIR}/nb-bin
CSI_BIN=${WORKDIR}/csi-bin

mkdir -p \
  "${NB_BIN}" \

echo "====================================="
echo "Installing dependencies"
echo "====================================="
sudo apt-get update

sudo apt-get install -y \
  git \
  curl \
  wget \
  conntrack \
  build-essential \
  autoconf \
  automake \
  libtool \
  pkg-config \
  uuid-dev \
  libaio-dev \
  liburing-dev
echo "Installing docker"
sudo systemctl enable docker
sudo systemctl start docker
echo "Adding user to docker group"
sudo usermod -aG docker $USER
echo "====================================="
echo "Installing kubectl"
echo "====================================="

curl -LO \
"https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"

chmod +x kubectl

sudo mv kubectl /usr/local/bin/

echo "====================================="
echo "Installing Minikube"
echo "====================================="

sudo curl -LO \
https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64

sudo install \
minikube-linux-amd64 \
/usr/local/bin/minikube

echo "====================================="
echo "Starting Minikube"
echo "====================================="

minikube start \
  --driver=docker \
  --cpus=4 \
  --memory=3900mb

kubectl get nodes

echo "build niova-block"
echo "====================================="
: << 'COMMENT'
cd "$NBLOCK_DIR"
cd niova-core

./prepare.sh && ./configure --prefix="${NB_BIN}" --enable-devel && make clean && make -j$(nproc) && sudo make install

cd ../modules/liburing

./configure && make -j$(nproc) && sudo make install

cd ../ubdsrv

autoreconf -i && ./configure --prefix="${NB_BIN}" && make -j$(nproc) && sudo make install

cd ../..

./prepare.sh && ./configure --with-niova="${NB_BIN}" --prefix="${NB_BIN}" && make -j$(nproc) && sudo make install
COMMENT

echo "====================================="
echo "Creating test disk"
echo "====================================="

cd "${WORKDIR}"

dd if=/dev/zero \
  of=disk.img \
  bs=1M \
  count=10240

sudo losetup -fP disk.img

lsblk

Cp -rf "$CSI_DIR"/niova-mdsvc/scripts/deploy/* "${CP_BIN}"
cd "${CP_BIN}"
ls

export GOSSIP_NODE_IP=$(ip route get 1.1.1.1 | awk '{print $7; exit}')
export GOSSIP_START_PORT=10110
export GOSSIP_END_PORT=10160
export PEER_PORT=20000
export CLIENT_PORT=20105

export OUTPUT_DIR=${CP_BIN}/ctlplane/
export BIN_DIR=${CP_BIN}/libexec/niova/
export LIB_DIR=${CP_BIN}/lib/
envsubst < $CSI_DIR/scripts/config.yaml.tpl > config.yaml

./deploy.sh -m init config.yaml

echo "====================================="
echo "Build CSI images"
echo "====================================="

cd "$CSI_DIR/docker"

sudo ./build.sh "${CSI_BIN}" "${NB_BIN}" "${DOCKER_REPO}" "${VERSION_TAG}"

echo "====================================="
echo "Load images into Minikube"
echo "====================================="

minikube image load \
  ${DOCKER_REPO}:controller-${VERSION_TAG}

minikube image load \
  ${DOCKER_REPO}:node-${VERSION_TAG}

echo "====================================="
echo "Deploy CSI"
echo "====================================="

#cd "${WORKDIR}/deploy"

#kubectl apply -f niova-rbac.yaml

#kubectl apply -f niova-csi-driver.yaml

#kubectl apply -f niova-csi-controller.yaml

#kubectl apply -f niova-csi-node.yaml

#kubectl wait \
 # --for=condition=Ready \
  #pod \
  #--all \
  #--timeout=600s

kubectl get pods -A

echo "====================================="
echo "DONE"
echo "====================================="
