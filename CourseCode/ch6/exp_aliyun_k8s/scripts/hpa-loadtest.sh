#!/usr/bin/env bash
set -euo pipefail

# Simple load test for HPA demo (TCP gateway protocol)
# Usage: bash scripts/hpa-loadtest.sh [target] [concurrency] [duration]
# Target can be http://host:port, tcp://host:port, or host:port
# Example: bash scripts/hpa-loadtest.sh http://127.0.0.1:18080 20 120

TARGET="${1:-http://127.0.0.1:18080}"
CONCURRENCY="${2:-10}"
DURATION="${3:-60}"
NAMESPACE="${NAMESPACE:-exp-aliyun-k8s}"
GATEWAY_SERVICE="${GATEWAY_SERVICE:-gateway}"
AUTO_PORT_FORWARD="${AUTO_PORT_FORWARD:-1}"
PORT_FORWARD_PID=""

TARGET_HOSTPORT="${TARGET#*://}"
TARGET_HOSTPORT="${TARGET_HOSTPORT%%/*}"
if [[ "$TARGET_HOSTPORT" == *":"* ]]; then
  TARGET_HOST="${TARGET_HOSTPORT%%:*}"
  TARGET_PORT="${TARGET_HOSTPORT##*:}"
else
  TARGET_HOST="$TARGET_HOSTPORT"
  TARGET_PORT="8080"
fi

END_TIME=$(( $(date +%s) + DURATION ))

printf "=== Load Test ===\n"
printf "Target:      %s\n" "$TARGET"
printf "Host:        %s\n" "$TARGET_HOST"
printf "Port:        %s\n" "$TARGET_PORT"
printf "Concurrency: %s\n" "$CONCURRENCY"
printf "Duration:    %ss\n\n" "$DURATION"

cleanup() {
  if [[ -n "$PORT_FORWARD_PID" ]]; then
    kill "$PORT_FORWARD_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

is_port_open() {
  local host="$1"
  local port="$2"
  if { exec 3<>"/dev/tcp/${host}/${port}"; } >/dev/null 2>&1; then
    exec 3>&-
    return 0
  fi
  return 1
}

maybe_start_port_forward() {
  if [[ "$TARGET_HOST" != "127.0.0.1" && "$TARGET_HOST" != "localhost" ]]; then
    return 0
  fi
  if is_port_open "$TARGET_HOST" "$TARGET_PORT"; then
    return 0
  fi
  if [[ "$AUTO_PORT_FORWARD" != "1" ]]; then
    echo "Port ${TARGET_HOST}:${TARGET_PORT} not reachable. Start port-forward first:" >&2
    echo "kubectl -n ${NAMESPACE} port-forward svc/${GATEWAY_SERVICE} ${TARGET_PORT}:8080" >&2
    exit 1
  fi

  echo "Starting port-forward svc/${GATEWAY_SERVICE} -> ${TARGET_PORT}:8080" >&2
  kubectl -n "$NAMESPACE" port-forward "svc/${GATEWAY_SERVICE}" "${TARGET_PORT}:8080" \
    >/tmp/hpa-loadtest-portforward.$$ 2>&1 &
  PORT_FORWARD_PID=$!

  for _ in $(seq 1 20); do
    if is_port_open "$TARGET_HOST" "$TARGET_PORT"; then
      return 0
    fi
    sleep 0.2
  done

  echo "Port-forward did not become ready. Logs:" >&2
  tail -n 5 /tmp/hpa-loadtest-portforward.$$ >&2 || true
  exit 1
}

send_command() {
  local host="$1"
  local port="$2"
  local payload="$3"
  if { exec 3<>"/dev/tcp/${host}/${port}"; } >/dev/null 2>&1; then
    printf "%s\n" "$payload" >&3
    if read -r -t 1 _ <&3 2>/dev/null; then
      exec 3>&-
      return 0
    fi
    exec 3>&-
    return 0
  fi
  return 1
}

req_worker() {
  local id="$1"
  local ok=0
  local fail=0
  local dirs=(w a s d)
  while [ "$(date +%s)" -lt "$END_TIME" ]; do
    local dir="${dirs[$((RANDOM % 4))]}"
    local cmd="MOVE loadtest-${id} ${dir}"
    if send_command "$TARGET_HOST" "$TARGET_PORT" "$cmd"; then
      ok=$((ok+1))
    else
      fail=$((fail+1))
      sleep 0.1
    fi
  done
  echo "$id $ok $fail" >> /tmp/hpa-loadtest.$$ 
}

: > /tmp/hpa-loadtest.$$
maybe_start_port_forward
for i in $(seq 1 "$CONCURRENCY"); do
  req_worker "$i" &
 done
wait

TOTAL_OK=$(awk '{s+=$2} END {print s+0}' /tmp/hpa-loadtest.$$)
TOTAL_FAIL=$(awk '{s+=$3} END {print s+0}' /tmp/hpa-loadtest.$$)
TOTAL=$((TOTAL_OK + TOTAL_FAIL))
RPS=$(awk -v t="$TOTAL" -v d="$DURATION" 'BEGIN {printf "%.1f", t/d}')
SUCCESS_RATE=$(awk -v ok="$TOTAL_OK" -v t="$TOTAL" 'BEGIN {if (t==0) print "0.0"; else printf "%.1f", ok/t*100}')

printf "\n=== Summary ===\n"
printf "Total Requests:  %s\n" "$TOTAL"
printf "Successful:      %s\n" "$TOTAL_OK"
printf "Failed:          %s\n" "$TOTAL_FAIL"
printf "RPS:             %s\n" "$RPS"
printf "Success Rate:    %s%%\n" "$SUCCESS_RATE"

rm -f /tmp/hpa-loadtest.$$
