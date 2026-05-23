#!/bin/bash
# ============================================================
# PACS 雲端部署腳本 (GKE + Cloud SQL + Memorystore)
#
# 使用方式：
#   ./deploy-to-gke.sh <PROJECT_ID> [REGION] [CLUSTER_NAME]
#
# 前置條件：
#   - gcloud auth login && gcloud auth configure-docker
#   - 已啟用 API：container, sqladmin, redis, iam, cloudbuild
# ============================================================

set -euo pipefail

# ── 參數 ──────────────────────────────────────────────────────
PROJECT_ID=${1:-}
REGION=${2:-asia-east1}
CLUSTER_NAME=${3:-pacs-cluster}
DB_INSTANCE_NAME=${4:-pacs-pg16}
REDIS_NAME=${5:-pacs-redis}
DB_PASSWORD=${DB_PASSWORD:-$(openssl rand -base64 16)}
JWT_SECRET=${JWT_SECRET:-$(openssl rand -base64 32)}

if [ -z "$PROJECT_ID" ]; then
    echo "❌ 用法：$0 <PROJECT_ID> [REGION] [CLUSTER_NAME]"
    exit 1
fi

echo "╔════════════════════════════════════════════════════════╗"
echo "║         PACS GKE 部署啟動                               ║"
echo "╠════════════════════════════════════════════════════════╣"
echo "║ 專案  : $PROJECT_ID"
echo "║ 區域  : $REGION"
echo "║ 叢集  : $CLUSTER_NAME"
echo "╚════════════════════════════════════════════════════════╝"

gcloud config set project "$PROJECT_ID"

# ── 1. GKE 叢集 ───────────────────────────────────────────────
echo ""
echo "📦 [1/7] 確保 GKE 叢集存在..."
if ! gcloud container clusters describe "$CLUSTER_NAME" --region="$REGION" &>/dev/null; then
    gcloud container clusters create "$CLUSTER_NAME" \
        --region="$REGION" \
        --num-nodes=2 \
        --machine-type=e2-standard-2 \
        --enable-autoscaling \
        --min-nodes=2 \
        --max-nodes=6 \
        --workload-pool="$PROJECT_ID.svc.id.goog"
    echo "   ✅ 叢集建立完成"
else
    echo "   ✅ 叢集已存在"
    gcloud container clusters update "$CLUSTER_NAME" \
        --region="$REGION" \
        --workload-pool="$PROJECT_ID.svc.id.goog" 2>/dev/null || true
fi
gcloud container clusters get-credentials "$CLUSTER_NAME" --region="$REGION"

# ── 2. Cloud SQL ───────────────────────────────────────────────
echo ""
echo "📦 [2/7] 確保 Cloud SQL PostgreSQL 16 存在..."
if ! gcloud sql instances describe "$DB_INSTANCE_NAME" &>/dev/null; then
    gcloud sql instances create "$DB_INSTANCE_NAME" \
        --database-version=POSTGRES_16 \
        --tier=db-custom-2-7680 \
        --region="$REGION"
    echo "   ✅ SQL 實例建立完成"
else
    echo "   ✅ SQL 實例已存在"
fi
gcloud sql databases create pacs_db --instance="$DB_INSTANCE_NAME" 2>/dev/null || true
gcloud sql users create pacs_user --instance="$DB_INSTANCE_NAME" --password="$DB_PASSWORD" 2>/dev/null || \
    gcloud sql users set-password pacs_user --instance="$DB_INSTANCE_NAME" --password="$DB_PASSWORD"

# ── 3. Memorystore Redis ───────────────────────────────────────
echo ""
echo "📦 [3/7] 確保 Memorystore Redis 存在..."
if ! gcloud redis instances describe "$REDIS_NAME" --region="$REGION" &>/dev/null; then
    gcloud redis instances create "$REDIS_NAME" \
        --size=1 \
        --region="$REGION" \
        --redis-version=redis_7_0
    echo "   ✅ Redis 建立完成"
else
    echo "   ✅ Redis 已存在"
fi

CLOUD_SQL_CONN_NAME=$(gcloud sql instances describe "$DB_INSTANCE_NAME" --format="value(connectionName)")
REDIS_IP=$(gcloud redis instances describe "$REDIS_NAME" --region="$REGION" --format="value(host)")
REDIS_PORT=$(gcloud redis instances describe "$REDIS_NAME" --region="$REGION" --format="value(port)")

echo "   DB  : $CLOUD_SQL_CONN_NAME"
echo "   Redis: $REDIS_IP:$REDIS_PORT"

# ── 4. Workload Identity ───────────────────────────────────────
echo ""
echo "🛡️  [4/7] 設定 Workload Identity..."
GSA_NAME="pacs-db-accessor"
GSA_EMAIL="$GSA_NAME@$PROJECT_ID.iam.gserviceaccount.com"

if ! gcloud iam service-accounts describe "$GSA_EMAIL" &>/dev/null; then
    gcloud iam service-accounts create "$GSA_NAME" \
        --display-name="PACS DB Accessor"
fi
gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member="serviceAccount:$GSA_EMAIL" \
    --role="roles/cloudsql.client" \
    --condition=None 2>/dev/null || true

kubectl create namespace pacs --dry-run=client -o yaml | kubectl apply -f -
kubectl create serviceaccount pacs-sa --namespace=pacs --dry-run=client -o yaml | kubectl apply -f -
gcloud iam service-accounts add-iam-policy-binding "$GSA_EMAIL" \
    --role="roles/iam.workloadIdentityUser" \
    --member="serviceAccount:$PROJECT_ID.svc.id.goog[pacs/pacs-sa]" \
    --condition=None 2>/dev/null || true
kubectl annotate serviceaccount pacs-sa \
    --namespace=pacs \
    iam.gke.io/gcp-service-account="$GSA_EMAIL" \
    --overwrite

# ── 5. Docker Build & Push ─────────────────────────────────────
echo ""
echo "🐳 [5/7] 建立並推送 Docker Images..."
gcloud auth configure-docker --quiet

BACKEND_SERVICES=("access-api" "event-processor" "reporting-api" "anomaly-detector" "mv-refresher" "org-sync")
for svc in "${BACKEND_SERVICES[@]}"; do
    echo "   building pacs-$svc ..."
    docker build \
        --build-arg SERVICE="$svc" \
        -t "gcr.io/$PROJECT_ID/pacs-$svc:latest" \
        ./backend
    docker push "gcr.io/$PROJECT_ID/pacs-$svc:latest"
done

echo "   building pacs-frontend ..."
docker build -t "gcr.io/$PROJECT_ID/pacs-frontend:latest" ./frontend
docker push "gcr.io/$PROJECT_ID/pacs-frontend:latest"
echo "   ✅ 所有 Images 推送完成"

# ── 6. Secrets & ConfigMap ────────────────────────────────────
echo ""
echo "🔐 [6/7] 建立 Secrets 與 ConfigMap..."

# Secret（含 DB 密碼與 JWT secret）
kubectl create secret generic pacs-secrets \
    --namespace=pacs \
    --from-literal=DB_PASSWORD="$DB_PASSWORD" \
    --from-literal=JWT_SECRET="$JWT_SECRET" \
    --dry-run=client -o yaml | kubectl apply -f -

# ConfigMap（動態注入 Memorystore IP 與 Cloud SQL 連線名）
kubectl create configmap pacs-config \
    --namespace=pacs \
    --from-literal=DB_HOST="127.0.0.1" \
    --from-literal=DB_PORT="5432" \
    --from-literal=DB_USER="pacs_user" \
    --from-literal=DB_NAME="pacs_db" \
    --from-literal=REDIS_HOST="$REDIS_IP" \
    --from-literal=REDIS_PORT="$REDIS_PORT" \
    --from-literal=CLOUD_SQL_INSTANCE="$CLOUD_SQL_CONN_NAME" \
    --from-literal=PORT="8080" \
    --from-literal=LOG_LEVEL="info" \
    --dry-run=client -o yaml | kubectl apply -f -

# Migration SQL ConfigMap
kubectl create configmap pacs-migration-sql \
    --namespace=pacs \
    --from-file=scripts/migrations/ \
    --dry-run=client -o yaml | kubectl apply -f -

# Cloud seed SQL ConfigMap（90,000 人播種；登入 db-tools Pod 手動執行）
kubectl create configmap pacs-cloud-seed-sql \
    --namespace=pacs \
    --from-file=scripts/cloud_migrations/ \
    --dry-run=client -o yaml | kubectl apply -f -

# Seed generator ConfigMap（產 SQL 種子用；非壓測）
kubectl create configmap pacs-seed-gen-source \
    --namespace=pacs \
    --from-file=scripts/seed-generator/ \
    --dry-run=client -o yaml | kubectl apply -f -

# k6 壓測腳本 ConfigMap（即時 HTTP 壓測，對應 NFR-1/2）
if [ -d "scripts/k6-load-test" ]; then
    kubectl create configmap pacs-k6-scripts \
        --namespace=pacs \
        --from-file=scripts/k6-load-test/ \
        --dry-run=client -o yaml | kubectl apply -f -
fi

echo "   ✅ Secrets & ConfigMap 建立完成"

# ── 7. 部署 K8s 資源 ──────────────────────────────────────────
echo ""
echo "🚀 [7/7] 部署 Kubernetes Resources..."

# 輔助函數：替換 PROJECT_ID 佔位符後套用
apply_yaml() {
    sed "s|gcr\.io/PROJECT_ID|gcr.io/$PROJECT_ID|g" "$1" | kubectl apply -f -
}

apply_yaml k8s/00-namespace.yaml
# k8s/01-config.yaml 不在此套用；ConfigMap/Secret 已由上方步驟動態建立
apply_yaml k8s/03-access-api.yaml
apply_yaml k8s/04-reporting-api.yaml
apply_yaml k8s/05-processors.yaml
apply_yaml k8s/06-migrations.yaml
apply_yaml k8s/08-ingress.yaml
apply_yaml k8s/09-network-policy.yaml
apply_yaml k8s/10-pdb.yaml
apply_yaml k8s/11-frontend.yaml
apply_yaml k8s/12-db-tools.yaml
# k8s/02-redis.yaml 不部署（GKE 使用 Memorystore）
# k8s/07-k6-load-test.yaml 手動執行

echo ""
echo "⏳ 等待 Migration Job 完成（最多 3 分鐘）..."
kubectl wait --for=condition=complete job/pacs-migrations \
    --namespace=pacs --timeout=180s || \
    echo "⚠️  Migration Job 未在時限內完成，請手動確認：kubectl logs job/pacs-migrations -n pacs"

echo ""
echo "✅ 部署完成！"
echo ""
echo "━━━ 下一步 ━━━"
echo "1. 查看 Pod 狀態："
echo "   kubectl get pods -n pacs"
echo ""
echo "2. 取得 Ingress IP（可能需 2-3 分鐘才分配）："
echo "   kubectl get ingress pacs-ingress -n pacs"
echo ""
echo "3. 雲端大規模播種（90,000 人，手動執行）："
echo "   kubectl exec -it -n pacs pod/db-tools -c psql -- sh"
echo "   psql -v ON_ERROR_STOP=1 -f /cloud-seed/0104_cloud_seed.up.sql"
echo ""
echo "4. 壓力測試："
echo "   kubectl delete job -n pacs k6-shift-burst --ignore-not-found"
echo "   kubectl apply -f k8s/07-k6-load-test.yaml"
echo "   kubectl logs -f -n pacs job/k6-shift-burst"
echo ""
