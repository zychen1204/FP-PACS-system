-- ============================================================
-- 0100 protect_access_event_partitions
--
-- 0005 moved access_events to monthly partitions and protected the
-- partitioned parent with FR-12 append-only triggers. Direct DML against a
-- child partition does not fire statement-level triggers defined only on the
-- parent, so attach the same guards to every existing partition.
-- ============================================================

DO $$
DECLARE
    partition_oid REGCLASS;
BEGIN
    FOR partition_oid IN
        SELECT inhrelid::regclass
        FROM pg_inherits
        WHERE inhparent = 'access_events'::regclass
    LOOP
        EXECUTE format('DROP TRIGGER IF EXISTS trg_protect_audit ON %s', partition_oid);
        EXECUTE format(
            'CREATE TRIGGER trg_protect_audit
             BEFORE UPDATE OR DELETE ON %s
             FOR EACH STATEMENT
             EXECUTE FUNCTION protect_audit_log()',
            partition_oid
        );

        EXECUTE format('DROP TRIGGER IF EXISTS trg_protect_audit_truncate ON %s', partition_oid);
        EXECUTE format(
            'CREATE TRIGGER trg_protect_audit_truncate
             BEFORE TRUNCATE ON %s
             FOR EACH STATEMENT
             EXECUTE FUNCTION protect_audit_log()',
            partition_oid
        );
    END LOOP;
END
$$;
