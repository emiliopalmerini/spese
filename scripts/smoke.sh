#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-${SPESE_PORT:-8080}}"
BASE_URL="${BASE_URL:-http://127.0.0.1:${PORT}}"
SHEET_FILE="${SPESE_LOCAL_SHEET_PATH:-tmp/local-sheet.json}"
ORIGIN="${BASE_URL}"

echo "[smoke] Checking liveness and readiness at ${BASE_URL} ..."
curl -fsS "${BASE_URL}/healthz" >/dev/null
curl -fsS "${BASE_URL}/readyz" >/dev/null

suffix="${SMOKE_SUFFIX:-$(date +%s)-$$}"
ACCOUNT="${ACCOUNT:-Smoke Cash ${suffix}}"
PAYEE="${PAYEE:-Smoke Test ${suffix}}"
TODAY="${TODAY:-$(date +%F)}"

post_json() {
  local path="$1"
  local body="$2"
  local expected="$3"
  local key="smoke-${suffix}-${RANDOM}"
  local response code payload
  response=$(curl -sS -w "\n%{http_code}" -X POST \
    -H 'Content-Type: application/json' \
    -H 'X-Spese-CSRF: 1' \
    -H "Origin: ${ORIGIN}" \
    -H "Idempotency-Key: ${key}" \
    --data "$body" \
    "${BASE_URL}${path}")
  code=$(printf '%s' "$response" | tail -n1)
  payload=$(printf '%s' "$response" | sed '$d')
  if [[ "$code" != "$expected" ]]; then
    echo "[smoke] ${path}: expected ${expected}, got ${code}"
    printf '%s\n' "$payload"
    exit 1
  fi
  printf '%s' "$payload"
}

echo "[smoke] Creating account '${ACCOUNT}' ..."
account_json=$(post_json "/api/v1/accounts" "{\"name\":\"${ACCOUNT}\",\"type\":\"Asset\",\"class\":\"Cash\",\"initialBalanceCents\":0,\"initialDate\":\"${TODAY}\",\"activeFrom\":\"\",\"activeTo\":\"\",\"note\":\"smoke\"}" 201)
account_id=$(printf '%s' "$account_json" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')

echo "[smoke] Creating category ..."
category_json=$(post_json "/api/v1/categories" "{\"kind\":\"expense\",\"name\":\"Smoke ${suffix}\",\"icon\":\"flame\",\"color\":\"#725B86\",\"sortOrder\":0}" 201)
category_id=$(printf '%s' "$category_json" | sed -n 's/.*"id":"\([^"]*\)".*/\1/p')

if [[ -z "$account_id" || -z "$category_id" ]]; then
  echo "[smoke] Could not parse created IDs"
  exit 1
fi

echo "[smoke] Creating movement '${PAYEE}' ..."
post_json "/api/v1/movements" "{\"kind\":\"expense\",\"status\":\"posted\",\"date\":\"${TODAY}\",\"accountId\":\"${account_id}\",\"amountCents\":123,\"merchant\":\"${PAYEE}\",\"note\":\"smoke\",\"origin\":\"manual\",\"allocations\":[{\"categoryId\":\"${category_id}\",\"amountCents\":123}]}" 201 >/dev/null

if [[ "${SPESE_SHEET_MIRROR_BACKEND:-}" == "local" || "${VERIFY_LOCAL_SHEET:-}" == "1" ]]; then
  echo "[smoke] Waiting for Rabbit worker to write ${SHEET_FILE} ..."
  for _ in {1..100}; do
    if [[ -f "$SHEET_FILE" ]] && grep -Fq "$ACCOUNT" "$SHEET_FILE" && grep -Fq "$PAYEE" "$SHEET_FILE"; then
      echo "[smoke] Local sheet mirror OK"
      exit 0
    fi
    sleep 0.1
  done
  echo "[smoke] Local sheet mirror did not contain smoke rows"
  exit 1
fi

echo "[smoke] OK"
