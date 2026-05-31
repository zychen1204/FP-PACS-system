#!/usr/bin/env bash
# ============================================================
# cloud-reset.sh — PACS GKE Cloud SQL access_events 一鍵重灌
#
# 用途：
#   * 把雲端 Cloud SQL 的 access_events 清空、灌入 seed-generator 產生的歷史資料
#   * employees / alerts / MV 定義不動
#   * Cloud SQL 不允許 superuser-only 操作（SET session_replication_role），所以
#     改用 ALTER TABLE ... DISABLE/ENABLE TRIGGER USER 對 parent + 所有 partition
#     toggle FR-12 trigger，TRUNCATE / INSERT / REFRESH MV 後恢復
#
# Usage:
#   ./scripts/cloud-reset.sh                       # 7 天 × local (1K)
#   ./scripts/cloud-reset.sh 30                    # 30 天 × local
#   ./scripts/cloud-reset.sh 7 fab                 # 7 天 × fab (30K)
#   ./scripts/cloud-reset.sh 2025-06-01            # 2025-06-01 → 昨天 × local
#   ./scripts/cloud-reset.sh 2025-06-01 fab        # 絕對日期 × fab
#   ./scripts/cloud-reset.sh --dry-run 2025-06-01  # 只 patch SQL 不灌
#
# 退出碼：
#   0  成功
#   1  kubectl context 錯誤（不在 pacs-cluster）
#   2  seed-generator 失敗
#   3  db-tools pod 起不來 / 灌入失敗
# ============================================================
set -euo pipefail

EXPECTED_CTX="gke_extreme-water-497313-j8_asia-east1_pacs-cluster"
NAMESPACE="pacs"
POD_NAME="db-tools"
DRY_RUN=false

# ── Parse flags ──────────────────────────────────────────────
POSITIONAL=()
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run) DRY_RUN=true; shift ;;
        --help|-h) sed -n '3,21p' "$0"; exit 0 ;;
        *) POSITIONAL+=("$1"); shift ;;
    esac
done
set -- "${POSITIONAL[@]:-}"

ARG1="${1:-7}"
MODE="${2:-local}"

# Detect absolute-date (YYYY-MM-DD) vs relative-days (integer)
if [[ "$ARG1" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
    SEED_LABEL="$ARG1 → yesterday (absolute)"
    SEED_ARGS=(--mode "$MODE" --start-date "$ARG1")
else
    SEED_LABEL="$ARG1 days (relative)"
    SEED_ARGS=(--mode "$MODE" --days "$ARG1")
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "==> PACS Cloud Reset"
echo "    Scale:    $MODE"
echo "    Range:    $SEED_LABEL"
echo "    Dry-Run:  $DRY_RUN"
echo "    Root:     $ROOT"
echo

# ── Phase 0: kubectl context 守則 ────────────────────────────
echo "[1/6] checking kubectl context ..."
CURRENT_CTX=$(kubectl config current-context 2>/dev/null || echo "")
if [[ "$CURRENT_CTX" != "$EXPECTED_CTX" ]]; then
    echo "❌ kubectl context 必須是 $EXPECTED_CTX" >&2
    echo "   當前: ${CURRENT_CTX:-<none>}" >&2
    echo "   切換: gcloud container clusters get-credentials pacs-cluster \\" >&2
    echo "          --location=asia-east1 --project=extreme-water-497313-j8" >&2
    exit 1
fi
echo "      OK: $CURRENT_CTX"

# ── Phase 1: 本地產 SQL ──────────────────────────────────────
echo "[2/6] seed-generator ${SEED_ARGS[*]} ..."
SQL_FILE="$ROOT/scripts/seed-generator/seed_history_events.sql"
(
    cd scripts/seed-generator
    go run . "${SEED_ARGS[@]}"
) || { echo "❌ seed-generator 失敗" >&2; exit 2; }
[[ -f "$SQL_FILE" ]] || { echo "❌ SQL 檔未產生" >&2; exit 2; }
echo "      SQL: $(du -h "$SQL_FILE" | awk '{print $1}')"

# ── Phase 2: 適配 Cloud SQL 限制 ─────────────────────────────
echo "[3/6] patching SQL for Cloud SQL ..."
CLOUD_SQL_FILE="$ROOT/scripts/seed-generator/seed_history_events_cloud.sql"

# Cloud SQL constraints:
#   (a) session_replication_role 需 superuser 權限，雲端 pacs_user 沒有
#       → 改用 ALTER TABLE ... DISABLE/ENABLE TRIGGER USER（pacs_user 為 owner OK）
#   (b) TRUNCATE access_events 同時會 fire parent + 每個 partition 的 trigger
#       → parent 與 37 個 partition 都要 toggle
DISABLE_PARTITIONS="DO \$\$ DECLARE p REGCLASS; BEGIN FOR p IN SELECT inhrelid::regclass FROM pg_inherits WHERE inhparent='access_events'::regclass LOOP EXECUTE format('ALTER TABLE %s DISABLE TRIGGER USER', p); END LOOP; END \$\$;"
ENABLE_PARTITIONS="DO \$\$ DECLARE p REGCLASS; BEGIN FOR p IN SELECT inhrelid::regclass FROM pg_inherits WHERE inhparent='access_events'::regclass LOOP EXECUTE format('ALTER TABLE %s ENABLE TRIGGER USER', p); END LOOP; END \$\$;"

sed -e "s|DELETE FROM access_events WHERE reason = '\[STRESS_TEST\]';|TRUNCATE access_events;|" \
    -e "s|^SET session_replication_role = 'replica';\$|${DISABLE_PARTITIONS}|" \
    -e "s|^SET session_replication_role = 'origin';\$|${ENABLE_PARTITIONS}|" \
    "$SQL_FILE" > "$CLOUD_SQL_FILE"

# Wrap with parent table ALTER (DO blocks above only handle partitions)
{
    echo "ALTER TABLE access_events DISABLE TRIGGER USER;"
    cat "$CLOUD_SQL_FILE"
    echo "ALTER TABLE access_events ENABLE TRIGGER USER;"
} > "$CLOUD_SQL_FILE.wrapped" && mv "$CLOUD_SQL_FILE.wrapped" "$CLOUD_SQL_FILE"

echo "      patched: $(du -h "$CLOUD_SQL_FILE" | awk '{print $1}')"

if [[ "$DRY_RUN" == "true" ]]; then
    echo
    echo "🔍 Dry-run — SQL ready at $CLOUD_SQL_FILE"
    echo "    Run without --dry-run to apply to cloud."
    exit 0
fi

# ── Phase 3: 起 db-tools pod ─────────────────────────────────
echo "[4/6] apply db-tools pod + wait Ready ..."
kubectl apply -f k8s/12-db-tools.yaml >/dev/null
kubectl wait --for=condition=Ready "pod/$POD_NAME" -n "$NAMESPACE" --timeout=120s >/dev/null || {
    echo "❌ db-tools pod 起不來" >&2
    kubectl describe "pod/$POD_NAME" -n "$NAMESPACE" >&2
    exit 3
}
echo "      pod ready"

# Cleanup pod on exit regardless of success / failure
cleanup() {
    echo "      cleaning up db-tools pod ..."
    kubectl delete "pod/$POD_NAME" -n "$NAMESPACE" --wait=false >/dev/null 2>&1 || true
}
trap cleanup EXIT

# ── Phase 4: cp + psql -f ────────────────────────────────────
echo "[5/6] uploading + executing SQL (1-5 min depending on volume) ..."
kubectl cp "$CLOUD_SQL_FILE" "$NAMESPACE/$POD_NAME:/tmp/seed.sql" -c psql
kubectl exec -n "$NAMESPACE" "$POD_NAME" -c psql -- \
    psql -v ON_ERROR_STOP=1 -f /tmp/seed.sql >/dev/null || {
    echo "❌ psql 灌入失敗" >&2
    exit 3
}

# ── Phase 5: 驗證 ────────────────────────────────────────────
echo "[6/6] verifying ..."
RESULT=$(kubectl exec -n "$NAMESPACE" "$POD_NAME" -c psql -- psql -tA -c "
SELECT MIN(event_date), MAX(event_date), COUNT(*), COUNT(DISTINCT badge_id) FROM access_events;
")
MIN_DATE=$(echo "$RESULT" | awk -F'|' '{print $1}')
MAX_DATE=$(echo "$RESULT" | awk -F'|' '{print $2}')
TOTAL=$(echo "$RESULT" | awk -F'|' '{print $3}')
BADGES=$(echo "$RESULT" | awk -F'|' '{print $4}')

MONTHS=$(kubectl exec -n "$NAMESPACE" "$POD_NAME" -c psql -- psql -tA -c "
SELECT COUNT(DISTINCT date_trunc('month', event_date)) FROM access_events;
")

echo
echo "✅ Cloud DB reset complete"
echo "    Range:           $MIN_DATE → $MAX_DATE"
echo "    Total events:    $TOTAL"
echo "    Unique badges:   $BADGES"
echo "    Months covered:  $MONTHS"
echo
echo "    Open frontend:   kubectl port-forward -n pacs svc/frontend 8080:80"
