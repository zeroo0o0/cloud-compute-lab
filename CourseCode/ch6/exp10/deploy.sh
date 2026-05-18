#!/bin/bash
set -e

# === exp10 部署脚本 ===
# 用法: bash deploy.sh [ACR_PREFIX]

ACR="${1:-crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute}"
IMAGE="${ACR}/ch6-exp10-runtime:latest"
NAMESPACE="exp10"

echo "=== 1. 构建镜像 (linux/amd64) ==="
docker build --platform linux/amd64 -t ch6-exp10-runtime:latest .

echo "=== 2. 推送到 ACR ==="
docker tag ch6-exp10-runtime:latest "${IMAGE}"
docker push "${IMAGE}"

echo "=== 3. 更新 YAML 中的镜像地址 ==="
sed -i "s|image:.*ch6-exp10-runtime.*|image: ${IMAGE}|g" k8s/faas-runtime-deployment.yaml

echo "=== 4. 部署 K8s 资源 ==="
kubectl create namespace "${NAMESPACE}" 2>/dev/null || true
kubectl apply -f k8s/

echo "=== 5. 等待 Pod 就绪 ==="
kubectl -n "${NAMESPACE}" wait --for=condition=ready pod -l app=faas-runtime --timeout=60s

echo ""
echo "=== 部署完成 ==="
kubectl -n "${NAMESPACE}" get pods
kubectl -n "${NAMESPACE}" get svc

NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}' 2>/dev/null)
if [ -z "$NODE_IP" ]; then
  NODE_IP="<任意节点公网IP>"
fi
echo ""
echo "访问地址: ${NODE_IP}:30090"
echo "查看状态: curl http://${NODE_IP}:30090/stats"
echo "加载函数: curl -X POST http://${NODE_IP}:30090/load -H 'Content-Type: application/json' -d '{\"name\":\"daily_reward\",\"runtime\":\"python\",\"handler\":\"handler.py\",\"entry\":\"handler\",\"memory_mb\":128,\"timeout_s\":30}'"
echo "调用函数: curl -X POST 'http://${NODE_IP}:30090/invoke?function=daily_reward' -H 'Content-Type: application/json' -d '{\"event\":{\"player_id\":\"player_001\",\"action\":\"signin\"}}'"
