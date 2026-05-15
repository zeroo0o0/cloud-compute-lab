#!/bin/bash
set -e

echo "=========================================="
echo "  清理 Lab4 K8s 资源"
echo "=========================================="

echo ""
echo "删除命名空间 battleworld..."
kubectl delete namespace battleworld --ignore-not-found

echo ""
echo "所有 battleworld 资源已清理完毕。"
echo "=========================================="
