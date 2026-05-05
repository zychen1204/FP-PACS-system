-- ============================================================
-- load_test fixture
-- 10,000 access_events for one badge spanning ~28 days, used to
-- validate NFR-2 (reporting P95 < 200ms) via EXPLAIN ANALYZE.
--
-- NOT auto-loaded. Run manually:
--   docker compose exec -T postgres \
--     psql -U pacs_user -d pacs_db < scripts/fixtures/load_test.sql
-- ============================================================

INSERT INTO access_events (badge_id, site_id, gate_id, direction, status, reason, event_time)
SELECT
    'B001',
    'Site-A',
    'Gate-1',
    CASE WHEN (n % 2) = 0 THEN 'IN' ELSE 'OUT' END,
    CASE WHEN (n % 100) = 0 THEN 'REJECTED_APB' ELSE 'SUCCESS' END,
    '[LOAD_TEST]',
    NOW() - (n * 4 || ' minutes')::interval
FROM generate_series(1, 10000) AS n;

ANALYZE access_events;

-- Sanity check
SELECT
    count(*)                                               AS total_rows,
    count(*) FILTER (WHERE status = 'SUCCESS')             AS success_rows,
    count(*) FILTER (WHERE status = 'REJECTED_APB')        AS rejected_rows,
    min(event_time)                                        AS earliest,
    max(event_time)                                        AS latest
FROM access_events
WHERE reason = '[LOAD_TEST]';
