#!/usr/bin/env bash
set -euo pipefail

retry() {
  local attempts="$1"
  shift
  local i=1
  while true; do
    if "$@"; then
      return 0
    fi
    if [[ "$i" -ge "$attempts" ]]; then
      echo "命令重试 $attempts 次后仍失败: $*" >&2
      return 1
    fi
    echo "第 $i 次失败，3 秒后重试: $*" >&2
    i=$((i + 1))
    sleep 3
  done
}

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
NAMESPACE="${NAMESPACE:-lab4}"
KUSTOMIZE_DIR="${KUSTOMIZE_DIR:-$ROOT_DIR/deploy/k8s}"

GATEWAY_IMAGE="${1:-${GATEWAY_IMAGE:-lab4/gateway:local}}"
COORDINATOR_IMAGE="${2:-${COORDINATOR_IMAGE:-lab4/coordinator:local}}"
MAP_IMAGE="${3:-${MAP_IMAGE:-lab4/map:local}}"

echo "==> Apply Kubernetes manifests: $KUSTOMIZE_DIR"
retry 5 kubectl apply -k "$KUSTOMIZE_DIR"

echo "==> Set deployment images"
retry 5 kubectl -n "$NAMESPACE" set image deployment/lab4-gateway gateway="$GATEWAY_IMAGE"
retry 5 kubectl -n "$NAMESPACE" set image deployment/lab4-coordinator coordinator="$COORDINATOR_IMAGE"
retry 5 kubectl -n "$NAMESPACE" set image deployment/lab4-map-green map="$MAP_IMAGE"
retry 5 kubectl -n "$NAMESPACE" set image deployment/lab4-map-cave map="$MAP_IMAGE"
retry 5 kubectl -n "$NAMESPACE" set image deployment/lab4-map-ruins map="$MAP_IMAGE"

echo "==> Wait for rollout"
for deploy in lab4-gateway lab4-coordinator lab4-map-green lab4-map-cave lab4-map-ruins; do
  retry 5 kubectl -n "$NAMESPACE" rollout status "deployment/$deploy" --timeout=240s
done

echo
echo "==> Current status"
retry 5 kubectl get pods -n "$NAMESPACE" -o wide
retry 5 kubectl get svc -n "$NAMESPACE"
retry 5 kubectl get hpa -n "$NAMESPACE"
