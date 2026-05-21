#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  ./scripts/import-images-to-nodes.sh <image-tar> <node-ip-1> [node-ip-2 ...]

Environment variables:
  REMOTE_USER=root        SSH user for Kubernetes nodes
  REMOTE_DIR=/root        Directory used to upload the image tar
  RUNTIME=containerd      containerd | docker | auto
  JUMP_HOST=              Optional public jump host, for example the control-plane public IP
  JUMP_USER=$REMOTE_USER  SSH user for jump host

Examples:
  ./scripts/import-images-to-nodes.sh .dist/images/lab4-images-v1.tar 10.0.2.10 10.0.2.11 10.0.2.12
  JUMP_HOST=120.79.8.174 ./scripts/import-images-to-nodes.sh .dist/images/lab4-images-v1.tar 10.0.2.10 10.0.2.11
  RUNTIME=docker ./scripts/import-images-to-nodes.sh .dist/images/lab4-images-v1.tar 10.0.2.10
USAGE
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" || "$#" -lt 2 ]]; then
  usage
  exit 0
fi

IMAGE_TAR="$1"
shift

if [[ ! -f "$IMAGE_TAR" ]]; then
  echo "image tar not found: $IMAGE_TAR" >&2
  exit 1
fi

REMOTE_USER="${REMOTE_USER:-root}"
REMOTE_DIR="${REMOTE_DIR:-/root}"
RUNTIME="${RUNTIME:-containerd}"
JUMP_HOST="${JUMP_HOST:-}"
JUMP_USER="${JUMP_USER:-$REMOTE_USER}"
REMOTE_TAR="$REMOTE_DIR/$(basename "$IMAGE_TAR")"

SSH_OPTS=(-o StrictHostKeyChecking=no)
if [[ -n "$JUMP_HOST" ]]; then
  SSH_OPTS+=(-J "$JUMP_USER@$JUMP_HOST")
fi

for node in "$@"; do
  echo
  echo "==> Upload image tar to $REMOTE_USER@$node:$REMOTE_TAR"
  ssh "${SSH_OPTS[@]}" "$REMOTE_USER@$node" "mkdir -p '$REMOTE_DIR'"
  scp "${SSH_OPTS[@]}" "$IMAGE_TAR" "$REMOTE_USER@$node:$REMOTE_TAR"

  echo "==> Import images on $node, runtime=$RUNTIME"
  case "$RUNTIME" in
    containerd)
      ssh "${SSH_OPTS[@]}" "$REMOTE_USER@$node" "ctr -n k8s.io images import '$REMOTE_TAR' && (crictl images 2>/dev/null | grep -E 'lab4|redis' || ctr -n k8s.io images ls | grep -E 'lab4|redis')"
      ;;
    docker)
      ssh "${SSH_OPTS[@]}" "$REMOTE_USER@$node" "docker load -i '$REMOTE_TAR' && docker images | grep -E 'lab4|redis'"
      ;;
    auto)
      ssh "${SSH_OPTS[@]}" "$REMOTE_USER@$node" "if command -v ctr >/dev/null 2>&1; then ctr -n k8s.io images import '$REMOTE_TAR' && (crictl images 2>/dev/null | grep -E 'lab4|redis' || ctr -n k8s.io images ls | grep -E 'lab4|redis'); elif command -v docker >/dev/null 2>&1; then docker load -i '$REMOTE_TAR' && docker images | grep -E 'lab4|redis'; else echo 'no containerd ctr or docker found' >&2; exit 1; fi"
      ;;
    *)
      echo "unknown RUNTIME: $RUNTIME, expected containerd, docker, or auto" >&2
      exit 1
      ;;
  esac
done

echo
echo "All nodes imported the image tar."
