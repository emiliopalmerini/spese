#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-${SPESE_PORT:-8080}}"
BASE_URL="${BASE_URL:-http://localhost:${PORT}}"
SHEET_FILE="${SPESE_LOCAL_SHEET_PATH:-tmp/local-sheet.json}"

echo "[smoke] Base URL: ${BASE_URL}"

echo "[smoke] Checking /healthz ..."
curl -fsS "${BASE_URL}/healthz" >/dev/null
echo "[smoke] /healthz OK"

suffix="${SMOKE_SUFFIX:-$(date +%s)-$$}"
ACCOUNT="${ACCOUNT:-Smoke Cash ${suffix}}"
PAYEE="${PAYEE:-Smoke Test ${suffix}}"
TODAY="${TODAY:-$(date +%F)}"
CATEGORY="${CATEGORY:-Smoke}"
SUBCATEGORY="${SUBCATEGORY:-Local}"

post_form() {
  local path="$1"
  shift

  local resp body code
  resp=$(curl -sS -w "\n%{http_code}" -X POST \
    -H 'Content-Type: application/x-www-form-urlencoded' \
    "$@" \
    "${BASE_URL}${path}")
  body=$(printf '%s' "$resp" | sed '$d')
  code=$(printf '%s' "$resp" | tail -n1)

  if [[ "$code" != "200" ]]; then
    echo "[smoke] Unexpected status from ${path}: ${code}"
    echo "[smoke] Response body:" && echo "$body"
    exit 1
  fi
  echo "[smoke] ${path} OK: ${code}"
}

echo "[smoke] Creating account '${ACCOUNT}' ..."
post_form "/accounts" \
  --data-urlencode "name=${ACCOUNT}" \
  --data-urlencode "type=Asset" \
  --data-urlencode "class=Cash" \
  --data-urlencode "currency=EUR" \
  --data-urlencode "note=smoke"

echo "[smoke] Creating transaction '${PAYEE}' ..."
post_form "/transactions" \
  --data-urlencode "kind=Expense" \
  --data-urlencode "date=${TODAY}" \
  --data-urlencode "account=${ACCOUNT}" \
  --data-urlencode "amount=1.23" \
  --data-urlencode "payee=${PAYEE}" \
  --data-urlencode "category=${CATEGORY}" \
  --data-urlencode "subcategory=${SUBCATEGORY}" \
  --data-urlencode "note=smoke"

if [[ "${SPESE_SHEET_MIRROR_BACKEND:-}" == "local" || "${VERIFY_LOCAL_SHEET:-}" == "1" ]]; then
  echo "[smoke] Waiting for Honker worker to write ${SHEET_FILE} ..."
  for _ in {1..50}; do
    if [[ -f "$SHEET_FILE" ]] && grep -Fq "$ACCOUNT" "$SHEET_FILE" && grep -Fq "$PAYEE" "$SHEET_FILE"; then
      echo "[smoke] Local sheet mirror OK"
      exit 0
    fi
    sleep 0.1
  done
  echo "[smoke] Local sheet mirror did not contain smoke rows"
  [[ -f "$SHEET_FILE" ]] && cat "$SHEET_FILE"
  exit 1
fi

echo "[smoke] OK"
