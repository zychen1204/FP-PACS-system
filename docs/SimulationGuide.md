# 0103 員工與刷卡紀錄模擬指南 (Simulation)

本功能用於在本地開發與測試環境中，快速生成大量的組織與員工資料（後續將支援刷卡紀錄生成）。

SQL模擬格式檔案位置

- `scripts/migrations/0103_seed_local.up.sql`
- `scripts/migrations/0103_seed_local.down.sql`

模擬程式

- `scripts/load-generator/`


## 做了什麼 (What it does)

1. **清除舊資料**：安全地清空 `employees` 與 `access_events` 內的測試髒資料。
2. **生成 1,000 名階層員工**：
   - 1 位廠長 (MANAGER_L1)
   - 10 位部門經理 (MANAGER_L2)
   - 989 位一般員工 (STAFF)
3. **建立查詢索引**：自動產生對應的 `org_path_ltree` (例如 `TSMC.製造部_01`) 以供主管視野權限查詢。
4. **刷新視圖**：自動呼叫更新 `mv_daily_attendance` 統計報表。
5. **(TODO) 刷卡紀錄模擬**：未來將擴充此腳本，為上述 1000 位員工自動生成「單日多次 IN/OUT」的刷卡配對，用以驗證停留時數累加與進行壓力測試。




## 如何模擬 (How to simulate)


### 1. 開啟docker compose
```bash
docker compose down -v

docker compose up -d
```

### 2. 動態打卡模擬 (Go Load Generator)

```bash
# 進入目錄
cd scripts/load-generator

# 執行模擬產出 SQL
go run . --mode local --days 30 --clear

**模擬器參數：**
- `--mode local`: 針對本地 1,000 人進行模擬。
- `--days N`: 模擬 N 天的打卡紀錄（預設 30）。
- `--clear`: 清除所有資料庫原本資料。

# 匯入產出的 SQL (Windows PowerShell 務必加cmd /c 在前面和雙引號)
docker compose exec -T postgres psql -U pacs_user -d pacs_db < seed_history_events.sql

```
### 3.前往前端介面 出席報表輸入ID看報表 (注意ID是 B-000001 ~ B-001000 )
http://localhost/


