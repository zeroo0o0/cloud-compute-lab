#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

TAG="${1:-local}"
IMAGE_PREFIX="${IMAGE_PREFIX:-lab4}"
REDIS_IMAGE="${REDIS_IMAGE:-redis:7-alpine}"
INCLUDE_REDIS_IMAGE="${INCLUDE_REDIS_IMAGE:-1}"
OUT_DIR="${OUT_DIR:-$ROOT_DIR/.dist/images}"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
DOCKER_PLATFORM="${DOCKER_PLATFORM:-linux/$GOARCH}"
PUSH="${PUSH:-0}"

mkdir -p "$ROOT_DIR/.dist" "$OUT_DIR"
export GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}"

cd "$ROOT_DIR"

echo "==> Build linux binaries: GOOS=$GOOS GOARCH=$GOARCH"
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags="-s -w" -o .dist/lab4-cloud-gateway ./cmd/cloud-gateway
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags="-s -w" -o .dist/lab4-cloud-coordinator ./cmd/cloud-coordinator
CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath -ldflags="-s -w" -o .dist/lab4-cloud-map ./cmd/cloud-map

echo "==> Build docker images: prefix=$IMAGE_PREFIX tag=$TAG platform=$DOCKER_PLATFORM"
docker build --platform "$DOCKER_PLATFORM" -f Dockerfile.cloud-gateway -t "$IMAGE_PREFIX/gateway:$TAG" .
docker build --platform "$DOCKER_PLATFORM" -f Dockerfile.cloud-coordinator -t "$IMAGE_PREFIX/coordinator:$TAG" .
docker build --platform "$DOCKER_PLATFORM" -f Dockerfile.cloud-map -t "$IMAGE_PREFIX/map:$TAG" .

SAVE_IMAGES=(
  "$IMAGE_PREFIX/gateway:$TAG"
  "$IMAGE_PREFIX/coordinator:$TAG"
  "$IMAGE_PREFIX/map:$TAG"
)

if [[ "$INCLUDE_REDIS_IMAGE" == "1" ]]; then
  echo "==> Pull redis image for offline node import: $REDIS_IMAGE"
  docker pull --platform "$DOCKER_PLATFORM" "$REDIS_IMAGE"
  SAVE_IMAGES+=("$REDIS_IMAGE")
fi

if [[ "$PUSH" == "1" ]]; then
  echo "==> Push docker images"
  docker push "$IMAGE_PREFIX/gateway:$TAG"
  docker push "$IMAGE_PREFIX/coordinator:$TAG"
  docker push "$IMAGE_PREFIX/map:$TAG"
fi

TAR_FILE="$OUT_DIR/lab4-images-$TAG.tar"
echo "==> Save docker images: $TAR_FILE"
docker save "${SAVE_IMAGES[@]}" -o "$TAR_FILE"

echo
echo "Images:"
echo "  $IMAGE_PREFIX/gateway:$TAG"
echo "  $IMAGE_PREFIX/coordinator:$TAG"
echo "  $IMAGE_PREFIX/map:$TAG"
if [[ "$INCLUDE_REDIS_IMAGE" == "1" ]]; then
  echo "  $REDIS_IMAGE"
fi
echo
echo "Image tar:"
echo "  $TAR_FILE"
