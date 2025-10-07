#!/usr/bin/env bash
set -euo pipefail

# ========= Config =========
PROFILE="${PROFILE:-sdcc-cluster}"
K8S_VERSION="${K8S_VERSION:-v1.30.5}"
NODES="${NODES:-6}"   # 1 control-plane + 5 workers
CPUS="${CPUS:-4}"
MEMORY="${MEMORY:-2200}"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
K8S_DIR="${K8S_DIR:-$ROOT_DIR/k8s}"

echo "==> Using profile: $PROFILE"
echo "==> Using K8S dir: $K8S_DIR"

# ========= Pre-flight =========
command -v minikube >/dev/null || { echo "minikube not found"; exit 1; }
command -v kubectl >/dev/null || { echo "kubectl not found"; exit 1; }
command -v docker  >/dev/null || { echo "docker not found"; exit 1; }

# ========= Start minikube =========
if ! minikube -p "$PROFILE" status >/dev/null 2>&1; then
  echo "==> Starting minikube profile '$PROFILE' with $NODES nodes..."
   minikube start -p "$PROFILE" --kubernetes-version="$K8S_VERSION" --nodes="$NODES" --cpus="$CPUS" --memory="$MEMORY"
else
  echo "==> Minikube profile '$PROFILE' already running"
fi

# ========= Label nodes (per i nodeSelector dei manifest) =========
echo "==> Labeling nodes"
kubectl wait --for=condition=Ready node --all --timeout=120s >/dev/null

mapfile -t nodes < <(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' \
  --sort-by='.metadata.name' -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')

(( ${#nodes[@]} == 5 )) || { echo "Servono esattamente 5 worker (trovati ${#nodes[@]})."; exit 1; }

kubectl label node "${nodes[0]}" role=rabbitmq --overwrite
kubectl label node "${nodes[1]}" role=sensors  --overwrite
kubectl label node "${nodes[2]}" role=fog      --overwrite
kubectl label node "${nodes[3]}" role=edge1 --overwrite;
kubectl label node "${nodes[4]}" role=edge2 --overwrite;

# ========= Build images (host docker) + load into all nodes =========
echo "==> Building images locally"
docker build --no-cache -t sdcc/sensor-simulator:local        -f "$ROOT_DIR/internal/sensor-simulator/Dockerfile" "$ROOT_DIR"
docker build --no-cache -t sdcc/aggregator:local              -f "$ROOT_DIR/internal/services/aggregator/Dockerfile" "$ROOT_DIR"
docker build --no-cache -t sdcc/device-service:local          -f "$ROOT_DIR/internal/services/device/Dockerfile" "$ROOT_DIR"
docker build --no-cache -t sdcc/irrigation-controller:local   -f "$ROOT_DIR/internal/services/irrigation-controller/Dockerfile" "$ROOT_DIR"
docker build --no-cache -t sdcc/event-service:local  -f "$ROOT_DIR/internal/services/event/Dockerfile" "$ROOT_DIR"

echo "==> Loading images into all minikube nodes"
for img in \
  sdcc/sensor-simulator:local \
  sdcc/aggregator:local \
  sdcc/device-service:local \
  sdcc/irrigation-controller:local \
  sdcc/event-service:local
do
  minikube -p "$PROFILE" image load "$img"
done

# ========= Apply manifests =========
kubectl apply -f "$K8S_DIR/namespaces.yaml"
kubectl apply -n rabbitmq -f k8s/monitoring/manifests/rabbitmq-enabled-plugins-cm.yaml

# RabbitMQ (singola replica)
kubectl apply -f "$K8S_DIR/rabbitmq/config-and-secrets.yaml"
kubectl apply -f "$K8S_DIR/rabbitmq/service-headless.yaml"
kubectl apply -f "$K8S_DIR/rabbitmq/service-internal.yaml"
kubectl apply -f "$K8S_DIR/rabbitmq/statefulset.yaml"

echo "==> Waiting for RabbitMQ (single replica) ..."
kubectl -n rabbitmq rollout status statefulset/rabbitmq -w


# Config comuni + servizi
kubectl apply -f "$K8S_DIR/config"
kubectl apply -f "$K8S_DIR/edge"
kubectl apply -f "$K8S_DIR/fog"
kubectl apply -f "$K8S_DIR/sensors"

# ========= Wait for rollouts =========
echo "==> Waiting for deployments to become ready"
for ns in sensors edge fog; do
  for d in $(kubectl -n "$ns" get deploy -o name | awk -F/ '{print $2}'); do
    kubectl -n "$ns" rollout status deploy/"$d" -w || true
  done
done

# ========= Summary =========
echo "==> Summary"
kubectl get nodes -o wide
kubectl get pods -A -o wide
kubectl get svc -A -o wide

# ========= Endpoints =========
MINIKUBE_IP=$(minikube -p "$PROFILE" ip)
echo ""
echo "RabbitMQ MQTT  : $MINIKUBE_IP:31883  (user: mqtt_user, pass: mqtt_pwd)"
echo "RabbitMQ AMQP  : $MINIKUBE_IP:30672"
echo "RabbitMQ Mgmt  : http://$MINIKUBE_IP:31672"
echo ""


kubectl apply -f ./k8s/rabbitmq/ngrok-secret.yaml
kubectl apply -f ./k8s/rabbitmq/ngrok-config.yaml
kubectl apply -f ./k8s/rabbitmq/ngrok-deploy.yaml

# per prendere indirizzo e porta esposte di rabbitmq
kubectl -n rabbitmq logs deploy/ngrok-mqtt | grep -Eo 'tcp://[^ ]+' | tail -n1
# Estrazione automatica dell'URL dal log
URL=$(kubectl -n fog logs deploy/event-quick-tunnel | grep -m1 -o 'https://[^ ]*trycloudflare.com')

# Controllo se l'URL è stato estratto correttamente
if [ -z "$URL" ]; then
  echo "Errore: URL non trovato."
  exit 1
fi

# Mostra l'URL estratto
echo "URL estratto: $URL"


echo "Done."

#----------- PER ESEGUIRLO -----------
#chmod +x scripts/deploy_minikube.sh
#scripts/deploy_minikube.sh
#minikube delete --all --purge per eliminare tutti i cluster su minikube