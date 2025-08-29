#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-8080}"
BASE_URL="${BASE_URL:-http://localhost:${PORT}}"

echo "[smoke] Base URL: ${BASE_URL}"

echo "[smoke] Checking /healthz ..."
curl -fsS "${BASE_URL}/healthz" >/dev/null
echo "[smoke] /healthz OK"

echo "[smoke] Fetching index to infer taxonomy ..."
HTML=$(curl -fsS "${BASE_URL}/")

category_block=$(printf '%s' "$HTML" | awk '/select name="category"/,/<\/select>/') || true
subcategory_block=$(printf '%s' "$HTML" | awk '/select name="subcategory"/,/<\/select>/') || true

CATEGORY=${CATEGORY:-$(printf '%s' "$category_block" | grep -o '<option>[^<]*</option>' | head -n1 | sed -E 's#<option>([^<]+)</option>#\1#')}
SUBCATEGORY=${SUBCATEGORY:-$(printf '%s' "$subcategory_block" | grep -o '<option>[^<]*</option>' | head -n1 | sed -E 's#<option>([^<]+)</option>#\1#')}

if [[ -z "${CATEGORY}" ]]; then CATEGORY="Casa"; fi
if [[ -z "${SUBCATEGORY}" ]]; then SUBCATEGORY="Generale"; fi

echo "[smoke] Using CATEGORY='${CATEGORY}', SUBCATEGORY='${SUBCATEGORY}'"

DAY=$(date +%d)
MONTH=$(date +%m | sed 's/^0*//')

echo "[smoke] Posting /expenses ..."
RESP=$(curl -sS -w "\n%{http_code}" -X POST \
  -H 'Content-Type: application/x-www-form-urlencoded' \
  --data-urlencode "day=${DAY}" \
  --data-urlencode "month=${MONTH}" \
  --data-urlencode "description=Smoke Test" \
  --data-urlencode "amount=1.23" \
  --data-urlencode "category=${CATEGORY}" \
  --data-urlencode "subcategory=${SUBCATEGORY}" \
  "${BASE_URL}/expenses")

BODY=$(printf '%s' "$RESP" | sed '$d')
CODE=$(printf '%s' "$RESP" | tail -n1)

if [[ "$CODE" != "200" ]]; then
  echo "[smoke] Unexpected status: $CODE"
  echo "[smoke] Response body:" && echo "$BODY"
  exit 1
fi

echo "[smoke] OK: $CODE"
echo "$BODY"

