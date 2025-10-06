#!/usr/bin/env bash
set -Eeuo pipefail

### ========= PARAMETRI =========
# Dove vive il TUO Dockerfile reale del gateway (puoi sovrascrivere via env)
: "${DOCKERFILE:=${DOCKERFILE:-$(pwd)/Dockerfile}}"
# Directory di build (contesto docker)
: "${BUILD_CONTEXT:=${BUILD_CONTEXT:-$(git rev-parse --show-toplevel 2>/dev/null || pwd)}}"
# Tag locale per l'immagine del tuo gateway
: "${IMAGE_NAME:=gateway:cbtest}"

# Target namespace/endpoint da usare nel test isolato
: "${NS:=cbtest}"
: "${GATEWAY_SVC:=gateway}"
: "${GATEWAY_PORT:=5009}"
: "${GATEWAY_PATH:=/dashboard/data}"

# Config CB/timeout da passare al tuo gateway (se le rispetta)
: "${CB_FAILURES_TO_TRIP:=3}"
: "${CB_OPEN_FOR_MS:=2000}"
: "${CB_HALF_OPEN_MAX:=2}"
: "${HTTP_TIMEOUT_MS:=600}"

# Parametri scenario
: "${SLOW_MS:=800}"

### ========= PREREQUISITI =========
if ! command -v kubectl >/dev/null 2>&1; then
  echo "[error] kubectl non trovato"; exit 1
fi

# Se usi Minikube: buildiamo direttamente nel suo daemon Docker
if command -v minikube >/dev/null 2>&1 && kubectl config current-context | grep -qi minikube; then
  echo "[info] using Minikube docker-env"
  eval "$(minikube docker-env)"
fi

### ========= BUILD DELLA TUA IMMAGINE =========
if [ ! -f "$DOCKERFILE" ]; then
  echo "[error] Dockerfile non trovato: $DOCKERFILE"
  echo "        Sovrascrivi con: DOCKERFILE=/path/al/tuo/Dockerfile BUILD_CONTEXT=/path/al/repo $0"
  exit 2
fi

echo "[build] docker build -f $DOCKERFILE -t $IMAGE_NAME $BUILD_CONTEXT"
docker build -f "$DOCKERFILE" -t "$IMAGE_NAME" "$BUILD_CONTEXT"

### ========= APPLY MANIFEST (ns + httpbin + gateway vuoto) =========
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
YAML="${REPO_ROOT}/test/k8s/cb-e2e.yaml"

echo "[kubectl] apply $YAML"
kubectl apply -f "$YAML"

# Imposta l'immagine del gateway al TUO build
echo "[kubectl] set image deploy/$GATEWAY_SVC -> $IMAGE_NAME (ns=$NS)"
kubectl -n "$NS" set image "deploy/${GATEWAY_SVC}" "${GATEWAY_SVC}=${IMAGE_NAME}" --record

# Inietta le env CB (se il tuo servizio le supporta, perfetto; altrimenti vengono ignorate inoffensivamente)
kubectl -n "$NS" set env "deploy/${GATEWAY_SVC}" \
  "GATEWAY_ADDR=:${GATEWAY_PORT}" \
  "HTTP_TIMEOUT_MS=${HTTP_TIMEOUT_MS}" \
  "CB_FAILURES_TO_TRIP=${CB_FAILURES_TO_TRIP}" \
  "CB_OPEN_FOR_MS=${CB_OPEN_FOR_MS}" \
  "CB_HALF_OPEN_MAX=${CB_HALF_OPEN_MAX}" \
  >/dev/null

# Assicurati che l'upstream del gateway punti all'httpbin nel namespace isolato
# Se il tuo servizio accetta una env tipo UPSTREAM_URL la usiamo (altrimenti ignora)
kubectl -n "$NS" set env "deploy/${GATEWAY_SVC}" \
  "UPSTREAM_URL=http://httpbin.${NS}.svc.cluster.local:8080" >/dev/null || true

### ========= WAIT READY =========
echo "[kubectl] wait deployments (up to 3m)"
kubectl -n "$NS" wait --for=condition=Available "deploy/httpbin"  --timeout=3m
kubectl -n "$NS" wait --for=condition=Available "deploy/${GATEWAY_SVC}" --timeout=3m

# Service ready (endpoints)
echo "[kubectl] wait service endpoints"
for i in $(seq 1 60); do
  EP="$(kubectl -n "$NS" get endpoints "$GATEWAY_SVC" -o jsonpath='{.subsets[0].addresses[0].ip}' 2>/dev/null || true)"
  [ -n "$EP" ] && break
  sleep 1
done
kubectl -n "$NS" get svc "$GATEWAY_SVC" -o wide
kubectl -n "$NS" get endpoints "$GATEWAY_SVC" -o wide

### ========= TEST E2E: POD EFFIMERO CURL =========
GATEWAY_URL="http://${GATEWAY_SVC}.${NS}.svc.cluster.local:${GATEWAY_PORT}${GATEWAY_PATH}"

echo "[run] E2E contro IL TUO gateway reale in ns=$NS -> $GATEWAY_URL"
kubectl -n "$NS" run cbtest-curl --rm -i --restart=Never \
  --image=curlimages/curl:8.10.1 \
  --env "GATEWAY_URL=${GATEWAY_URL}" \
  --env "OPEN_FOR_MS=${CB_OPEN_FOR_MS}" \
  --env "SLOW_MS=${SLOW_MS}" \
  --command -- sh -lc '
set -e
echo "Gateway: $GATEWAY_URL  open_for_ms=$OPEN_FOR_MS"

run() {
  local url="$1"
  local out code t_ms cb
  out="$(curl -s -D - -o /dev/null -w "code=%{http_code} t=%{time_total}\n" "$url")"
  code="$(printf "%s" "$out" | awk "/^code=/{print \$1}" | cut -d= -f2)"
  t_ms="$(printf "%s" "$out" | awk "/^code=/{print \$2}" | cut -d= -f2 | awk "{printf(\"%d\",\$1*1000)}")"
  cb="$(printf "%s" "$out" | awk -F": " "/^X-CB-Outcome:/{print \$2}" | tr -d "\r")"
  [ -z "$cb" ] && cb="-"
  echo "$(date +%H:%M:%S) code=$code rt_ms=$t_ms cb=$cb"
}

echo "Warmup"
run "$GATEWAY_URL?mode=ok"

echo "S1: Trip by 5xx → expect Open + 503 fast"
for i in 1 2 3; do run "$GATEWAY_URL?mode=error"; done
echo "S1b: Immediately after (short_circuit atteso)"
run "$GATEWAY_URL?mode=ok"

echo "Cooldown (open_for_ms + 150ms)"
sleep $(awk -v ms="$OPEN_FOR_MS" "BEGIN{printf \"%.2f\",(ms/1000)+0.15}")

echo "Half-Open success → Closed"
run "$GATEWAY_URL?mode=ok"

echo "S3: Trip by TIMEOUTs (slow > gateway timeout)"
for i in 1 2 3; do run "$GATEWAY_URL?mode=slow&ms=$SLOW_MS"; done
echo "S3b: Immediately after (short_circuit atteso)"
run "$GATEWAY_URL?mode=ok"

echo "DONE"
'

echo "[done]"


# per eseguire:
# DOCKERFILE=internal/services/gateway/Dockerfile \
 #BUILD_CONTEXT=. \
 #IMAGE_NAME=gateway:cbtest \
 #test/scripts/cb_e2e_k8s.sh