# `scripts/`

Database build artefacts for PACS. PostgreSQL 16 schema is managed via
[golang-migrate](https://github.com/golang-migrate/migrate); fixtures
are manual-load helpers for performance testing.

> **狀態**：Phase 1 + Phase 2 migrations 已落地（0001~0006 + 0100~0101 + 0099）。
> 設計脈絡見 [`../docs/PHASE2_CHANGES.md`](../docs/PHASE2_CHANGES.md)。

```
scripts/
├── migrations/                                               versioned schema (single source of truth)
│   ├── 0001_init_schema.{up,down}.sql                          baseline (tables, indexes, triggers, roles, employees seed)
│   ├── 0002_add_manager_flag.{up,down}.sql                     FR-6/9 schema gap: is_manager flag + 廠長/部員 seed
│   ├── 0003_ltree_org_path.{up,down}.sql                       Phase 2: ltree extension + org_path_ltree + GiST + sync trigger
│   ├── 0004_alerts_table.{up,down}.sql                         Phase 2: FR-11 alerts table (CHECK 列舉 + 索引)
│   ├── 0005_partition_access_events.{up,down}.sql              Phase 2: access_events RANGE-partition by month (36 個月)
│   ├── 0006_mv_daily_attendance.{up,down}.sql                  Phase 2: materialized view + UNIQUE + GiST 索引
│   ├── 0100_protect_access_event_partitions.{up,down}.sql      Phase 2 hardening: FR-12 trigger 擴到每個子 partition
│   ├── 0101_access_event_partition_safety.{up,down}.sql        Phase 2 hardening: default partition + ensure_access_event_partition() function
│   └── 0099_dev_seed.{up,down}.sql                             ~45 demo rows tagged reason='[DEV_SEED]' + REFRESH MV
└── fixtures/
    └── load_test.sql                                           10k events fixture for NFR-2 EXPLAIN ANALYZE
```

> 註：golang-migrate 依整數版本號排序，實際執行順序為
> `0001 → 0002 → 0003 → 0004 → 0005 → 0006 → 0099 → 0100 → 0101`，
> 因此 `0099_dev_seed` 不再是最後一支（被 0100/0101 hardening migration 接在後面）。
> 0100/0101 是 schema-only（CREATE FUNCTION / CREATE TRIGGER / ATTACH default partition），
> 不依賴 dev_seed 資料，跑在 dev_seed 之後完全安全。日後若想保留「dev_seed 永遠最後」
> 的慣例，可考慮把 dev_seed 改 `9999_dev_seed.sql`。

## Why split into separate migrations (vs. one merged file)

The schema 由 `0001_init_schema` baseline 起跳（tables, indexes, triggers,
`REVOKE`, role grants, 5 員工 seed）。後續每次 schema 變動都開一個新 migration，
規則：

- **觸碰 production 資料的變動**（additive `ALTER` vs table rebuild）→ 必須新 migration
- **schema 已 publish**（PR / pushed branch）→ 修改 baseline 等同 force-push
  public history，違反 git safety → 新 migration
- **Phase 升級**（partitioning、closure table、materialised view、ltree、
  alerts 表等）→ 必須分階段、可獨立 review → 新 migration

實際歷史：

| Migration | 觸發原因 |
|---|---|
| `0002` | `0001` 已上 PR #1，後深度核對 spec 發現 FR-6/9 缺 `is_manager` flag，加新 migration 而非 amendment（rule 2）|
| `0003` | Phase 2 §5.3 ltree 升級 |
| `0004` | Phase 2 FR-11 警報表 |
| `0005` | Phase 2 §5.3 partition by month |
| `0006` | Phase 2 §5.3 materialized view |

`0099_dev_seed` 永遠保持最後一支：FR-12 immutability 不允許 `down` 刪 INSERTs，
所以重置 demo 資料只能用 `docker compose down -v`；耦合到 schema migration
會造成混亂 semantics。

## How migrations run

`docker compose up` brings up a one-shot `migrate` service that waits
for postgres to be healthy, then runs `migrate ... up`. Application
services (`event-processor`, `reporting-api`, `anomaly-detector`,
`mv-refresher`, `org-sync`) wait on `migrate: service_completed_successfully`,
so they only start once the schema is at head.

To run migrations manually outside compose:

```bash
docker run --rm -v "$(pwd)/scripts/migrations:/migrations" \
  --network fp-pacs-system_pacs-network \
  migrate/migrate:v4.17.1 \
  -path=/migrations \
  -database='postgres://pacs_user:pacs_password@postgres:5432/pacs_db?sslmode=disable' \
  up
```

Roll back the most recent migration:

```bash
docker run --rm -v "$(pwd)/scripts/migrations:/migrations" \
  --network fp-pacs-system_pacs-network \
  migrate/migrate:v4.17.1 \
  -path=/migrations -database='...' down 1
```

## Adding a new migration

1. Pick the next free four-digit prefix（已使用至 `0006` + `0100`/`0101`；
   next available 是 `0007` 或 `0102`，看是 Phase 2 完工項目還是後續 hardening）。
2. Create both files: `NNNN_short_description.up.sql` and `.down.sql`。
3. `up` 檔在合理範圍內 idempotent（`IF NOT EXISTS`、`CREATE OR REPLACE`、
   `DROP ... IF EXISTS`）；`down` 檔應精確 undo `up`，除非被 FR-12 immutability 阻擋。
4. 本地測試：
   ```bash
   docker compose down -v && docker compose up -d
   docker compose logs migrate     # exit code 0
   ```

## Naming conventions

| Range | Purpose |
|---|---|
| `0001` | baseline schema |
| `0002` | FR-6/FR-9 manager flag + extra demo employees |
| `0003` | Phase 2 ltree organization tree |
| `0004` | Phase 2 alerts table (FR-11) |
| `0005` | Phase 2 access_events partition by month |
| `0006` | Phase 2 mv_daily_attendance materialized view |
| `0007-0098` | future schema changes（Phase 3 升級、ad-hoc additions） |
| `0099` | dev seed（only loaded in dev/demo；不再嚴格保證最後）|
| `0100` | Phase 2 hardening：FR-12 trigger 擴到每個子 partition |
| `0101` | Phase 2 hardening：default partition + `ensure_access_event_partition()` 預建函式 |
| `0102+` | future hardening / Phase 3 schema 改動 |

## Roles

| Role | Provisioned by | Privileges |
|---|---|---|
| `pacs_user` | postgres image (`POSTGRES_USER`) | `SELECT, INSERT` on `access_events` / `alerts`（`UPDATE/DELETE` revoked + FR-12 trigger guard）；owner of `mv_daily_attendance`（可 REFRESH） |
| `pacs_reporter` | migration `0001` (+ `0004`/`0006` grants) | `SELECT` only on `access_events` / `employees` / `alerts` / `mv_daily_attendance` |

連線分配：

- `event-processor` 連 `pacs_user`（寫 events）
- `anomaly-detector` 連 `pacs_user`（寫 alerts）
- `mv-refresher` 連 `pacs_user`（REFRESH MV 需要 owner 權限）
- `org-sync` 連 `pacs_user`（UPSERT employees）
- `reporting-api` 連 `pacs_reporter`（read-only，via `postgres-replica` alias）
- `access-api` 完全不連 PostgreSQL（走 Redis）

## FR-12 immutability

`access_events` is append-only. 由 **兩層保護**：

1. **Privilege**: `REVOKE UPDATE, DELETE ON access_events FROM pacs_user`。
2. **Trigger**: `BEFORE UPDATE OR DELETE` 與 `BEFORE TRUNCATE` 一律 RAISE EXCEPTION，
   superuser 也擋下。Phase 2 `0005` partition swap 後 trigger 重掛到 partition root。

Migration 影響：

- `down` migration 想 DELETE `access_events` 會失敗。`0099_dev_seed.down.sql` 因此
  no-op；重置 demo 資料用 `docker compose down -v && docker compose up -d`。
- `0005` partition swap 用 `INSERT INTO ... SELECT` 搬資料、`RENAME` swap、`DROP TABLE` 舊表 —
  `DROP TABLE` 是 DDL 不會 fire BEFORE DELETE trigger，所以 swap 安全。

## NFR-2 verification (reporting P95 < 200 ms)

Load the fixture（注意：Phase 2 後 `access_events.event_date` 是普通欄位，
fixture 必須帶 event_date；用根目錄 `docs/PHASE2_VERIFICATION.md` §12.2 的
inline SQL 載入 10k rows），再 `EXPLAIN ANALYZE`。

Plan must show:
- `Bitmap Index Scan on access_events_yYYYYmMM_event_date_badge_id_idx`（partition-local partial index）
- `Subplans Removed: 35`（partition pruning 砍掉非當月 35 個 partition）
- Execution Time ≪ 200 ms

實測（10k rows，2026-05）：attendance 2.564 ms / audit 0.331 ms。詳見
[`../docs/PHASE2_VERIFICATION.md`](../docs/PHASE2_VERIFICATION.md) §12。

## Phase 2 partition 落地後的維護

`access_events` 已依 `event_date` `PARTITION BY RANGE`，預建 `access_events_y2025m01` ~
`access_events_y2027m12` 共 36 個月份分區。後續維護：

1. **預建下一個月**：接近 2027-12 時，加一個 migration 預建 2028 起的 partition，
   或加 cron job（推薦 `pg_partman` extension）自動預建未來 N 個月。
2. **歸檔舊月份**：> 24 個月的 partition DETACH 後可移到 Cloud Storage（Phase 3）：
   ```sql
   ALTER TABLE access_events DETACH PARTITION access_events_y2024m01;
   COPY access_events_y2024m01 TO '/tmp/2024_01.csv' WITH CSV;
   DROP TABLE access_events_y2024m01;
   ```
3. **新 partition 自動繼承索引**：partition root 上的索引（`idx_events_status_date` 等）
   會自動傳播到子表，無需手動建。

升級到真 Read Replica 的步驟見 [`../docs/PHASE2_CHANGES.md`](../docs/PHASE2_CHANGES.md) §9.4。
