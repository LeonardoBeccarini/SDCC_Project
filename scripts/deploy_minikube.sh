#!/usr/bin/env bash
set -euo pipefail

# ========= Config =========
PROFILE="${PROFILE:-sdcc-cluster}"
K8S_VERSION="${K8S_VERSION:-stable}"

# Quanti worker vogliamo (tot = 5)
DESIRED_WORKERS="${DESIRED_WORKERS:-5}"

# ========= Paths =========
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
K8S_DIR="${K8S_DIR:-$ROOT_DIR/k8s}"

echo "==> Using K8S dir: $K8S_DIR"

# ========= Cluster (Minikube) =========
if command -v minikube >/dev/null 2>&1; then
  if ! minikube profile list 2>/dev/null | grep -q "^| $PROFILE "; then
    echo "==> Creating minikube profile $PROFILE"
    # Se vuoi dare risorse ai nodi, aggiungi qui --nodes=6 --cpus=2 --memory=2200
    minikube start -p "$PROFILE" --kubernetes-version="$K8S_VERSION"
  else
    echo "==> Reusing existing minikube profile $PROFILE"
    minikube -p "$PROFILE" status || true
  fi

  echo "==> Setting kube-context to '$PROFILE'"
  minikube update-context -p "$PROFILE"
  kubectl config use-context "$PROFILE"

  # Ensure desired number of workers
  echo "==> Ensuring $DESIRED_WORKERS worker nodes"
  # workers = nodi senza label control-plane
  get_worker_count() {
    kubectl get nodes -l '!node-role.kubernetes.io/control-plane' --no-headers 2>/dev/null | wc -l | xargs
  }
  cur_workers="$(get_worker_count || echo 0)"
  needed=$(( DESIRED_WORKERS - cur_workers ))
  if [ "$needed" -gt 0 ]; then
    for i in $(seq 1 "$needed"); do
      echo "   -> Adding worker $i/$needed"
      # Nota: su v1.36.0 non ci sono flag --cpus/--memory qui
      minikube -p "$PROFILE" node add --worker
    done
  else
    echo "   -> Already have $cur_workers workers"
  fi

  echo "==> Waiting for nodes to be Ready"
  kubectl wait node --all --for=condition=Ready --timeout=300s
fi

# ========= Label workers (1 broker, 2 edge, 1 fog, 1 sensors) =========
mapfile -t WORKERS < <(kubectl get nodes -l '!node-role.kubernetes.io/control-plane' -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' | sort)
if [ "${#WORKERS[@]}" -lt 5 ]; then
  echo "ERROR: need at least 5 worker nodes; found ${#WORKERS[@]}. Aborting."
  exit 1
fi

BROKER_NODE="${WORKERS[0]}"
EDGE1_NODE="${WORKERS[1]}"
EDGE2_NODE="${WORKERS[2]}"
FOG_NODE="${WORKERS[3]}"
SENS_NODE="${WORKERS[4]}"

echo "==> Assigning roles to workers:"
echo "   broker : $BROKER_NODE"
echo "   edge1  : $EDGE1_NODE"
echo "   edge2  : $EDGE2_NODE"
echo "   fog    : $FOG_NODE"
echo "   sensors: $SENS_NODE"

kubectl label node "$BROKER_NODE" role=rabbitmq  --overwrite
kubectl label node "$EDGE1_NODE"  role=edge1     --overwrite
kubectl label node "$EDGE2_NODE"  role=edge2     --overwrite
kubectl label node "$FOG_NODE"    role=fog       --overwrite
kubectl label node "$SENS_NODE"   role=sensors   --overwrite

# ========= Namespaces =========
echo "==> Applying namespaces..."
kubectl apply -f "$K8S_DIR/namespaces.yaml"

# ========= RabbitMQ (3 replicas, cluster) =========
echo "==> Deploying RabbitMQ (3 replicas, cluster)..."
kubectl apply -f "$K8S_DIR/rabbitmq/config-and-secrets.yaml"
kubectl apply -f "$K8S_DIR/rabbitmq/rbac.yaml"
kubectl apply -f "$K8S_DIR/rabbitmq/service-headless.yaml"
kubectl apply -f "$K8S_DIR/rabbitmq/service-internal.yaml"
kubectl apply -f "$K8S_DIR/rabbitmq/service-lb.yaml"
kubectl apply -f "$K8S_DIR/rabbitmq/statefulset.yaml"

# Pin RabbitMQ al nodo broker
echo "==> Pinning RabbitMQ to node role=rabbitmq"
kubectl -n rabbitmq patch statefulset rabbitmq -p \
'{"spec":{"template":{"spec":{"nodeSelector":{"role":"rabbitmq"}}}}}'

echo "==> Waiting for RabbitMQ StatefulSet to be Ready..."
kubectl -n rabbitmq rollout status statefulset/rabbitmq -w || true

echo "==> RabbitMQ services:"
kubectl -n rabbitmq get svc -o wide

# ========= (Opzionale) carica immagini locali =========
echo "==> (Optional) Loading local images into Minikube (ignore errors if not present)"
minikube -p "$PROFILE" image load sdcc/aggregator:local            || true
minikube -p "$PROFILE" image load sdcc/device-service:local        || true
minikube -p "$PROFILE" image load sdcc/irrigation-controller:local || true
minikube -p "$PROFILE" image load sdcc/sensor-simulator:local      || true

# ========= Base config (se presente) =========
if [[ -d "$K8S_DIR/config" ]]; then
  echo "==> Applying config/ ..."
  kubectl apply -R -f "$K8S_DIR/config"
fi

# ========= Workloads =========
for NSDIR in edge fog sensors; do
  if [[ -d "$K8S_DIR/$NSDIR" ]]; then
    echo "==> Applying $NSDIR ..."
    kubectl apply -R -f "$K8S_DIR/$NSDIR"
  fi
done

# Assicurati che i deployment vadano sui nodi giusti
echo "==> Ensuring nodeSelectors on deployments"

# EDGE
kubectl -n edge patch deploy/aggregator-1     -p '{"spec":{"template":{"spec":{"nodeSelector":{"role":"edge1"}}}}}'       || true
kubectl -n edge patch deploy/device-service-1 -p '{"spec":{"template":{"spec":{"nodeSelector":{"role":"edge1"}}}}}'       || true
kubectl -n edge patch deploy/aggregator-2     -p '{"spec":{"template":{"spec":{"nodeSelector":{"role":"edge2"}}}}}'       || true
kubectl -n edge patch deploy/device-service-2 -p '{"spec":{"template":{"spec":{"nodeSelector":{"role":"edge2"}}}}}'       || true

# FOG
kubectl -n fog  patch deploy/irrigation-controller -p '{"spec":{"template":{"spec":{"nodeSelector":{"role":"fog"}}}}}'     || true
kubectl -n fog  patch deploy/event-service         -p '{"spec":{"template":{"spec":{"nodeSelector":{"role":"fog"}}}}}'     || true

# SENSORS
kubectl -n sensors patch deploy/sensor-1 -p '{"spec":{"template":{"spec":{"nodeSelector":{"role":"sensors"}}}}}'           || true
kubectl -n sensors patch deploy/sensor-2 -p '{"spec":{"template":{"spec":{"nodeSelector":{"role":"sensors"}}}}}'           || true
kubectl -n sensors patch deploy/sensor-3 -p '{"spec":{"template":{"spec":{"nodeSelector":{"role":"sensors"}}}}}'           || true
kubectl -n sensors patch deploy/sensor-4 -p '{"spec":{"template":{"spec":{"nodeSelector":{"role":"sensors"}}}}}'           || true

echo "==> Waiting for key workloads..."
kubectl -n edge rollout status deploy/aggregator-1 -w || true
kubectl -n edge rollout status deploy/aggregator-2 -w || true
kubectl -n edge rollout status deploy/device-service-1 -w || true
kubectl -n edge rollout status deploy/device-service-2 -w || true
kubectl -n fog  rollout status deploy/irrigation-controller -w || true
kubectl -n fog  rollout status deploy/event-service -w || true
kubectl -n sensors rollout status deploy/sensor-1 -w || true
kubectl -n sensors rollout status deploy/sensor-2 -w || true
kubectl -n sensors rollout status deploy/sensor-3 -w || true
kubectl -n sensors rollout status deploy/sensor-4 -w || true

echo "==> Summary:"
kubectl get nodes -o wide
kubectl get pods -A -o wide
kubectl get svc -A -o wide

echo "Done."


#----------- PER ESEGUIRLO -----------
#chmod +x scripts/deploy_minikube.sh
#scripts/deploy_minikube.sh