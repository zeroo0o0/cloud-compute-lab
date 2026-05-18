#!/bin/bash
set -e

# === Lab4 部署脚本 ===
# 用法: bash deploy.sh [phase] [ACR_PREFIX]
# 示例: bash deploy.sh              # 默认部署 Phase 1
#       bash deploy.sh phase1        # 部署 Phase 1（单体模式）
#       bash deploy.sh phase2        # 部署 Phase 2（微服务模式）

PHASE="${1:-phase1}"
ACR="${2:-crpi-074nws9q0fix3aih.cn-shenzhen.personal.cr.aliyuncs.com/hnu-cloud-compute}"
IMAGE="${ACR}/lab4-battleworld:latest"
NAMESPACE="Lab4"

echo "=== 1. 构建镜像 (linux/amd64) ==="
cd student
docker build --platform linux/amd64 -t battleworld:latest .
cd ..

echo "=== 2. 推送到 ACR ==="
docker tag battleworld:latest "${IMAGE}"
docker push "${IMAGE}"

echo "=== 3. 更新 YAML 中的镜像地址 ==="
sed -i "s|image:.*battleworld.*|image: ${IMAGE}|g" k8s/*.yaml
sed -i "s|imagePullPolicy: Never|imagePullPolicy: Always|g" k8s/*.yaml

echo "=== 4. 部署 K8s 资源 ==="
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/pvc.yaml

if [ "$PHASE" = "phase2" ]; then
  echo "--- Phase 2: 微服务模式 ---"
  kubectl -n "${NAMESPACE}" delete deployment battleworld 2>/dev/null || true
  kubectl apply -f k8s/gateway-deployment.yaml
  kubectl apply -f k8s/node-statefulset.yaml
  kubectl apply -f k8s/headless-service.yaml

  echo "=== 5. 等待 Pod 就绪 ==="
  kubectl -n "${NAMESPACE}" wait --for=condition=ready pod -l app=battleworld-gateway --timeout=120s
  kubectl -n "${NAMESPACE}" wait --for=condition=ready pod -l app=battleworld-node --timeout=120s
else
  echo "--- Phase 1: 单体模式 ---"
  kubectl apply -f k8s/deployment.yaml
  kubectl apply -f k8s/service.yaml

  echo "=== 5. 等待 Pod 就绪 ==="
  kubectl -n "${NAMESPACE}" wait --for=condition=ready pod -l app=battleworld --timeout=120s
fi

echo ""
echo "=== 部署完成 ==="
kubectl -n "${NAMESPACE}" get pods
kubectl -n "${NAMESPACE}" get svc

NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="ExternalIP")].address}' 2>/dev/null)
if [ -z "$NODE_IP" ]; then
  NODE_IP="<任意节点公网IP>"
fi
echo ""
echo "访问地址: ${NODE_IP}:30310"
echo "连接游戏: cd student && go run ./cmd/client → 选择 2 → IP: ${NODE_IP} → 端口: 30310"
echo "管理命令: cd student && go run ./cmd/admin 状态 ${NODE_IP}:30310"
