#!/bin/bash
# run_vanilla.sh - Control Test

echo "1. Tearing down any existing clusters..."
kind delete cluster --name vanilla-demo

echo "2. Booting fresh Vanilla Kubernetes cluster..."
kind create cluster --name vanilla-demo --config kind-config.yaml

echo "3. Deploying Observability Stack (Kepler, Prometheus, Grafana)..."
# Add and update Helm Repos
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add kepler https://sustainable-computing-io.github.io/kepler-helm-chart
helm repo update

# Install Prometheus via Helm
helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false

# Install Kepler via Helm
helm upgrade --install kepler kepler/kepler \
  --namespace kepler --create-namespace \
  --set provider=kind

# Apply specific Kepler Services and Grafana Dashboard
kubectl apply -f ../demo-yamls/kepler-svc.yaml
kubectl apply -f ../demo-yamls/kepler-sm.yaml
kubectl apply -f ../demo-yamls/grafana-dashboard.yaml

echo "Waiting for observability stack to spin up..."
sleep 60

echo "4. Deploying Operator (METRICS ONLY - No Enforcement)"
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml
echo "Waiting for Cert-Manager to initialise..."
sleep 30

kind load docker-image susk8s-operator:v1 --name vanilla-demo
make -C ../operator install
make -C ../operator deploy IMG=susk8s-operator:v1

kubectl apply -f ../demo-yamls/rescheduling-rbac.yaml
kubectl create clusterrolebinding operator-admin-bypass --clusterrole=cluster-admin --serviceaccount=operator-system:operator-controller-manager || true
kubectl rollout restart deployment/operator-controller-manager -n operator-system

kubectl rollout status deployment/operator-controller-manager -n operator-system
kubectl apply -f ../demo-yamls/susk8s-monitor.yaml

echo "5. Deploying the CPU Stress Workload..."
kubectl apply -f ../demo-yamls/test-elecmaps-carbon.yaml
kubectl apply -f workload2.yaml

echo "6. Starting the Weather Simulator in the background..."
./weather.sh &

echo "=========================================================="
echo "VANILLA RUN ACTIVE"
echo "Open Grafana in browser."
echo "Wait for the weather script to finish in 15 minutes."
echo "=========================================================="