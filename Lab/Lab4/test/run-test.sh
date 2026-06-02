#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-}"
if [[ "$MODE" != "autotest" && "$MODE" != "scoretest" ]]; then
  echo "Usage: $0 autotest|scoretest [test args]" >&2
  exit 2
fi
shift || true

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
NAMESPACE="${LAB4_NAMESPACE:-lab4}"
REMOTE_DIR="${LAB4_REMOTE_DIR:-/tmp/lab4-test}"
SSH_USER="${LAB4_SSH_USER:-root}"
SSH_PORT="${LAB4_SSH_PORT:-22}"

if [[ "$MODE" == "autotest" ]]; then
  GO_TARGET="test/autotest.go"
else
  GO_TARGET="./test/scoretest"
fi

shell_quote() {
  printf "'%s'" "$(printf "%s" "$1" | sed "s/'/'\\\\''/g")"
}

quote_words() {
  local out=""
  local item
  for item in "$@"; do
    out+=" $(shell_quote "$item")"
  done
  printf "%s" "$out"
}

prompt_if_empty() {
  local var_name="$1"
  local prompt="$2"
  local default_value="${3:-}"
  local value="${!var_name:-}"
  if [[ -n "$value" ]]; then
    printf -v "$var_name" "%s" "$value"
    return
  fi
  if [[ -n "$default_value" ]]; then
    read -r -p "$prompt [$default_value]: " value
    value="${value:-$default_value}"
  else
    read -r -p "$prompt: " value
  fi
  if [[ -z "$value" ]]; then
    echo "$var_name cannot be empty" >&2
    exit 2
  fi
  printf -v "$var_name" "%s" "$value"
}

local_kubectl_ready() {
  command -v kubectl >/dev/null 2>&1 &&
    kubectl -n "$NAMESPACE" get svc lab4-gateway >/dev/null 2>&1
}

run_local() {
  local gateway_addr="${LAB4_GATEWAY_ADDR:-}"
  prompt_if_empty gateway_addr "Gateway address, for example 120.79.8.174:30910"
  echo "==> Local kubectl is available. Running $MODE locally."
  (
    cd "$ROOT_DIR"
    LAB4_NAMESPACE="$NAMESPACE" \
      LAB4_GATEWAY_ADDR="$gateway_addr" \
      LAB4_KUBECTL="${LAB4_KUBECTL:-kubectl}" \
      go run "$GO_TARGET" "$@"
  )
}

run_remote() {
  local ssh_host="${LAB4_SSH_HOST:-}"
  local gateway_addr="${LAB4_GATEWAY_ADDR:-}"
  prompt_if_empty ssh_host "Kubernetes master public IP"
  prompt_if_empty SSH_USER "SSH user" "$SSH_USER"
  prompt_if_empty SSH_PORT "SSH port" "$SSH_PORT"
  prompt_if_empty gateway_addr "Gateway address" "$ssh_host:30910"

  local ssh_target="$SSH_USER@$ssh_host"
  local ssh_opts=(-p "$SSH_PORT" -o StrictHostKeyChecking=accept-new)
  local archive
  archive="$(mktemp -t lab4-test.XXXXXX.tar.gz)"
  trap 'rm -f "$archive"' RETURN

  echo "==> Pack Lab4 files"
  COPYFILE_DISABLE=1 tar \
    --format ustar \
    --exclude ".git" \
    --exclude ".gocache" \
    --exclude ".dist" \
    --exclude "tmp" \
    -czf "$archive" \
    -C "$ROOT_DIR" .

  echo "==> Upload files and run $MODE on remote master"
  local remote_cmd
  remote_cmd="rm -rf $(shell_quote "$REMOTE_DIR")"
  remote_cmd+=" && mkdir -p $(shell_quote "$REMOTE_DIR")"
  remote_cmd+=" && tar -xzf - -C $(shell_quote "$REMOTE_DIR")"
  remote_cmd+=" && cd $(shell_quote "$REMOTE_DIR")"
  remote_cmd+=" && { command -v go >/dev/null 2>&1 || { echo 'Go is not installed on the master node.' >&2; exit 127; }; }"
  remote_cmd+=" && { command -v kubectl >/dev/null 2>&1 || { echo 'kubectl is not installed on the master node.' >&2; exit 127; }; }"
  remote_cmd+=" && LAB4_NAMESPACE=$(shell_quote "$NAMESPACE")"
  remote_cmd+=" LAB4_GATEWAY_ADDR=$(shell_quote "$gateway_addr")"
  remote_cmd+=" LAB4_KUBECTL=kubectl"
  if [[ -n "${LAB4_SKIP_DISRUPTIVE:-}" ]]; then
    remote_cmd+=" LAB4_SKIP_DISRUPTIVE=$(shell_quote "$LAB4_SKIP_DISRUPTIVE")"
  fi
  if [[ -n "${LAB4_SCORE_FAST:-}" ]]; then
    remote_cmd+=" LAB4_SCORE_FAST=$(shell_quote "$LAB4_SCORE_FAST")"
  fi
  if [[ -n "${LAB4_SKIP_CHAOS:-}" ]]; then
    remote_cmd+=" LAB4_SKIP_CHAOS=$(shell_quote "$LAB4_SKIP_CHAOS")"
  fi
  if [[ -n "${LAB4_SCORE_HIGH_CLIENTS:-}" ]]; then
    remote_cmd+=" LAB4_SCORE_HIGH_CLIENTS=$(shell_quote "$LAB4_SCORE_HIGH_CLIENTS")"
  fi
  remote_cmd+=" go run $(shell_quote "$GO_TARGET")$(quote_words "$@")"

  ssh "${ssh_opts[@]}" "$ssh_target" "$remote_cmd" < "$archive"
}

if local_kubectl_ready; then
  run_local "$@"
else
  echo "==> Local kubectl is not ready for namespace '$NAMESPACE'. Falling back to SSH."
  run_remote "$@"
fi
