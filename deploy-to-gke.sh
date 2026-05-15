#!/bin/bash
# ============================================================
# PACS 雲端部署腳本 (GKE + Cloud SQL + Memorystore)
#
# 此腳本會自動：
# 1. 建立 GKE 叢集、Cloud SQL (PostgreSQL 16)、Memorystore (Redis)
# 2. 建立所需的 K8s Secrets 與 ConfigMaps
# 3. 處理 Workload Identity 權限綁定（讓 Cloud SQL Proxy 可以連線）
# 4. 把 scripts/migrations/ 內的 SQL 打包成 ConfigMap 供 migrate Job 使用
# 5. 部署所有 K8s resources
#
# 前置條件：
# - 已經登入 gcloud (gcloud auth login)
# - 已經啟用對應 API (container.googleapis.com, sqladmin.googleapis.com, redis.googleapis.com, iam.googleapis.com)
#
# 使用方式：
#   ./deploy-to-gke.sh <PROJECT_ID> [REGION] [CLUSTER_NAME] [DB_INSTANCE_NAME] [REDIS_NAME]
# ============================================================

set -e

# ── 1. 讀取配置與參數 ──────────────────────────────────────
PROJECT_ID=${1}
REGION=${2:-asia-east1}
CLUSTER_NAME=${3:-pacs-cluster}
DB_INSTANCE_NAME=${4:-pacs-pg16}
REDIS_NAME=${5:-pacs-redis}
DB_PASSWORD=${DB_PASSWORD:-$(openssl rand -base64 16)} # 若沒設定環境變數，自動生成一組密碼
JWT_SECRET=${JWT_SECRET:-$(openssl rand -base64 32)}

if [ -z "$PROJECT_ID" ]; then
    echo "❌ 錯誤：請提供 GCP Project ID。"
    echo "使用方式：$0 <PROJECT_ID> [REGION]"
    exit 1
fi

echo "╔════════════════════════════════════════════════════════════════╗"
echo "║             PACS 部署到 GKE 啟動                               ║"
echo "╠════════════════════════════════════════════════════════════════╣"
echo "║ 專案 ID   : $PROJECT_ID"
echo "║ 區域      : $REGION"
echo "║ 叢集      : $CLUSTER_NAME"
echo "║ DB 實例   : $DB_INSTANCE_NAME"
echo "║ Redis     : $REDIS_NAME"
echo "╚════════════════════════════════════════════════════════════════╝"
echo ""

# 設定 gcloud 專案
gcloud config set project "$PROJECT_ID"

# ── 2. 建立基礎設施 (若已存在則跳過) ──────────────────────────

echo "📦 [1/7] 確保 GKE 叢集存在 (包含 Workload Identity)..."
if ! gcloud container clusters describe "$CLUSTER_NAME" --region="$REGION" &>/dev/null; then
    gcloud container clusters create "$CLUSTER_NAME" \
        --region="$REGION" \
        --num-nodes=3 \
        --machine-type=e2-standard-4 \
        --enable-autoscaling \
        --min-nodes=2 \
        --max-nodes=10 \
        --workload-pool="$PROJECT_ID.svc.id.goog"
else
    echo "   ✅ 叢集已存在"
    # 確保開啟 Workload Identity
    gcloud container clusters update "$CLUSTER_NAME" \
        --region="$REGION" \
        --workload-pool="$PROJECT_ID.svc.id.goog" || true
fi

echo "🔑 取得叢集憑證..."
gcloud container clusters get-credentials "$CLUSTER_NAME" --region="$REGION"

echo "📦 [2/7] 確保 Cloud SQL PostgreSQL 實例存在..."
if ! gcloud sql instances describe "$DB_INSTANCE_NAME" &>/dev/null; then
    gcloud sql instances create "$DB_INSTANCE_NAME" \
        --database-version=POSTGRES_16 \
        --tier=db-custom-4-15360 \
        --region="$REGION" \
        --root-password="$DB_PASSWORD"
else
    echo "   ✅ Cloud SQL 實例已存在"
fi

# 確保 DB 與 User 存在
echo "   建立/更新資料庫與使用者..."
gcloud sql databases create pacs_db --instance="$DB_INSTANCE_NAME" || true
gcloud sql users create pacs_user --instance="$DB_INSTANCE_NAME" --password="$DB_PASSWORD" || true

echo "📦 [3/7] 確保 Memorystore Redis 存在..."
if ! gcloud redis instances describe "$REDIS_NAME" --region="$REGION" &>/dev/null; then
    gcloud redis instances create "$REDIS_NAME" \
        --size=5 \
        --region="$REGION" \
        --redis-version=redis_7_0
else
    echo "   ✅ Redis 已存在"
fi

# 取得 DB Connection Name 與 Redis IP
CLOUD_SQL_CONN_NAME=$(gcloud sql instances describe "$DB_INSTANCE_NAME" --format="value(connectionName)")
REDIS_IP=$(gcloud redis instances describe "$REDIS_NAME" --region="$REGION" --format="value(host)")
REDIS_PORT=$(gcloud redis instances describe "$REDIS_NAME" --region="$REGION" --format="value(port)")

echo "   🔹 DB Connection Name: $CLOUD_SQL_CONN_NAME"
echo "   🔹 Redis Endpoint: $REDIS_IP:$REDIS_PORT"


# ── 3. 設定 Workload Identity ────────────────────────────────

echo "🛡️ [4/7] 設定 Workload Identity (授權 Cloud SQL Proxy)..."

# 建立 GCP Service Account (如果沒有)
GSA_NAME="pacs-db-accessor"
if ! gcloud iam service-accounts describe "$GSA_NAME@$PROJECT_ID.iam.gserviceaccount.com" &>/dev/null; then
    gcloud iam service-accounts create "$GSA_NAME" --display-name="PACS DB Accessor"
    
    # 賦予 Cloud SQL Client 權限
    gcloud projects add-iam-policy-binding "$PROJECT_ID" \
        --member="serviceAccount:$GSA_NAME@$PROJECT_ID.iam.gserviceaccount.com" \
        --role="roles/cloudsql.client"
fi

# 建立 K8s Namespace
kubectl create namespace pacs --dry-run=client -o yaml | kubectl apply -f -

# 建立 K8s ServiceAccount
kubectl create serviceaccount pacs-sa --namespace=pacs --dry-run=client -o yaml | kubectl apply -f -

# 綁定 GSA 與 KSA
gcloud iam service-accounts add-iam-policy-binding \
    "$GSA_NAME@$PROJECT_ID.iam.gserviceaccount.com" \
    --role="roles/iam.workloadIdentityUser" \
    --member="serviceAccount:$PROJECT_ID.svc.id.goog[pacs/pacs-sa]" \
    --condition=None

# 標註 KSA
kubectl annotate serviceaccount pacs-sa \
    --namespace=pacs \
    iam.gke.io/gcp-service-account="$GSA_NAME@$PROJECT_ID.iam.gserviceaccount.com" \
    --overwrite


# ── 4. 準備 ConfigMap 與 Secrets ─────────────────────────────

echo "🔐 [5/7] 建立 ConfigMap 與 Secrets..."

# 密碼與機密設定
kubectl create secret generic pacs-secrets \
    --namespace=pacs \
    --from-literal=DB_PASSWORD="$DB_PASSWORD" \
    --from-literal=JWT_SECRET="$JWT_SECRET" \
    --dry-run=client -o yaml | kubectl apply -f -

# 建立主設定 ConfigMap (包含動態抓取的 Redis IP 與 Cloud SQL 連線字串)
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: pacs-config
  namespace: pacs
data:
  DB_USER: "pacs_user"
  DB_NAME: "pacs_db"
  DB_PORT: "5432"
  CLOUD_SQL_INSTANCE: "$CLOUD_SQL_CONN_NAME"
  REDIS_HOST: "$REDIS_IP"
  REDIS_PORT: "$REDIS_PORT"
  PORT: "8080"
  LOG_LEVEL: "info"
EOF

# 把 migrations 目錄打包成 ConfigMap (供 migrate Job initContainer 使用)
echo "   打包 SQL migrations..."
kubectl create configmap pacs-migration-sql \
    --namespace=pacs \
    --from-file=scripts/migrations/ \
    --dry-run=client -o yaml | kubectl apply -f -

# 把 load-generator 目錄打包成 ConfigMap (供 load-tester 使用)
echo "   打包 load-generator 源碼..."
kubectl create configmap pacs-load-gen-source \
    --namespace=pacs \
    --from-file=scripts/load-generator/ \
    --dry-run=client -o yaml | kubectl apply -f -


# ── 5. 部署 K8s 資源 ─────────────────────────────────────────

echo "🚀 [6/7] 部署 Kubernetes Resources..."

# 順序部署
kubectl apply -f k8s/00-namespace.yaml    # （雖然前面已建，確保 role binding 正確）
kubectl apply -f k8s/01-config.yaml       # （可選，若有其他基礎設定）
# k8s/02-redis.yaml 不需要，因為雲端我們直接用 Memorystore
kubectl apply -f k8s/03-access-api.yaml
kubectl apply -f k8s/04-reporting-api.yaml
kubectl apply -f k8s/05-processors.yaml
kubectl apply -f k8s/06-migrations.yaml
kubectl apply -f k8s/08-ingress.yaml      # 包含 Ingress 設定
kubectl apply -f k8s/09-network-policy.yaml
kubectl apply -f k8s/10-pdb.yaml

echo "⏳ 等待 Migration Job 完成..."
kubectl wait --for=condition=complete job/pacs-migrations --namespace=pacs --timeout=120s || true


# ── 6. 完成與說明 ────────────────────────────────────────────

echo "✅ [7/7] 部署流程執行完畢！"
echo ""
echo "🎉 恭喜！PACS 系統已部署至 GKE。"
echo ""
echo "下一步 / 壓力測試指南："
echo "1. 檢查 Pod 狀態："
echo "   kubectl get pods -n pacs"
echo ""
echo "2. 確認 Ingress IP (可能需要幾分鐘才會分配 IP)："
echo "   kubectl get ingress pacs-ingress -n pacs"
echo ""
echo "3. 執行 90,000 人雲端播種 (需要連線到 Cloud SQL)："
echo "   # 啟動臨時的 psql Pod"
echo "   kubectl run psql-seeder --rm -it --image=postgres:16-alpine -n pacs --env=\"PGPASSWORD=\$DB_PASSWORD\" -- sh"
echo "   # 在 Pod 內執行 (這裡使用 06-migrations 內的 proxy IP 或 Memorystore 同 Vpc 下的連線方式)："
echo "   # ps. 可以利用已部署好的 access-api pod 的 shell 執行"
echo "   kubectl exec -it deployment/access-api -c access-api -n pacs -- sh"
echo "   # 然後在裡面執行："
echo "   apk add postgresql-client"
echo "   psql -h 127.0.0.1 -U pacs_user -d pacs_db -f /migrations/0104_cloud_seed.up.sql"
echo ""
echo "4. 執行壓力測試："
echo "   kubectl apply -f k8s/07-load-tester.yaml"
echo "   kubectl logs -f load-tester -n pacs"
echo ""
