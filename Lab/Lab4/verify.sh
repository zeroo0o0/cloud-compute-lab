#!/bin/bash
set -e

echo "=========================================="
echo "  Lab4 部署验证"
echo "=========================================="

# 检查 minikube
echo ""
echo "=== 1. 检查 minikube ==="
if minikube status &> /dev/null; then
    echo "minikube: 运行中"
    echo "IP: $(minikube ip)"
else
    echo "minikube: 未运行"
    exit 1
fi

# 检查命名空间
echo ""
echo "=== 2. 检查命名空间 ==="
kubectl get namespace battleworld 2>/dev/null || echo "命名空间 battleworld 不存在"

# 检查 Pod
echo ""
echo "=== 3. 检查 Pod ==="
kubectl -n battleworld get pods -o wide

# 检查 Service
echo ""
echo "=== 4. 检查 Service ==="
kubectl -n battleworld get svc

# 检查 PVC
echo ""
echo "=== 5. 检查 PVC ==="
kubectl -n battleworld get pvc

# 检查 ConfigMap
echo ""
echo "=== 6. 检查 ConfigMap ==="
kubectl -n battleworld get configmap battleworld-config -o yaml 2>/dev/null | head -20

# TCP 连通性测试
echo ""
echo "=== 7. TCP 连通性测试 ==="
NODE_IP=$(minikube ip)
NODE_PORT=$(kubectl -n battleworld get svc battleworld-gateway -o jsonpath='{.spec.ports[0].nodePort}' 2>/dev/null)

if [ -z "$NODE_PORT" ]; then
    echo "错误: 无法获取 Service 端口"
else
    echo "测试连接: ${NODE_IP}:${NODE_PORT}"
    if command -v nc &> /dev/null; then
        if nc -z -w3 "$NODE_IP" "$NODE_PORT" 2>/dev/null; then
            echo "连接成功!"
        else
            echo "连接失败"
        fi
    else
        echo "nc 命令不可用，跳过连通性测试"
    fi
fi

# Pod 日志
echo ""
echo "=== 8. Pod 日志（最后 10 行）==="
kubectl -n battleworld logs -l app=battleworld --tail=10 2>/dev/null || echo "无法获取日志"

echo ""
echo "=========================================="
echo "  验证完成"
echo "=========================================="
echo ""
echo "连接游戏:"
echo "  cd Lab4/student"
echo "  go run ./cmd/client"
echo "  选择 2. 连接指定网关"
echo "  IP: ${NODE_IP}"
echo "  端口: ${NODE_PORT}"
echo "=========================================="
