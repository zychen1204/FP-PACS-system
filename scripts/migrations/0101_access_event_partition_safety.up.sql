-- ============================================================
-- 0101 access_event_partition_safety
--
-- Prevent writes from failing when an event falls outside the pre-created
-- monthly partition window. The default partition preserves correctness, while
-- ensure_access_event_partition() gives operations a reusable way to add future
-- month partitions with FR-12 guards attached.
-- ============================================================

CREATE OR REPLACE FUNCTION attach_access_event_partition_guards(partition_name REGCLASS)
RETURNS VOID AS $$
BEGIN
    EXECUTE format('DROP TRIGGER IF EXISTS trg_protect_audit ON %s', partition_name);
    EXECUTE format(
        'CREATE TRIGGER trg_protect_audit
         BEFORE UPDATE OR DELETE ON %s
         FOR EACH STATEMENT
         EXECUTE FUNCTION protect_audit_log()',
        partition_name
    );

    EXECUTE format('DROP TRIGGER IF EXISTS trg_protect_audit_truncate ON %s', partition_name);
    EXECUTE format(
        'CREATE TRIGGER trg_protect_audit_truncate
         BEFORE TRUNCATE ON %s
         FOR EACH STATEMENT
         EXECUTE FUNCTION protect_audit_log()',
        partition_name
    );
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION ensure_access_event_partition(month_start DATE)
RETURNS VOID AS $$
DECLARE
    partition_start DATE := date_trunc('month', month_start)::date;
    partition_end   DATE := (date_trunc('month', month_start) + INTERVAL '1 month')::date;
    partition_name  TEXT := format('access_events_y%sm%s',
                                   to_char(date_trunc('month', month_start), 'YYYY'),
                                   to_char(date_trunc('month', month_start), 'MM'));
BEGIN
    EXECUTE format(
        'CREATE TABLE IF NOT EXISTS %I PARTITION OF access_events
         FOR VALUES FROM (%L) TO (%L)',
        partition_name, partition_start, partition_end
    );
    PERFORM attach_access_event_partition_guards(partition_name::regclass);
END;
$$ LANGUAGE plpgsql;

CREATE TABLE IF NOT EXISTS access_events_default
    PARTITION OF access_events DEFAULT;

SELECT attach_access_event_partition_guards('access_events_default'::regclass);
