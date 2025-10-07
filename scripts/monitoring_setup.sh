#!/usr/bin/env bash
set -euo pipefail

# ===== Parametri =====
NS_OP="${NS_OP:-monitoring}"
REL="${REL:-kube-prometheus-stack}"
BLACKBOX_REL="${BLACKBOX_REL:-blackbox}"
RABBIT_NS="${RABBIT_NS:-rabbitmq}"
RABBIT_SVC="${RABBIT_SVC:-rabbitmq}"
GATEWAY_HOST="${GATEWAY_HOST:-}"      # es: my-tunnel.ngrok.app (senza /healthz)
PROM_PORT="${PROM_PORT:-9091}"
KPS_VALUES="${KPS_VALUES:-k8s/monitoring/helm-values/kps-values.yaml}"
OPEN_PF="${OPEN_PF:-0}"               # =1 apre port-forward Prometheus a fine setup
MINIKUBE_AUTOSTART="${MINIKUBE_AUTOSTART:-1}"
MINIKUBE_PROFILE="${MINIKUBE_PROFILE:-sdcc-cluster}"

# === Nuove variabili per Grafana ===
DASHBOARD_JSON="${DASHBOARD_JSON:-metrics/dashboards/rabbitmq-dashboard.json}"
DATASOURCE_YAML="${DATASOURCE_YAML:-k8s/monitoring/manifests/grafana-datasource-cm.yaml}"

need(){ command -v "$1" >/dev/null 2>&1 || { echo "ERROR: manca $1"; exit 1; }; }
need kubectl; need helm; need jq; need lsof; need curl

log(){ printf "\033[1;36m==> %s\033[0m\n" "$*"; }

check_cluster() {
  if kubectl cluster-info >/dev/null 2>&1; then return 0; fi
  if command -v minikube >/dev/null 2>&1 && [[ "$MINIKUBE_AUTOSTART" == "1" ]]; then
    log "Cluster non raggiungibile, avvio minikube ($MINIKUBE_PROFILE)…"
    minikube start -p "$MINIKUBE_PROFILE" --driver=docker --cpus=4 --memory=6g
    minikube update-context -p "$MINIKUBE_PROFILE"
  fi
  kubectl cluster-info >/dev/null 2>&1 || { echo "ERROR: nessun cluster raggiungibile"; exit 1; }
}

# === Nuova funzione: crea/aggiorna CM dashboard SENZA annotations giganti ===
upsert_dashboard_cm() {
  local ns="$1" name="$2" file="$3" label_key="grafana_dashboard" label_val="1"

  if [[ ! -f "$file" ]]; then
    echo "WARN: dashboard JSON non trovato: $file — salto import."
    return 0
  fi

  if kubectl -n "$ns" get configmap "$name" >/dev/null 2>&1; then
    log "Aggiorno ConfigMap $name (replace)…"
    kubectl -n "$ns" create configmap "$name" \
      --from-file="rabbitmq-dashboard.json=$file" \
      --dry-run=client -o yaml | kubectl replace -f -
  else
    log "Creo ConfigMap $name…"
    kubectl -n "$ns" create configmap "$name" \
      --from-file="rabbitmq-dashboard.json=$file"
    kubectl -n "$ns" label configmap "$name" "$label_key=$label_val" --overwrite
  fi
}

check_cluster

log "Stack ns=$NS_OP  release=$REL  blackbox=$BLACKBOX_REL  rabbitmq=$RABBIT_NS/$RABBIT_SVC"

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts >/dev/null
helm repo update >/dev/null

log "Install/Upgrade kube-prometheus-stack…"
if [[ -f "$KPS_VALUES" ]]; then
  helm upgrade --install "$REL" prometheus-community/kube-prometheus-stack \
    -n "$NS_OP" --create-namespace -f "$KPS_VALUES" --set crds.enabled=true
else
  helm upgrade --install "$REL" prometheus-community/kube-prometheus-stack \
    -n "$NS_OP" --create-namespace --set crds.enabled=true
fi

log "Attendo l'operator…"
kubectl -n "$NS_OP" rollout status deploy/${REL}-operator --timeout=5m

log "CRD presenti?"
kubectl api-resources --api-group=monitoring.coreos.com | grep -E 'servicemonitors|probes|prometheuses' >/dev/null

log "Install/Upgrade blackbox_exporter…"
helm upgrade --install "$BLACKBOX_REL" prometheus-community/prometheus-blackbox-exporter -n "$NS_OP" \
  --set config.modules.http_2xx.prober=http \
  --set config.modules.http_2xx.timeout=10s \
  --set config.modules.http_2xx.http.valid_http_versions='{HTTP/1.1,HTTP/2}' \
  --set config.modules.http_2xx.http.follow_redirects=true \
  --set config.modules.tcp_connect.prober=tcp \
  --set config.modules.tcp_connect.timeout=5s

# ===== Grafana: datasource (se c'è) + dashboard RabbitMQ via CM =====
if [[ -f "$DATASOURCE_YAML" ]]; then
  log "Applico datasource Grafana: $DATASOURCE_YAML"
  kubectl -n "$NS_OP" apply -f "$DATASOURCE_YAML"
else
  echo "INFO: datasource YAML non trovato ($DATASOURCE_YAML) — salto."
fi

# Import dashboard RabbitMQ (no server-side apply)
upsert_dashboard_cm "$NS_OP" "grafana-dashboard-rabbitmq" "$DASHBOARD_JSON"

# (opzionale) forza il reload facendo restart della grafana
kubectl -n "$NS_OP" rollout restart deploy ${REL}-grafana || true

# ===== Blackbox + RabbitMQ SM/Probes =====
BBSVC="$(kubectl -n "$NS_OP" get svc -l app.kubernetes.io/instance="$BLACKBOX_REL" -o jsonpath='{.items[0].metadata.name}')"
log "blackbox service: $BBSVC.$NS_OP.svc.cluster.local:9115"

log "Preparo Service $RABBIT_NS/$RABBIT_SVC…"
if kubectl -n "$RABBIT_NS" get svc "$RABBIT_SVC" >/dev/null 2>&1; then
  kubectl -n "$RABBIT_NS" patch svc "$RABBIT_SVC" -p \
    '{"spec":{"ports":[
      {"name":"metrics","port":15692,"targetPort":15692},
      {"name":"mqtt","port":1883,"targetPort":1883},
      {"name":"amqp","port":5672,"targetPort":5672},
      {"name":"mgmt","port":15672,"targetPort":15672}
    ],
    "selector":{"app":"rabbitmq"}}}' >/dev/null || true
  kubectl -n "$RABBIT_NS" label svc "$RABBIT_SVC" app.kubernetes.io/component=metrics --overwrite >/dev/null
else
  echo "WARN: Service $RABBIT_NS/$RABBIT_SVC non trovato; salto patch."
fi

log "ServiceMonitor rabbitmq…"
cat <<'YAML' | kubectl apply -n "${NS_OP}" -f - >/dev/null
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: rabbitmq
  labels: { release: kube-prometheus-stack }
spec:
  selector:
    matchLabels: { app.kubernetes.io/component: metrics }
  namespaceSelector: { matchNames: ["rabbitmq"] }
  endpoints:
  - port: metrics
    path: /metrics
    interval: 30s
YAML

log "Probe TCP (MQTT:1883)…"
cat > /tmp/probe-tcp.yaml <<EOF
apiVersion: monitoring.coreos.com/v1
kind: Probe
metadata:
  name: tcp-core-services
  namespace: ${NS_OP}
  labels: { release: kube-prometheus-stack }
spec:
  jobName: blackbox-tcp
  prober: { url: http://${BBSVC}.${NS_OP}.svc.cluster.local:9115 }
  module: tcp_connect
  targets:
    staticConfig: { static: [ ${RABBIT_SVC}.${RABBIT_NS}.svc.cluster.local:1883 ] }
EOF
kubectl apply -f /tmp/probe-tcp.yaml >/dev/null

if [[ -n "$GATEWAY_HOST" ]]; then
  log "Probe HTTP su https://${GATEWAY_HOST}/healthz…"
  cat > /tmp/probe-http.yaml <<EOF
apiVersion: monitoring.coreos.com/v1
kind: Probe
metadata:
  name: http-gateway
  namespace: ${NS_OP}
  labels: { release: kube-prometheus-stack }
spec:
  jobName: blackbox-http
  prober: { url: http://${BBSVC}.${NS_OP}.svc.cluster.local:9115 }
  module: http_2xx
  targets:
    staticConfig: { static: [ https://${GATEWAY_HOST}/healthz ] }
EOF
  kubectl apply -f /tmp/probe-http.yaml >/dev/null
else
  echo "INFO: GATEWAY_HOST non impostato → salto Probe HTTP."
fi

# apri port-forward Prometheus (opzionale)
if [[ "$OPEN_PF" == "1" ]]; then
  log "Apro port-forward Prometheus su http://localhost:$PROM_PORT …"
  SVC=$(kubectl -n "$NS_OP" get svc prometheus-operated -o name 2>/dev/null | cut -d/ -f2 \
     || kubectl -n "$NS_OP" get svc ${REL}-prometheus -o name 2>/dev/null | cut -d/ -f2)
  lsof -ti :"$PROM_PORT" | xargs -r kill || true
  kubectl -n "$NS_OP" port-forward "svc/$SVC" "$PROM_PORT:9090"
else
  echo "Setup finito. Per vedere Prometheus:"
  echo "  kubectl -n $NS_OP port-forward svc/kube-prometheus-stack-prometheus $PROM_PORT:9090"
fi
kubectl -n monitoring apply -f k8s/monitoring/manifests/probes/probe-fog.yaml
kubectl -n monitoring apply -f k8s/monitoring/manifests/probes/probe-edge.yaml

