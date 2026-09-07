#!/usr/bin/env bash
# Builds both images, loads them into a local kind cluster, and applies
# every Kubernetes manifest. Zero cost, zero registry — everything stays on
# this machine. See infra/kubernetes/README.md for first-time cluster setup
# (kind create cluster, ingress-nginx, secrets.yaml).
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
CLUSTER_NAME="${KIND_CLUSTER_NAME:-re-api-books}"
NAMESPACE="${NAMESPACE:-re-api-books-dev}"

command -v kind >/dev/null 2>&1 || { echo "kind não encontrado — https://kind.sigs.k8s.io/docs/user/quick-start/#installation" >&2; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo "kubectl não encontrado." >&2; exit 1; }

if ! kind get clusters | grep -qx "${CLUSTER_NAME}"; then
  echo "Cluster kind '${CLUSTER_NAME}' não existe. Criando..."
  kind create cluster --name "${CLUSTER_NAME}" --config "${ROOT_DIR}/infra/kubernetes/kind-config.yaml"

  echo "Instalando ingress-nginx (build \"kind\" do controller, free/OSS)..."
  kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.13.0/deploy/static/provider/kind/deploy.yaml
  kubectl wait --namespace ingress-nginx \
    --for=condition=ready pod \
    --selector=app.kubernetes.io/component=controller \
    --timeout=120s
fi

"${ROOT_DIR}/infra/cicd/scripts/build.sh"

echo "Loading images into kind..."
kind load docker-image movies-service:latest api-gateway:latest --name "${CLUSTER_NAME}"

echo "Applying manifests..."
kubectl apply -f "${ROOT_DIR}/infra/kubernetes/namespace.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/configmap.yaml"

if [ -f "${ROOT_DIR}/infra/kubernetes/secrets.yaml" ]; then
  kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/secrets.yaml"
else
  echo "AVISO: infra/kubernetes/secrets.yaml não existe (copie de secrets.yaml.example)." >&2
  echo "Os deployments abaixo vão falhar até esse Secret existir." >&2
fi

kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/mongodb-pvc.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/mongodb-deployment.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/mongodb-service.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/rabbitmq-deployment.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/rabbitmq-service.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/backend-movies-service-deployment.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/backend-movies-service-service.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/backend-api-gateway-deployment.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/backend-api-gateway-service.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/backend-hpa.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/ingress.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/monitoring/prometheus.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/kubernetes/monitoring/grafana.yaml"
kubectl apply -n "${NAMESPACE}" -f "${ROOT_DIR}/infra/security/network-policies.yaml"

echo "Done. kubectl -n ${NAMESPACE} get pods"
