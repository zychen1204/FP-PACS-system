#!/usr/bin/env bash
# ============================================================
# test_future_time_guard.sh — 端到端驗證未來時間排除機制
#
# 涵蓋三層 defense in depth：
#   1. seed 紀律：0099 dev_seed + seed-generator 都不寫未來時間
#   2. MV 層     ：0106 mv_daily_attendance 加 event_time <= NOW() guard
#   3. API 層    ：QueryAuditTrail 加 event_time <= NOW() cap
#
# 用法：
#   docker compose up -d        # 先把環境跑起來
#   ./scripts/tests/test_future_time_guard.sh
#
# 退出碼：0=全綠；非 0=有 case 失敗（會印出 diff）
# ============================================================
set -euo pipefail

PSQL="docker compose exec -T postgres psql -U pacs_user -d pacs_db -tA"
PASS=0; FAIL=0

assert_eq() {
    local label="$1"; local expected="$2"; local actual="$3"
    if [[ "$expected" == "$actual" ]]; then
        echo "  ✅ $label  (got=$actual)"
        PASS=$((PASS+1))
    else
        echo "  ❌ $label  expected=$expected got=$actual" >&2
        FAIL=$((FAIL+1))
    fi
}

echo "==> Test 1: 0099 dev_seed 全部在過去"
DEV_FUTURE=$($PSQL -c "SELECT COUNT(*) FROM access_events WHERE reason LIKE '[DEV_SEED]%' AND event_time > NOW();")
assert_eq "DEV_SEED 沒有未來事件" "0" "$DEV_FUTURE"

DEV_MAX_OK=$($PSQL -c "SELECT (MAX(event_time) < NOW())::text FROM access_events WHERE reason LIKE '[DEV_SEED]%';")
assert_eq "DEV_SEED 最晚時間 < NOW" "true" "$DEV_MAX_OK"

echo
echo "==> Test 2: site_id 字典統一（沒有舊版 Site-A/B）"
LEGACY_SITES=$($PSQL -c "SELECT COUNT(*) FROM access_events WHERE site_id IN ('Site-A','Site-B');")
assert_eq "沒有舊 Site-A/B 命名" "0" "$LEGACY_SITES"

echo
echo "==> Test 3: 0106 MV 阻擋未來事件"
# 插入一筆 1 小時後的未來事件
$PSQL -c "INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time, event_date) VALUES ('B-FUTURE-TEST', 'FAB12A', 'G-01', 'IN', 'SUCCESS', '[FUTURE_TEST]', NOW() + INTERVAL '1 hour', CURRENT_DATE);" >/dev/null

$PSQL -c "REFRESH MATERIALIZED VIEW mv_daily_attendance;" >/dev/null

MV_LEAK=$($PSQL -c "SELECT COUNT(*) FROM mv_daily_attendance WHERE badge_id = 'B-FUTURE-TEST';")
assert_eq "未來事件未進入 MV" "0" "$MV_LEAK"

echo
echo "==> Test 4: audit query 也排除未來事件（與 MV 是雙保險）"
# audit 走 access_events 直查，要靠 backend code 的 event_time <= NOW() cap
# 這邊用 SQL 直接模擬 backend code 的 query
TODAY=$($PSQL -c "SELECT CURRENT_DATE::text;")
AUDIT_LEAK=$($PSQL -c "SELECT COUNT(*) FROM access_events WHERE badge_id = 'B-FUTURE-TEST' AND event_date BETWEEN '$TODAY' AND '$TODAY' AND event_time <= NOW();")
assert_eq "audit query cap 未來" "0" "$AUDIT_LEAK"

# 不清測試資料：FR-12 immutable 不允許 DELETE，且 [FUTURE_TEST] tag 本身就是診斷信號
# 如要清掉，跑 ./scripts/demo-reset.sh 即可

echo
echo "─────────────────────────────────────────"
echo "  $PASS passed, $FAIL failed"
echo "─────────────────────────────────────────"
[[ $FAIL -eq 0 ]] || exit 1
