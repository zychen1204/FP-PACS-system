DROP TABLE IF EXISTS access_events_default;
DROP FUNCTION IF EXISTS ensure_access_event_partition(DATE);
DROP FUNCTION IF EXISTS attach_access_event_partition_guards(REGCLASS);
