#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="$SCRIPT_DIR/dist"
REGISTRY="${REGISTRY:-10.0.2.12:5000}"
IMAGE_PREFIX="$REGISTRY/exp5"

mkdir -p "$DIST_DIR"

if ! command -v go >/dev/null 2>&1; then
  cat >&2 <<'EOF'
未找到 go 命令。

原因：当前是在 Ubuntu / WSL 里运行脚本，但 WSL 里还没有安装 Go。

处理方式：
  sudo apt update
  sudo apt install -y golang-go

安装后重新执行：
  cd exp5
  bash build-images.sh
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

pushd "$SCRIPT_DIR/game-app" >/dev/null
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$DIST_DIR/gateway" ./cmd/server/gateway
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$DIST_DIR/game" ./cmd/server/game
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$DIST_DIR/storage" ./cmd/server/storage
popd >/dev/null

pushd "$SCRIPT_DIR" >/dev/null
docker build -f Dockerfile.prebuilt --build-arg SERVICE=gateway -t exp5-gateway:v1 .
docker build -f Dockerfile.prebuilt --build-arg SERVICE=game -t exp5-game:v1 .
docker build -f Dockerfile.prebuilt --build-arg SERVICE=storage -t exp5-storage:v1 .

docker tag exp5-gateway:v1 "$IMAGE_PREFIX/exp5-gateway:v1"
docker tag exp5-game:v1 "$IMAGE_PREFIX/exp5-game:v1"
docker tag exp5-storage:v1 "$IMAGE_PREFIX/exp5-storage:v1"

docker push "$IMAGE_PREFIX/exp5-gateway:v1"
docker push "$IMAGE_PREFIX/exp5-game:v1"
docker push "$IMAGE_PREFIX/exp5-storage:v1"
popd >/dev/null

cat <<EOF

镜像已构建并推送到：
  $IMAGE_PREFIX/exp5-gateway:v1
  $IMAGE_PREFIX/exp5-game:v1
  $IMAGE_PREFIX/exp5-storage:v1
EOF
