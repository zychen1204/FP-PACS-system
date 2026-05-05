# `scripts/`

Database build artefacts for PACS. PostgreSQL 15 schema is managed via
[golang-migrate](https://github.com/golang-migrate/migrate); fixtures
are manual-load helpers for performance testing.

```
scripts/
├── migrations/                                       versioned schema (single source of truth)
│   ├── 0001_init_schema.{up,down}.sql                  consolidated baseline (tables, indexes, triggers, roles)
│   ├── 0002_add_manager_flag.{up,down}.sql             FR-6/FR-9 schema gap: is_manager + 廠長/部員 seed
│   └── 0099_dev_seed.{up,down}.sql                     ~45 demo rows tagged reason='[DEV_SEED]'
└── fixtures/
    └── load_test.sql                                 10k events for one badge, used to verify NFR-2 with EXPLAIN ANALYZE
```

## Why baseline + 0002 (and not one merged file)

The schema is defined in a single `0001_init_schema` baseline (tables,
indexes, triggers, `REVOKE`, role grants, and seed employees). We only
split a change into its own migration when:

- the change touches data already in production (additive `ALTER` vs
  table rebuild), or
- the schema is already published (open PR / pushed branch) — folding
  into the baseline file would force-push public history, or
- the change is part of a Phase 2 upgrade (partitioning, closure
  table, materialised view) that must be staged for a planned window.

`0002_add_manager_flag` falls under rule 2: `0001` was already on PR #1
when the FR-6/FR-9 manager-identification gap was found, so the fix
ships as a new file rather than an amendment.

The `0099_dev_seed` slot stays separate because FR-12 immutability
prevents its `down` from undoing INSERTs (resetting demo data requires
`docker compose down -v`), so coupling it to the schema migration
would create confusing semantics.

## How migrations run

`docker compose up` brings up a one-shot `migrate` service that waits
for postgres to be healthy, then runs `migrate ... up`. Application
services (`event-processor`, `reporting-api`) wait on
`migrate: service_completed_successfully`, so they only start once
the schema is at head.

To run migrations manually outside compose:

```bash
docker run --rm -v "$(pwd)/scripts/migrations:/migrations" \
  --network pacs-system_pacs-network \
  migrate/migrate:v4.17.1 \
  -path=/migrations \
  -database='postgres://pacs_user:pacs_password@postgres:5432/pacs_db?sslmode=disable' \
  up
```

Roll back the most recent migration:

```bash
docker run --rm -v "$(pwd)/scripts/migrations:/migrations" \
  --network pacs-system_pacs-network \
  migrate/migrate:v4.17.1 \
  -path=/migrations -database='...' down 1
```

## Adding a new migration

1. Pick the next free four-digit prefix (next is `0003`; the `0099_dev_seed`
   slot is reserved for the demo seed and must remain last).
2. Create both files: `NNNN_short_description.up.sql` and `.down.sql`.
3. The `up` file should be idempotent where reasonable (`IF NOT EXISTS`,
   `CREATE OR REPLACE`, `DROP ... IF EXISTS`); the `down` file should
   undo `up` precisely, except where blocked by FR-12 immutability.
4. Test locally:
   ```bash
   docker compose down -v && docker compose up -d
   docker compose logs migrate     # exit code 0
   ```

## Naming conventions

| Range       | Purpose |
|-------------|---------|
| `0001`      | consolidated baseline schema |
| `0002`      | FR-6/FR-9 manager flag + extra demo employees |
| `0003-0098` | future schema changes (Phase 2 upgrades, ad-hoc additions) |
| `0099`      | dev seed (always last; only loaded in dev/demo) |

## Roles

| Role            | Provisioned by   | Privileges on `access_events` / `employees` |
|-----------------|------------------|---------------------------------------------|
| `pacs_user`     | postgres image (`POSTGRES_USER`) | `SELECT, INSERT` (`UPDATE`/`DELETE` revoked, plus trigger guard for FR-12) |
| `pacs_reporter` | migration `0001` | `SELECT` only |

`event-processor` connects as `pacs_user` (writes events).
`reporting-api` connects as `pacs_reporter` (read-only).
`access-api` does not connect to PostgreSQL at all.

## FR-12 immutability

`access_events` is append-only. The guarantee is enforced by **two
layers** (both in `0001`):

1. **Privilege**: `REVOKE UPDATE, DELETE ON access_events FROM pacs_user`.
2. **Trigger**: `BEFORE UPDATE OR DELETE` and `BEFORE TRUNCATE` raise
   an exception, so even a superuser invocation fails with a loud error.

Consequences for migrations:

- A `down` migration that needs to delete from `access_events` will
  fail. `0099_dev_seed.down.sql` is therefore a no-op; to reset demo
  data run `docker compose down -v && docker compose up -d`.

## NFR-2 verification (reporting P95 < 200 ms)

Load the fixture, then EXPLAIN ANALYZE the attendance query. The plan
must show `Index Scan using idx_events_status_date`, no `Seq Scan` on
`access_events`. See `TESTING.md`.

## Phase 2 partitioning playbook (out of scope for Phase 1)

At ~30k DAU (~10 GB/year, per the architecture doc) `access_events`
should be partitioned by `event_date` monthly to keep index footprint
and `VACUUM` cost flat. Outline:

1. New migration `NNNN_partition_access_events.up.sql`:
   - `CREATE TABLE access_events_new (... LIKE access_events INCLUDING ALL) PARTITION BY RANGE (event_date);`
   - `CREATE TABLE access_events_y2026m05 PARTITION OF access_events_new FOR VALUES FROM ('2026-05-01') TO ('2026-06-01');` (one per month going forward).
   - `INSERT INTO access_events_new SELECT * FROM access_events;` (transactional swap).
   - `BEGIN; ALTER TABLE access_events RENAME TO access_events_legacy; ALTER TABLE access_events_new RENAME TO access_events; COMMIT;`
   - Reattach trigger (`prevent_audit_log`) to the new partitioned root.
2. Add a monthly cron (or `pg_partman`) to pre-create the next month's
   partition.
3. Drop `access_events_legacy` after a week of dual-read confidence.

This is a one-time, planned-downtime operation (~30 minutes), not an
auto-run migration.
