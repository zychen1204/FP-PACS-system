#!/usr/bin/env bash
# ============================================================
# demo-reset.sh — PACS demo 環境一鍵重灌
#
# 用途：
#   * Demo 前重置乾淨環境
#   * 灌入過去 N 天歷史資料（不含今天，今天留給即時 swipe demo）
#   * 強制 REFRESH mv_daily_attendance
#   * 驗證沒有未來時間事件
#
# Usage:
#   ./scripts/demo-reset.sh           # default 7 days
#   ./scripts/demo-reset.sh 30        # 30 days
#   ./scripts/demo-reset.sh 7 fab     # 30K employees (Phase 2 scale)
#
# 退出碼：
#   0  成功
#   1  postgres 起不來
#   2  seed-generator 失敗
#   3  發現未來時間事件（不應該）
# ============================================================
set -euo pipefail

DAYS="${1:-7}"
MODE="${2:-local}"

# 對齊 repo root（不論從哪呼叫都能 work）
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "==> PACS Demo Reset"
echo "    Mode:  $MODE"
echo "    Days:  $DAYS (today excluded — reserved for live swipe demo)"
echo "    Root:  $ROOT"
echo

# ── Phase 1: 完全清空 ─────────────────────────────────────
echo "[1/5] docker compose down -v ..."
docker compose down -v

# ── Phase 2: 啟動 DB + Redis ──────────────────────────────
echo "[2/5] docker compose up -d postgres redis ..."
docker compose up -d postgres redis

echo "      waiting for postgres healthy ..."
for i in {1..60}; do
    if docker compose exec -T postgres pg_isready -U pacs_user -d pacs_db >/dev/null 2>&1; then
        echo "      postgres ready"
        break
    fi
    sleep 1
    if [[ $i -eq 60 ]]; then
        echo "❌ postgres did not become healthy in 60s" >&2
        exit 1
    fi
done

# ── Phase 3: 跑全部服務（含 migrate 0001~0106）────────────
echo "[3/5] docker compose up -d (runs migrations 0001~0106) ..."
docker compose up -d

# 等 migrate exit；用 docker inspect 確認 ExitCode = 0
# 比 `docker compose wait` 穩定（後者在新版 compose 上 container 退出後找不到）
echo "      waiting for migrate to complete ..."
for i in {1..60}; do
    STATE=$(docker inspect final_project-migrate-1 --format '{{.State.Status}}/{{.State.ExitCode}}' 2>/dev/null || echo "missing/")
    case "$STATE" in
        exited/0)
            echo "      migrate completed (exit 0)"
            break
            ;;
        exited/*)
            echo "❌ migrate failed: $STATE" >&2
            docker compose logs migrate >&2
            exit 1
            ;;
    esac
    sleep 1
    if [[ $i -eq 60 ]]; then
        echo "❌ migrate did not finish in 60s (state=$STATE)" >&2
        docker compose logs migrate >&2
        exit 1
    fi
done

# ── Phase 4: seed-generator + 灌歷史 ──────────────────────
echo "[4/5] seed-generator --mode $MODE --days $DAYS ..."
(
    cd scripts/seed-generator
    go run . --mode "$MODE" --days "$DAYS"
)

echo "      psql < seed_history_events.sql ..."
docker compose exec -T postgres psql -U pacs_user -d pacs_db \
    < scripts/seed-generator/seed_history_events.sql >/dev/null

echo "      REFRESH MATERIALIZED VIEW mv_daily_attendance ..."
docker compose exec -T postgres psql -U pacs_user -d pacs_db \
    -c "REFRESH MATERIALIZED VIEW mv_daily_attendance;" >/dev/null

# ── Phase 5: 驗證沒有未來時間 ─────────────────────────────
echo "[5/5] sanity check — no future events ..."
FUTURE_COUNT=$(docker compose exec -T postgres psql -U pacs_user -d pacs_db -tA -c \
    "SELECT COUNT(*) FROM access_events WHERE event_time > NOW();")

if [[ "$FUTURE_COUNT" != "0" ]]; then
    echo "❌ Found $FUTURE_COUNT future events in access_events — seed pollution!" >&2
    docker compose exec -T postgres psql -U pacs_user -d pacs_db -c \
        "SELECT badge_id, event_time, NOW() AS server_now, reason
         FROM access_events WHERE event_time > NOW() LIMIT 5;" >&2
    exit 3
fi

TOTAL=$(docker compose exec -T postgres psql -U pacs_user -d pacs_db -tA -c \
    "SELECT COUNT(*) FROM access_events;")
DEV=$(docker compose exec -T postgres psql -U pacs_user -d pacs_db -tA -c \
    "SELECT COUNT(*) FROM access_events WHERE reason LIKE '[DEV_SEED]%';")
STRESS=$(docker compose exec -T postgres psql -U pacs_user -d pacs_db -tA -c \
    "SELECT COUNT(*) FROM access_events WHERE reason = '[STRESS_TEST]';")

echo
echo "✅ Demo data ready"
echo "    access_events total : $TOTAL"
echo "      └─ [DEV_SEED]     : $DEV   (0099 demo rows)"
echo "      └─ [STRESS_TEST]  : $STRESS (seed-generator $DAYS days)"
echo "      └─ future events  : $FUTURE_COUNT (must be 0)"
echo
echo "    Open dashboard:  http://localhost/"
echo "    Trend：過去 $DAYS 天有資料，今天空白；用前端模擬器送 swipe 即時展示 CQRS write path"
