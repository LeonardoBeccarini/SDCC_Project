#!/usr/bin/env bash
set -euo pipefail

PROM_PORT="${PROM_PORT:-9091}"
SVC_NS="${SVC_NS:-monitoring}"
SVC_NAME="${SVC_NAME:-kube-prometheus-stack-prometheus}"

need(){ command -v "$1" >/dev/null 2>&1 || { echo "ERROR: manca $1"; exit 1; }; }
need kubectl; need jq; need curl; need lsof

# port-forward se non già pronto
if ! curl -sf "http://127.0.0.1:$PROM_PORT/-/ready" >/dev/null 2>&1; then
  lsof -ti :"$PROM_PORT" | xargs -r kill || true
  if ! kubectl -n "$SVC_NS" get svc "$SVC_NAME" >/dev/null 2>&1; then
    # fallback: trova un service con la 9090
    SVC_NAME=$(kubectl -n "$SVC_NS" get svc -o json \
      | jq -r '.items[] | select([.spec.ports[]? | select(.port==9090)] | length>0) | .metadata.name' | head -n1)
  fi
  echo "Port-forward → http://localhost:$PROM_PORT  (svc/$SVC_NAME)"
  kubectl -n "$SVC_NS" port-forward "svc/$SVC_NAME" "$PROM_PORT:9090" >/tmp/prom_pf.log 2>&1 &
  PF=$!
  trap 'kill $PF >/dev/null 2>&1 || true' EXIT
  for _ in {1..40}; do
    curl -sf "http://127.0.0.1:$PROM_PORT/-/ready" >/dev/null && break || sleep 0.25
  done
fi

BASE="http://127.0.0.1:$PROM_PORT/api/v1/query"
runq () {
  local title="$1" expr="$2"
  echo -e "\n▶ $title"
  curl -sG "$BASE" --data-urlencode "query=$expr" | jq -r '
    if .status=="success" then
      if (.data.result|length)==0 then "  (no data yet)"
      else .data.result[] | "  -> " + .value[1]
      end
    else "  ERROR: " + (.error // "unknown")
    end'
}

# ---- Query chiave ----
runq "HTTP availability (current)"                      'probe_success{job="blackbox-http"}'
runq "HTTP availability avg(6h)"                        'avg_over_time(probe_success{job="blackbox-http"}[6h])'
runq "HTTP latency p95 (30m)"                           'quantile_over_time(0.95, probe_duration_seconds{job="blackbox-http"}[30m])'
runq "TCP MQTT availability min(30m)"                   'min_over_time(probe_success{job="blackbox-tcp"}[30m])'
runq "RabbitMQ backlog totale (messages_ready)"         'sum(rabbitmq_queue_messages_ready)'
runq "RabbitMQ consumers totali"                        'sum(rabbitmq_queue_consumers)'
runq "RabbitMQ publish rate totale (msg/s)"             'sum(rate(rabbitmq_queue_messages_published_total[5m]))'

# (facoltative) risorse pod/nodi:
# runq "CPU per pod (cores)"  'sum by (namespace,pod) (rate(container_cpu_usage_seconds_total{image!="",container!="POD"}[5m]))'
# runq "RAM per pod (bytes)"  'sum by (namespace,pod) (container_memory_working_set_bytes{image!="",container!="POD"})'

echo -e "\nDone. Prometheus: http://localhost:$PROM_PORT"
wait || true
