-- Remove child-partition audit guards added by 0100. The parent triggers from
-- 0005 remain in place.

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
        EXECUTE format('DROP TRIGGER IF EXISTS trg_protect_audit_truncate ON %s', partition_oid);
    END LOOP;
END
$$;
