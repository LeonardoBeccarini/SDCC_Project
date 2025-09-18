#!/usr/bin/env bash
set -euo pipefail
# Usage:
#   GRAFANA_URL=http://localhost:3000 API_KEY=xxx ./grab_panel_png.sh <dashboard-uid> <panelId> [from] [to] [out]
# Example:
#   GRAFANA_URL=http://localhost:3000 API_KEY=xxx ./grab_panel_png.sh abCdEfG 12 now-6h now panel.png

URL="${GRAFANA_URL:-http://localhost:3000}"
KEY="${API_KEY:?set API_KEY}"
UID="${1:?dashboard uid}"; PANEL="${2:?panelId}"
FROM="${3:-now-6h}"; TO="${4:-now}"
OUT="${5:-panel.png}"

curl -fsSL -H "Authorization: Bearer $KEY" \
  "$URL/render/d-solo/$UID/_?panelId=$PANEL&from=$FROM&to=$TO&width=1600&height=900&tz=Europe/Rome" \
  -o "$OUT"

echo "✔ salvato $OUT"
