#!/bin/bash
set -e

# === exp11 部署脚本 ===
# 用法: bash deploy.sh [ACR_PREFIX]

ACR="${1:-crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute}"
IMAGE="${ACR}/ch6-exp11-game:latest"
NAMESPACE="exp11"

echo "=== 1. 构建镜像 (linux/amd64) ==="
docker build --platform linux/amd64 -t ch6-exp11-game:latest .

echo "=== 2. 推送到 ACR ==="
docker tag ch6-exp11-game:latest "${IMAGE}"
docker push "${IMAGE}"

echo "=== 3. 更新 YAML 中的镜像地址 ==="
sed -i "s|image:.*ch6-exp11-game.*|image: ${IMAGE}|g" k8s/game-deployment.yaml

echo "=== 4. 部署 K8s 资源 ==="
kubectl create namespace "${NAMESPACE}" 2>/dev/null || true
kubectl apply -f k8s/

echo "=== 5. 等待 Pod 就绪 ==="
kubectl -n "${NAMESPACE}" wait --for=condition=ready pod -l app=game-service --timeout=60s

echo ""
echo "=== 部署完成 ==="
kubectl -n "${NAMESPACE}" get pods
kubectl -n "${NAMESPACE}" get svc

NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}' 2>/dev/null)
if [ -z "$NODE_IP" ]; then
  NODE_IP="<任意节点公网IP>"
fi
echo ""
echo "访问地址: ${NODE_IP}:30081"
echo "健康检查: curl http://${NODE_IP}:30081/health"
echo "模拟死锁: curl -X POST http://${NODE_IP}:30081/deadlock"
echo "观察恢复: watch -n 1 kubectl -n ${NAMESPACE} get pods"
