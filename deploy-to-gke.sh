#!/bin/bash
# ============================================================
# PACS 雲端部署腳本 (GKE + Cloud SQL + Memorystore)
#
# 使用方式：
#   ./deploy-to-gke.sh <PROJECT_ID> [REGION] [CLUSTER_NAME]
#
# 前置條件：
#   - gcloud auth login
#   - Docker daemon 已啟動
#   - 專案 Billing 已啟用
# ============================================================

set -euo pipefail

# ── 參數 ──────────────────────────────────────────────────────
PROJECT_ID=${1:-}
REGION=${2:-asia-east1}
CLUSTER_NAME=${3:-pacs-cluster}
DB_INSTANCE_NAME=${4:-pacs-pg16}
REDIS_NAME=${5:-pacs-redis}
DB_EDITION=${DB_EDITION:-ENTERPRISE}
DB_TIER=${DB_TIER:-db-custom-2-7680}
GKE_NUM_NODES=${GKE_NUM_NODES:-1}
GKE_MIN_NODES=${GKE_MIN_NODES:-1}
GKE_MAX_NODES=${GKE_MAX_NODES:-3}
GKE_MACHINE_TYPE=${GKE_MACHINE_TYPE:-e2-standard-2}
GKE_DISK_TYPE=${GKE_DISK_TYPE:-pd-standard}
GKE_DISK_SIZE=${GKE_DISK_SIZE:-30}
DB_PASSWORD=${DB_PASSWORD:-$(openssl rand -base64 16)}
JWT_SECRET=${JWT_SECRET:-$(openssl rand -base64 32)}
BUILD_IMAGES=${BUILD_IMAGES:-1}
DOMAIN_NAME=${DOMAIN_NAME:-}
ENABLE_HTTPS=${ENABLE_HTTPS:-}
STATIC_IP_NAME=${STATIC_IP_NAME:-pacs-ingress-ip}
WAIT_FOR_CERT=${WAIT_FOR_CERT:-0}
CERT_WAIT_TIMEOUT=${CERT_WAIT_TIMEOUT:-3600}

if [ -n "$DOMAIN_NAME" ] && [ -z "$ENABLE_HTTPS" ]; then
    ENABLE_HTTPS=1
fi
ENABLE_HTTPS=${ENABLE_HTTPS:-0}

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
echo "║ 節點  : ${GKE_NUM_NODES}/zone, ${GKE_MACHINE_TYPE}, ${GKE_DISK_SIZE}GB ${GKE_DISK_TYPE}"
echo "║ HTTPS : ${ENABLE_HTTPS}"
if [ "$ENABLE_HTTPS" = "1" ]; then
    echo "║ 網域  : ${DOMAIN_NAME}"
    echo "║ 靜態IP: ${STATIC_IP_NAME}"
    echo "║ 等憑證: ${WAIT_FOR_CERT}"
fi
echo "╚════════════════════════════════════════════════════════╝"

if [ "$ENABLE_HTTPS" = "1" ] && [ -z "$DOMAIN_NAME" ]; then
    echo "❌ ENABLE_HTTPS=1 時必須設定 DOMAIN_NAME，例如："
    echo "   DOMAIN_NAME=pacs.example.com make gke-deploy"
    exit 1
fi

gcloud config set project "$PROJECT_ID"

# ── 0. 必要 API ────────────────────────────────────────────────
echo ""
echo "🔧 [0/7] 啟用必要 Google Cloud APIs..."
gcloud services enable \
    container.googleapis.com \
    sqladmin.googleapis.com \
    redis.googleapis.com \
    iam.googleapis.com \
    cloudbuild.googleapis.com \
    containerregistry.googleapis.com \
    compute.googleapis.com \
    serviceusage.googleapis.com \
    --project="$PROJECT_ID"
echo "   ✅ APIs 已啟用或原本已啟用"

if [ "$ENABLE_HTTPS" = "1" ]; then
    echo ""
    echo "🌐 [0.5/7] 確保 HTTPS Ingress 使用的 global static IP 存在..."
    if ! gcloud compute addresses describe "$STATIC_IP_NAME" --global &>/dev/null; then
        gcloud compute addresses create "$STATIC_IP_NAME" --global
        echo "   ✅ Static IP 建立完成"
    else
        echo "   ✅ Static IP 已存在"
    fi
    STATIC_IP_ADDRESS=$(gcloud compute addresses describe "$STATIC_IP_NAME" --global --format="value(address)")
    echo "   Static IP: $STATIC_IP_ADDRESS"
    echo "   請確認 DNS A record 已指向此 IP：${DOMAIN_NAME} -> ${STATIC_IP_ADDRESS}"
    if command -v getent >/dev/null 2>&1; then
        DNS_IPS=$(getent ahostsv4 "$DOMAIN_NAME" | awk '{print $1}' | sort -u | tr '\n' ' ' || true)
        if [ -z "$DNS_IPS" ]; then
            echo "   ⚠️  目前查不到 ${DOMAIN_NAME} 的 A record。請先到 DNS provider 新增 A record。"
        elif [[ " $DNS_IPS " == *" $STATIC_IP_ADDRESS "* ]]; then
            echo "   ✅ DNS 已指向 static IP：$DNS_IPS"
        else
            echo "   ⚠️  DNS 目前是：$DNS_IPS"
            echo "      需要改成：${DOMAIN_NAME} -> ${STATIC_IP_ADDRESS}"
        fi
    fi
fi

# ── 1. GKE 叢集 ───────────────────────────────────────────────
echo ""
echo "📦 [1/7] 確保 GKE 叢集存在..."
if ! gcloud container clusters describe "$CLUSTER_NAME" --region="$REGION" &>/dev/null; then
    gcloud container clusters create "$CLUSTER_NAME" \
        --region="$REGION" \
        --num-nodes="$GKE_NUM_NODES" \
        --machine-type="$GKE_MACHINE_TYPE" \
        --disk-type="$GKE_DISK_TYPE" \
        --disk-size="$GKE_DISK_SIZE" \
        --enable-autoscaling \
        --min-nodes="$GKE_MIN_NODES" \
        --max-nodes="$GKE_MAX_NODES" \
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
        --edition="$DB_EDITION" \
        --tier="$DB_TIER" \
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
if [ "$BUILD_IMAGES" = "1" ]; then
    if ! command -v docker >/dev/null 2>&1; then
        echo "❌ 找不到 docker。請安裝 Docker Desktop，並開啟 WSL Integration。"
        echo "   如果 images 已經推送過，可以改用：make gke-deploy-no-build"
        exit 1
    fi
    if ! docker info >/dev/null 2>&1; then
        echo "❌ Docker 在目前 shell 無法使用。"
        echo "   WSL 解法：啟動 Docker Desktop，並到 Settings > Resources > WSL Integration 啟用此 distro。"
        echo "   如果 images 已經存在，可以改用：make gke-deploy-no-build"
        exit 1
    fi

    export DOCKER_CONFIG=${DOCKER_CONFIG:-/tmp/pacs-docker-config}
    mkdir -p "$DOCKER_CONFIG"
    if ! gcloud auth print-access-token | docker login -u oauth2accesstoken --password-stdin https://gcr.io >/dev/null; then
        echo "❌ Docker 登入 gcr.io 失敗。請確認 gcloud 已登入，且帳號有推送 Container Registry 的權限。"
        exit 1
    fi

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
else
    echo "   ⏭️  BUILD_IMAGES=0，略過 Docker build/push"
fi

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
    --from-file=scripts/seed-generator/main.go \
    --from-file=scripts/seed-generator/realistic-simulator.go \
    --from-file=scripts/seed-generator/go.mod \
    --dry-run=client -o yaml | kubectl apply -f -

# k6 壓測腳本 ConfigMap（即時 HTTP 壓測，對應 NFR-1/2）
if [ -d "scripts/k6-load-test" ]; then
    kubectl create configmap pacs-k6-scripts \
        --namespace=pacs \
        --from-file=scripts/k6-load-test/ \
        --from-file=scripts/k6-load-test/lib/ \
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

apply_https_ingress() {
    sed \
        -e "s|DOMAIN_NAME|$DOMAIN_NAME|g" \
        -e "s|STATIC_IP_NAME|$STATIC_IP_NAME|g" \
        k8s/08-ingress-https.yaml | kubectl apply -f -
}

apply_yaml k8s/00-namespace.yaml
# k8s/01-config.yaml 不在此套用；ConfigMap/Secret 已由上方步驟動態建立
apply_yaml k8s/03-access-api.yaml
apply_yaml k8s/04-reporting-api.yaml
apply_yaml k8s/05-processors.yaml
apply_yaml k8s/06-migrations.yaml
if [ "$ENABLE_HTTPS" = "1" ]; then
    apply_https_ingress
else
    apply_yaml k8s/08-ingress.yaml
fi

apply_yaml k8s/09-network-policy.yaml
apply_yaml k8s/10-pdb.yaml
apply_yaml k8s/11-frontend.yaml
apply_yaml k8s/12-db-tools.yaml
# k8s/02-redis.yaml 不部署（GKE 使用 Memorystore）
# k8s/07-k6-load-test.yaml 手動執行

if [ "$ENABLE_HTTPS" = "1" ] && [ "$WAIT_FOR_CERT" = "1" ]; then
    echo ""
    echo "⏳ 等待 ManagedCertificate Active（最多 ${CERT_WAIT_TIMEOUT} 秒）..."
    deadline=$((SECONDS + CERT_WAIT_TIMEOUT))
    while true; do
        CERT_STATUS=$(kubectl get managedcertificate pacs-managed-cert \
            --namespace=pacs \
            -o jsonpath='{.status.certificateStatus}' 2>/dev/null || true)
        echo "   CertificateStatus: ${CERT_STATUS:-<not ready>}"
        if [ "$CERT_STATUS" = "Active" ]; then
            echo "   ✅ HTTPS 憑證已啟用：https://${DOMAIN_NAME}"
            break
        fi
        if [ "$SECONDS" -ge "$deadline" ]; then
            echo "   ⚠️  等待憑證逾時。請確認 DNS A record 指向 ${STATIC_IP_ADDRESS}，再執行："
            echo "      kubectl describe managedcertificate pacs-managed-cert -n pacs"
            break
        fi
        sleep 30
    done
fi

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
echo "   make k8s-pods"
echo "   # 或："
echo "   kubectl get pods -n pacs"
echo ""
echo "2. 取得 Ingress IP（可能需 2-3 分鐘才分配）："
echo "   make gke-ingress"
echo "   # 或："
echo "   kubectl get ingress pacs-ingress -n pacs"
if [ "$ENABLE_HTTPS" = "1" ]; then
    echo "   HTTPS 憑證狀態："
    echo "   kubectl describe managedcertificate pacs-managed-cert -n pacs"
    echo "   或：make gke-https-status DOMAIN_NAME=${DOMAIN_NAME}"
    echo "   等待：make gke-https-wait DOMAIN_NAME=${DOMAIN_NAME}"
    echo "   等狀態變成 Active 後使用：https://${DOMAIN_NAME}"
fi
echo ""
echo "3. 雲端大規模播種（90,000 人，手動執行）："
echo "   make gke-seed-cloud"
echo "   # 或進入 db-tools Pod 手動執行："
echo "   kubectl exec -it -n pacs pod/db-tools -c psql -- sh"
echo "   psql -v ON_ERROR_STOP=1 -f /cloud-seed/0104_cloud_seed.up.sql"
echo ""
echo "4. 壓力測試："
echo "   make k6-gke"
echo "   # 或："
echo "   kubectl delete job -n pacs k6-shift-burst --ignore-not-found"
echo "   kubectl apply -f k8s/07-k6-load-test.yaml"
echo "   kubectl logs -f -n pacs job/k6-shift-burst"
echo ""
