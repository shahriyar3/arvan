#!/usr/bin/env bash
# End-to-end demo for reviewers: health → topup → send → delivery → idempotency → reports.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TOKEN_A="${TOKEN_A:-demo-token-account-a}"
TOKEN_B="${TOKEN_B:-demo-token-account-b}"

bold() { printf '\033[1m%s\033[0m\n' "$*"; }
step() { printf '\n→ %s\n' "$*"; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

require_cmd curl
require_cmd jq

wait_for_status() {
  local message_id=$1
  local expected=$2
  local max_attempts=${3:-30}
  local status=""
  for ((i = 1; i <= max_attempts; i++)); do
    status=$(curl -sf "$BASE_URL/v1/sms/$message_id" \
      -H "X-Account-Token: $TOKEN_A" | jq -r .status)
    if [[ "$status" == "$expected" ]]; then
      echo "status=$status (attempt $i)"
      return 0
    fi
    sleep 1
  done
  echo "timeout waiting for status=$expected (last: ${status:-unknown})" >&2
  return 1
}

bold "SMS Gateway demo"
echo "API: $BASE_URL"

step "1/8 — readiness check"
curl -sf "$BASE_URL/health/ready" | jq .

step "2/8 — account balance (after make seed)"
curl -sf "$BASE_URL/v1/account/balance" \
  -H "X-Account-Token: $TOKEN_A" | jq .

step "3/8 — topup 100 units"
curl -sf -X POST "$BASE_URL/v1/account/topup" \
  -H "X-Account-Token: $TOKEN_A" \
  -H "Content-Type: application/json" \
  -d '{"amount":100}' | jq .

step "4/8 — send standard SMS (202 Accepted)"
SEND_RESP=$(curl -sf -X POST "$BASE_URL/v1/sms/send" \
  -H "X-Account-Token: $TOKEN_A" \
  -H "Content-Type: application/json" \
  -d '{"to":"+989121234567","body":"Hello from demo.sh","message_type":"standard"}')
echo "$SEND_RESP" | jq .
MESSAGE_ID=$(echo "$SEND_RESP" | jq -r .message_id)

step "5/8 — wait for async delivery (status=sent)"
wait_for_status "$MESSAGE_ID" "sent"

step "6/8 — idempotent retry (same key → same message_id)"
IDEMPOTENCY_KEY="$(uuidgen 2>/dev/null || python3 -c 'import uuid; print(uuid.uuid4())')"
FIRST=$(curl -sf -X POST "$BASE_URL/v1/sms/send" \
  -H "X-Account-Token: $TOKEN_A" \
  -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
  -H "Content-Type: application/json" \
  -d '{"to":"+989121234568","body":"OTP 1234","message_type":"express"}')
SECOND=$(curl -sf -X POST "$BASE_URL/v1/sms/send" \
  -H "X-Account-Token: $TOKEN_A" \
  -H "Idempotency-Key: $IDEMPOTENCY_KEY" \
  -H "Content-Type: application/json" \
  -d '{"to":"+989121234568","body":"OTP 1234","message_type":"express"}')
echo "$FIRST" | jq .
echo "$SECOND" | jq .
FIRST_ID=$(echo "$FIRST" | jq -r .message_id)
SECOND_ID=$(echo "$SECOND" | jq -r .message_id)
if [[ "$FIRST_ID" != "$SECOND_ID" ]]; then
  echo "idempotency failed: message_id mismatch" >&2
  exit 1
fi
echo "idempotency OK: $FIRST_ID"

step "7/8 — list messages + get by id"
curl -sf "$BASE_URL/v1/sms?limit=5" \
  -H "X-Account-Token: $TOKEN_A" | jq .
curl -sf "$BASE_URL/v1/sms/$MESSAGE_ID" \
  -H "X-Account-Token: $TOKEN_A" | jq .

step "8/8 — multi-tenant isolation (account B cannot read account A message)"
HTTP_CODE=$(curl -s -o /dev/null -w '%{http_code}' "$BASE_URL/v1/sms/$MESSAGE_ID" \
  -H "X-Account-Token: $TOKEN_B")
if [[ "$HTTP_CODE" != "404" ]]; then
  echo "expected 404 for cross-account access, got $HTTP_CODE" >&2
  exit 1
fi
echo "isolation OK: account B got 404"

bold "Demo completed successfully"
echo "Swagger UI:  $BASE_URL/swagger/index.html"
echo "Metrics:     $BASE_URL/metrics"
echo "Grafana:     http://localhost:3000/d/sms-gateway/sms-gateway (admin / admin)"
echo "Prometheus:  http://localhost:9090/targets"
echo "Jaeger UI:   http://localhost:16686"
