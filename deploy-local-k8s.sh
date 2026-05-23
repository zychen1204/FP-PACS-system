#!/bin/bash
# Local Kubernetes deployment for PACS.
#
# This path does not create or use any GCP resources. It deploys Postgres,
# Redis, PACS services, db-tools, and k6 into the current local Kubernetes
# context. If the current context is a kind cluster, local images are loaded
# into kind automatically.

set -euo pipefail

NAMESPACE=${NAMESPACE:-pacs}
BUILD_IMAGES=${BUILD_IMAGES:-1}
RUN_MIGRATIONS=${RUN_MIGRATIONS:-1}

BACKEND_SERVICES=(
    access-api
    event-processor
    reporting-api
    anomaly-detector
    mv-refresher
    org-sync
)

require_cmd() {
    if ! command -v "$1" >/dev/null 2>&1; then
        echo "Missing required command: $1"
        exit 1
    fi
}

require_cmd docker
require_cmd kubectl

if ! kubectl config current-context >/dev/null 2>&1; then
    cat <<'EOF'
No Kubernetes context is currently set.

Use one of these local-only options first:
  1. Enable Kubernetes in Docker Desktop, then rerun this script.
  2. Install kind and create a cluster:
     kind create cluster --name pacs-local

No GCP PROJECT_ID is needed for this local path.
EOF
    exit 1
fi

CURRENT_CONTEXT=$(kubectl config current-context)

echo "Using Kubernetes context: ${CURRENT_CONTEXT}"

if [ "$BUILD_IMAGES" = "1" ]; then
    echo "Building local Docker images..."
    for svc in "${BACKEND_SERVICES[@]}"; do
        docker build --build-arg SERVICE="$svc" -t "pacs-$svc:local" ./backend
    done
    docker build -t pacs-frontend:local ./frontend
fi

if [[ "$CURRENT_CONTEXT" == kind-* ]]; then
    require_cmd kind
    KIND_CLUSTER=${CURRENT_CONTEXT#kind-}
    echo "Loading local images into kind cluster: ${KIND_CLUSTER}"
    for svc in "${BACKEND_SERVICES[@]}"; do
        kind load docker-image "pacs-$svc:local" --name "$KIND_CLUSTER"
    done
    kind load docker-image pacs-frontend:local --name "$KIND_CLUSTER"
fi

echo "Applying local namespace/config..."
kubectl apply -f k8s/local/00-namespace-config.yaml

echo "Creating SQL and k6 ConfigMaps..."
kubectl create configmap pacs-migration-sql \
    --namespace="$NAMESPACE" \
    --from-file=scripts/migrations/ \
    --dry-run=client -o yaml | kubectl apply -f -

kubectl create configmap pacs-cloud-seed-sql \
    --namespace="$NAMESPACE" \
    --from-file=scripts/cloud_migrations/ \
    --dry-run=client -o yaml | kubectl apply -f -

kubectl create configmap pacs-k6-scripts \
    --namespace="$NAMESPACE" \
    --from-file=scripts/k6-load-test/ \
    --from-file=scripts/k6-load-test/lib/ \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Deploying local Postgres and Redis..."
kubectl apply -f k8s/local/01-postgres-redis.yaml
kubectl wait --for=condition=Available -n "$NAMESPACE" deployment/postgres --timeout=180s
kubectl wait --for=condition=Available -n "$NAMESPACE" deployment/redis --timeout=180s

if [ "$RUN_MIGRATIONS" = "1" ]; then
    echo "Running migrations..."
    kubectl delete job -n "$NAMESPACE" pacs-migrations --ignore-not-found
    kubectl apply -f k8s/local/02-migrations.yaml
    kubectl wait --for=condition=complete -n "$NAMESPACE" job/pacs-migrations --timeout=240s
fi

echo "Deploying PACS services and db-tools..."
kubectl apply -f k8s/local/03-apps.yaml
kubectl apply -f k8s/local/04-db-tools.yaml

kubectl wait --for=condition=Available -n "$NAMESPACE" deployment/access-api --timeout=180s
kubectl wait --for=condition=Available -n "$NAMESPACE" deployment/reporting-api --timeout=180s
kubectl wait --for=condition=Ready -n "$NAMESPACE" pod/db-tools --timeout=120s

cat <<'EOF'

Local Kubernetes deployment is ready.

Seed PostgreSQL from inside the db-tools Pod:
  kubectl exec -it -n pacs pod/db-tools -c psql -- sh
  psql -v ON_ERROR_STOP=1 -f /cloud-seed/0104_cloud_seed.up.sql

Run k6 load test:
  kubectl delete job -n pacs k6-shift-burst --ignore-not-found
  kubectl apply -f k8s/local/05-k6-load-test.yaml
  kubectl logs -f -n pacs job/k6-shift-burst

Optional local UI:
  kubectl port-forward -n pacs svc/frontend 8088:80
  Open http://localhost:8088
EOF
