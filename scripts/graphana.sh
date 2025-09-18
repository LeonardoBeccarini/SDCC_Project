#!/usr/bin/env bash
set -euo pipefail

# ===== Parametri =====
NS="${NS:-monitoring}"
REL="${REL:-kube-prometheus-stack}"
PORT="${GRAFANA_PORT:-3000}"
USER="${GRAFANA_USER:-admin}"
PASS="${GRAFANA_PASS:-sdcc}"
OPEN_BROWSER="${OPEN_BROWSER:-1}"   # 1 = prova ad aprire il browser

need(){ command -v "$1" >/dev/null 2>&1 || { echo "ERROR: manca $1"; exit 1; }; }
need kubectl; need curl; need lsof

log(){ printf "\033[1;36m==> %s\033[0m\n" "$*"; }

# trova il Service/POD di Grafana
find_grafana_target() {
  # 1) Service standard della kube-prometheus-stack
  kubectl -n "$NS" get svc "${REL}-grafana" -o name 2>/dev/null | sed -n '1p' && return 0 || true
  # 2) Primo Service con label grafana
  kubectl -n "$NS" get svc -l app.kubernetes.io/name=grafana -o jsonpath='{.items[0].metadata.name}' 2>/dev/null \
    | sed -n '1{s/^/service\//;p}' && return 0 || true
  # 3) Pod come fallback
  kubectl -n "$NS" get pod -l app.kubernetes.io/name=grafana -o name 2>/dev/null | sed -n '1p' && return 0 || true
  return 1
}

log "Cerco Grafana in ns=$NS …"
TARGET="$(find_grafana_target || true)"
[[ -z "$TARGET" ]] && { echo "ERROR: Grafana non trovato nel namespace $NS"; exit 1; }
echo "  -> $TARGET"

# libera la porta locale
lsof -ti :"$PORT" | xargs -r kill || true

# determina la porta del Service (di solito 80)
if [[ "$TARGET" == service/* ]]; then
  SVC="${TARGET#service/}"
  SVC_PORT="$(kubectl -n "$NS" get svc "$SVC" -o jsonpath='{.spec.ports[0].port}')"
  [[ -z "$SVC_PORT" ]] && SVC_PORT=80
  log "Port-forward: http://localhost:$PORT  →  svc/$SVC:$SVC_PORT"
  kubectl -n "$NS" port-forward "svc/$SVC" "$PORT:$SVC_PORT" >/tmp/grafana_pf.log 2>&1 &
else
  POD="${TARGET#pod/}"
  log "Port-forward: http://localhost:$PORT  →  pod/$POD:3000"
  kubectl -n "$NS" port-forward "pod/$POD" "$PORT:3000" >/tmp/grafana_pf.log 2>&1 &
fi
PF_PID=$!
trap 'kill $PF_PID >/dev/null 2>&1 || true' EXIT

# attendo readiness (Grafana /api/health non richiede auth)
for _ in {1..60}; do
  if curl -fsS "http://127.0.0.1:$PORT/api/health" >/dev/null; then
    break
  fi
  sleep 0.3
done

echo
echo "Grafana pronto su:  http://localhost:$PORT"
echo "Credenziali:       $USER / $PASS"
echo

if [[ "$OPEN_BROWSER" == "1" ]]; then
  (xdg-open "http://localhost:$PORT" || open "http://localhost:$PORT" || true) >/dev/null 2>&1
fi

echo "Lascia lo script aperto per mantenere attivo il port-forward (CTRL+C per uscire)."
wait
#chmod +x scripts/grafana.sh
# variabili opzionali: NS, REL, GRAFANA_PORT, GRAFANA_USER, GRAFANA_PASS, OPEN_BROWSER
#./scripts/grafana.sh