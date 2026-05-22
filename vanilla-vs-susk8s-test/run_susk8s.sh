#!/bin/bash
# run_susk8s.sh - Experimental Test

echo "1. Tearing down any existing clusters..."
kind delete cluster --name susk8s-demo

echo "2. Booting fresh Kubernetes cluster..."
kind create cluster --name susk8s-demo --config kind-config.yaml

echo "3. Deploying Observability Stack..."
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo add kepler https://sustainable-computing-io.github.io/kepler-helm-chart
helm repo update

helm upgrade --install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --set prometheus.prometheusSpec.serviceMonitorSelectorNilUsesHelmValues=false

helm upgrade --install kepler kepler/kepler \
  --namespace kepler --create-namespace \
  --set provider=kind

kubectl apply -f ../demo-yamls/kepler-svc.yaml
kubectl apply -f ../demo-yamls/kepler-sm.yaml
kubectl apply -f ../demo-yamls/grafana-dashboard.yaml

echo "Waiting for observability stack to spin up..."
sleep 60

echo "4. Deploying SusK8s Intelligence (Operator, Webhook, Scheduler)..."
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.4/cert-manager.yaml
echo "Waiting for Cert-Manager to initialize..."
sleep 30

kind load docker-image susk8s-operator:v1 --name susk8s-demo
kind load docker-image susk8s-scheduler:v1 --name susk8s-demo

make -C ../operator install
make -C ../operator deploy IMG=susk8s-operator:v1

kubectl apply -f ../demo-yamls/rescheduling-rbac.yaml
kubectl create clusterrolebinding operator-admin-bypass --clusterrole=cluster-admin --serviceaccount=operator-system:operator-controller-manager || true
kubectl rollout restart deployment/operator-controller-manager -n operator-system

kubectl rollout status deployment/operator-controller-manager -n operator-system
kubectl apply -f ../demo-yamls/susk8s-monitor.yaml

kubectl create configmap susk8s-scheduler-config -n kube-system \
  --from-file=scheduler-config.yaml=../demo-yamls/scheduler-config.yaml \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f ../demo-yamls/scheduler-deploy.yaml

echo "Waiting for Custom Scheduler to initialize..."
sleep 60

echo "5. Applying the Sustainability Policies and Data..."
kubectl apply -f ../demo-yamls/test-elecmaps-carbon.yaml
kubectl apply -f ../demo-yamls/policy.yaml

echo "6. Deploying the CPU Stress Workload..."
kubectl apply -f workload2.yaml

echo "7. Starting the EXACT SAME Weather Simulator..."
./weather.sh &

echo "=========================================================="
echo "SUSK8S RUN ACTIVE"
echo "Open Grafana in your browser."
echo "Wait for the weather script to finish in 15 minutes."
echo "=========================================================="