#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_DIR="$SCRIPT_DIR/dist"
REGISTRY="${REGISTRY:-10.0.2.12:5000}"
IMAGE_PREFIX="$REGISTRY/exp6"

mkdir -p "$DIST_DIR"

if ! command -v go >/dev/null 2>&1; then
  cat >&2 <<'EOF'
未找到 go 命令。

处理方式：
  sudo apt update
  sudo apt install -y golang-go

安装后重新执行：
  bash exp6/build-images.sh
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

pushd "$ROOT_DIR/exp6/game-app" >/dev/null
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$DIST_DIR/game" ./cmd/server/game
popd >/dev/null

pushd "$ROOT_DIR" >/dev/null
docker build -f exp6/Dockerfile.prebuilt -t exp6-game:v1 .

docker tag exp6-game:v1 "$IMAGE_PREFIX/exp6-game:v1"
docker push "$IMAGE_PREFIX/exp6-game:v1"
popd >/dev/null

cat <<EOF

镜像已构建并推送到：
  $IMAGE_PREFIX/exp6-game:v1
EOF
