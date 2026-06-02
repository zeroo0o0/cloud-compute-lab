#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DIST_DIR="$SCRIPT_DIR/dist"
REGISTRY="${REGISTRY:-10.0.2.12:5000}"
IMAGE_PREFIX="$REGISTRY/exp8"
REDIS_SOURCE_IMAGE="${REDIS_SOURCE_IMAGE:-redis:7-alpine}"

mkdir -p "$DIST_DIR"

if ! command -v go >/dev/null 2>&1; then
  cat >&2 <<'EOF'
未找到 go 命令。

处理方式：
  sudo apt update
  sudo apt install -y golang-go

安装后重新执行：
  cd exp8
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
for service in gateway game storage; do
  docker build -f Dockerfile.prebuilt --build-arg SERVICE="$service" -t "exp8-$service:v1" .
  docker tag "exp8-$service:v1" "$IMAGE_PREFIX/exp8-$service:v1"
  docker push "$IMAGE_PREFIX/exp8-$service:v1"
done

docker pull "$REDIS_SOURCE_IMAGE"
docker tag "$REDIS_SOURCE_IMAGE" "$IMAGE_PREFIX/redis:v1"
docker push "$IMAGE_PREFIX/redis:v1"
popd >/dev/null

cat <<EOF

镜像已构建并推送到：
  $IMAGE_PREFIX/exp8-gateway:v1
  $IMAGE_PREFIX/exp8-game:v1
  $IMAGE_PREFIX/exp8-storage:v1
  $IMAGE_PREFIX/redis:v1
EOF
