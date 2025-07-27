
docker build -f ./internal/sensor-simulator/Dockerfile -t sensor-simulator:latest .
# Carica il file di configurazione condiviso
kubectl apply -f ./deploy/sensors/configMap/sensors-config-configmap.yaml

# Per ogni sensore
kubectl apply -f ./deploy/sensors/configMap/sensor-1-env-configMap.yaml
kubectl apply -f ./deploy/sensors/sensor-1.yaml

kubectl apply -f ./deploy/sensors/configMap/sensor-2-env-configMap.yaml
kubectl apply -f ./deploy/sensors/sensor-2.yaml

kubectl apply -f ./deploy/sensors/configMap/sensor-3-env-configMap.yaml
kubectl apply -f ./deploy/sensors/sensor-3.yaml

kubectl apply -f ./deploy/sensors/configMap/sensor-4-env-configMap.yaml
kubectl apply -f ./deploy/sensors/sensor-4.yaml