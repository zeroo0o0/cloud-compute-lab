#!/usr/bin/env bash
set -euo pipefail

# Simple load test for HPA demo
# Usage: bash scripts/hpa-loadtest.sh [target] [concurrency] [duration]
# Example: bash scripts/hpa-loadtest.sh http://127.0.0.1:18080 20 120

TARGET="${1:-http://127.0.0.1:18080}"
CONCURRENCY="${2:-10}"
DURATION="${3:-60}"

END_TIME=$(( $(date +%s) + DURATION ))

printf "=== Load Test ===\n"
printf "Target:      %s\n" "$TARGET"
printf "Concurrency: %s\n" "$CONCURRENCY"
printf "Duration:    %ss\n\n" "$DURATION"

req_worker() {
  local id="$1"
  local ok=0
  local fail=0
  while [ "$(date +%s)" -lt "$END_TIME" ]; do
    if curl -fsS "${TARGET}/move?player=loadtest&dir=north" >/dev/null 2>&1; then
      ok=$((ok+1))
    else
      fail=$((fail+1))
      sleep 0.1
    fi
  done
  echo "$id $ok $fail" >> /tmp/hpa-loadtest.$$ 
}

: > /tmp/hpa-loadtest.$$
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
