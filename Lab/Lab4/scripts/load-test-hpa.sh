#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

ADDR="${ADDR:-120.79.8.174:30910}"
CLIENTS="${CLIENTS:-300}"
SPAWN_RATE="${SPAWN_RATE:-30}"
OPS_PER_CLIENT="${OPS_PER_CLIENT:-5}"
DURATION="${DURATION:-8m}"
ACTIONS="${ACTIONS:-move,move,move,attack,boss,heal,switch}"

cd "$ROOT_DIR"

echo "压测目标: $ADDR"
echo "压测参数: clients=$CLIENTS spawn_rate=$SPAWN_RATE/s ops_per_client=$OPS_PER_CLIENT duration=$DURATION"
echo
echo "示例：ADDR=你的公网IP:30910 CLIENTS=600 OPS_PER_CLIENT=8 DURATION=10m ./scripts/load-test-hpa.sh"
echo
echo "观察 HPA 请在另一个终端运行：watch -n 0.5 'kubectl -n lab4 get pods,hpa -o wide'"
echo

GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}" go run ./cmd/gateway-loadtest \
  -addr "$ADDR" \
  -clients "$CLIENTS" \
  -spawn-rate "$SPAWN_RATE" \
  -ops-per-client "$OPS_PER_CLIENT" \
  -duration "$DURATION" \
  -actions "$ACTIONS"
