#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$SCRIPT_DIR/dist"

mkdir -p "$DIST_DIR"

if ! command -v go >/dev/null 2>&1; then
  cat >&2 <<'EOF'
未找到 go 命令。

处理方式：
  sudo apt update
  sudo apt install -y golang-go

安装后重新执行：
  bash exp7/build-images.sh
EOF
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  cat >&2 <<'EOF'
未找到 docker 命令。

请确认 Docker Desktop 已启动，并且 Settings -> Resources -> WSL Integration 中已开启 Ubuntu。
EOF
  exit 1
fi

pushd "$ROOT_DIR/exp7/game-app" >/dev/null
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$DIST_DIR/game" ./cmd/server/game
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$DIST_DIR/loadgen" ./cmd/loadgen
popd >/dev/null

pushd "$ROOT_DIR" >/dev/null
docker build -f exp7/Dockerfile.prebuilt --build-arg SERVICE=game -t exp7-game:v1 .
docker build -f exp7/Dockerfile.prebuilt --build-arg SERVICE=loadgen -t exp7-loadgen:v1 .
popd >/dev/null
