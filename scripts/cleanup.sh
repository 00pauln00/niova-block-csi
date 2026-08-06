#!/bin/bash

set -euo pipefail

WORKDIR="${1:-}"

if [[ -z "$WORKDIR" ]]; then
echo "Usage: $0 <repo-root-dir>"
exit 1
fi

echo "========================================="
echo " Niova CSI Cleanup"
echo "========================================="

echo
echo "[1/8] Deleting CSI resources from Kubernetes..."
kubectl delete -f ~/deploy/niova-csi-nod.yml --ignore-not-found=true || true
kubectl delete -f ~/deploy/niova-csi-con.yml --ignore-not-found=true || true
kubectl delete -f ~/deploy/niova-csi-driver.yaml --ignore-not-found=true || true
kubectl delete -f ~/deploy/niova-rbac.yaml --ignore-not-found=true || true

echo
echo "[2/8] Deleting all pods in default namespace..."
kubectl delete pods --all -n default --ignore-not-found=true || true

echo
echo "[3/8] Stopping and deleting Minikube..."
minikube stop || true
minikube delete --all --purge || true

echo
echo "[4/8] Removing kube config/cache..."
rm -rf ~/.kube/cache || true
rm -rf ~/.minikube || true

echo
echo "[5/8] Cleaning Docker..."

docker ps -aq | xargs -r docker rm -f

docker images -aq | xargs -r docker rmi -f

docker volume ls -q | xargs -r docker volume rm

docker network prune -f || true

docker system prune -a -f --volumes || true

echo
echo "[6/8] Removing CSI build artifacts..."

#rm -rf ${WORKDIR}/cp-bin || true
#rm -rf ${WORKDIR}/nb-bin || true
#rm -rf ${WORKDIR}/csi-bin || true

echo
echo "[7/8] Removing cloned repositories..."

#rm -rf ${WORKDIR}/niova-mdsvc || true
#rm -rf ${WORKDIR}/niova-block || true
#rm -rf ${WORKDIR}/niova-block-csi || true

echo
echo "[8/8] Removing leftover Niova runtime data..."

#sudo rm -rf /var/niova || true
#sudo rm -rf /var/lib/niova-csi || true
#sudo rm -rf /etc/niova || true
#sudo rm -rf /root/niova-block || true

echo
echo "========================================="
echo " Cleanup Complete"
echo "========================================="
