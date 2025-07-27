#!/bin/bash

set -e

NAMESPACE="rabbitmq"

echo "Creating namespace '$NAMESPACE' (if not exists)..."
kubectl get ns $NAMESPACE || kubectl create ns $NAMESPACE

echo "Applying RabbitMQ cluster..."
kubectl apply -f deploy/rabbitmq/rabbitmq.yaml

echo "Applying RabbitMQ MQTT service..."
kubectl apply -f deploy/rabbitmq/rabbitmq-service.yaml

echo "Waiting for RabbitMQ pods to be ready..."
kubectl wait --for=condition=ready pod -l app.kubernetes.io/component=rabbitmq -n $NAMESPACE --timeout=180s

echo "RabbitMQ cluster is up"
