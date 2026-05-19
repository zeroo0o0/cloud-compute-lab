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

if [[ $# -lt 3 ]]; then
  echo "用法: $0 <gateway-image> <coordinator-image> <map-image> [tcr-username] [tcr-password]" >&2
  exit 1
fi

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
NAMESPACE="lab3-split"
GATEWAY_IMAGE="$1"
COORDINATOR_IMAGE="$2"
MAP_IMAGE="$3"
TCR_USERNAME="${4:-${TCR_USERNAME:-}}"
TCR_PASSWORD="${5:-${TCR_PASSWORD:-}}"

if ! retry 5 kubectl get namespace "$NAMESPACE" >/dev/null 2>&1; then
  retry 5 kubectl create namespace "$NAMESPACE"
fi

if [[ -n "$TCR_USERNAME" && -n "$TCR_PASSWORD" ]]; then
  retry 5 kubectl -n "$NAMESPACE" delete secret tcr-pull --ignore-not-found
  retry 5 kubectl -n "$NAMESPACE" create secret docker-registry tcr-pull \
    --docker-server=ccr.ccs.tencentyun.com \
    --docker-username="$TCR_USERNAME" \
    --docker-password="$TCR_PASSWORD"
fi

retry 5 kubectl apply -k "$ROOT_DIR/deploy/split-k8s"
retry 5 kubectl -n "$NAMESPACE" set image deployment/lab3-gateway gateway="$GATEWAY_IMAGE"
retry 5 kubectl -n "$NAMESPACE" set image deployment/lab3-coordinator coordinator="$COORDINATOR_IMAGE"
retry 5 kubectl -n "$NAMESPACE" set image deployment/lab3-map-green map="$MAP_IMAGE"
retry 5 kubectl -n "$NAMESPACE" set image deployment/lab3-map-cave map="$MAP_IMAGE"
retry 5 kubectl -n "$NAMESPACE" set image deployment/lab3-map-ruins map="$MAP_IMAGE"

retry 5 kubectl -n "$NAMESPACE" rollout status deployment/lab3-gateway --timeout=180s
retry 5 kubectl -n "$NAMESPACE" rollout status deployment/lab3-coordinator --timeout=180s
retry 5 kubectl -n "$NAMESPACE" rollout status deployment/lab3-map-green --timeout=180s
retry 5 kubectl -n "$NAMESPACE" rollout status deployment/lab3-map-cave --timeout=180s
retry 5 kubectl -n "$NAMESPACE" rollout status deployment/lab3-map-ruins --timeout=180s

retry 5 kubectl get pods -n "$NAMESPACE" -o wide
retry 5 kubectl get svc -n "$NAMESPACE"
retry 5 kubectl get pvc -n "$NAMESPACE"
