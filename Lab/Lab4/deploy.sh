#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=========================================="
echo "  Lab4: 分布式战场云原生部署"
echo "=========================================="

# Step 1: 检查 minikube 状态
echo ""
echo "=== Step 1: 检查 minikube 状态 ==="
if ! command -v minikube &> /dev/null; then
    echo "错误: minikube 未安装"
    echo "请先安装: https://minikube.sigs.k8s.io/docs/start/"
    exit 1
fi

if ! minikube status &> /dev/null; then
    echo "启动 minikube..."
    minikube start --cpus=2 --memory=2048
else
    echo "minikube 已运行"
fi

# Step 2: 切换 Docker 到 minikube
echo ""
echo "=== Step 2: 切换 Docker 到 minikube ==="
eval $(minikube docker-env)

# Step 3: 构建 Docker 镜像
echo ""
echo "=== Step 3: 构建 Docker 镜像 ==="
cd "$SCRIPT_DIR/student"
docker build -t battleworld:latest -f Dockerfile .
echo "镜像构建完成: battleworld:latest"

# Step 4: 创建 K8s 资源
echo ""
echo "=== Step 4: 创建 K8s 资源 ==="
cd "$SCRIPT_DIR"
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/pvc.yaml
kubectl apply -f k8s/deployment.yaml
kubectl apply -f k8s/service.yaml

# Step 5: 等待部署就绪
echo ""
echo "=== Step 5: 等待部署就绪 ==="
kubectl -n battleworld rollout status deployment/battleworld --timeout=60s

# Step 6: 输出访问信息
echo ""
echo "=========================================="
echo "  部署完成！"
echo "=========================================="
NODE_IP=$(minikube ip)
NODE_PORT=$(kubectl -n battleworld get svc battleworld-gateway -o jsonpath='{.spec.ports[0].nodePort}')
echo ""
echo "网关地址: ${NODE_IP}:${NODE_PORT}"
echo ""
echo "连接游戏:"
echo "  cd Lab4/student"
echo "  go run ./cmd/client"
echo "  选择 2. 连接指定网关"
echo "  IP: ${NODE_IP}"
echo "  端口: ${NODE_PORT}"
echo ""
echo "管理命令:"
echo "  cd Lab4/student"
echo "  go run ./cmd/admin 状态 ${NODE_IP}:${NODE_PORT}"
echo ""
echo "验证部署:"
echo "  bash verify.sh"
echo ""
echo "清理资源:"
echo "  bash undeploy.sh"
echo "=========================================="
