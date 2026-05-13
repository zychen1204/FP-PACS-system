# PACS 報表 API — JSON 格式規範書

> 對象：前端組員、API 整合方、寫測試的人
> 對應規格：HW2 §2.2（FR-5~FR-8）、§2.3（FR-9/10）、§2.4（FR-11/13）
> 互補文件：
> - 後端設計脈絡 → [`PHASE2_CHANGES.md`](PHASE2_CHANGES.md)
> - 完整整合指引（含 UI 建議 / JS snippet）→ [`FRONTEND_INTEGRATION.md`](FRONTEND_INTEGRATION.md)
> - 驗收劇本 → [`PHASE2_VERIFICATION.md`](PHASE2_VERIFICATION.md)
>
> 本文件只描述「JSON wire format」：每個 endpoint 的 request、success response、
> error response 結構與真實範例。所有範例由實際 stack 抓取（dev_seed 資料）。

---

## 目錄

- [0. 總覽](#0-總覽)
- [1. 通用約定](#1-通用約定)
- [2. `GET /v1/reports/attendance`](#2-get-v1reportsattendance--fr-5-員工出勤)
- [3. `GET /v1/reports/manager-team`](#3-get-v1reportsmanager-team--fr-6--fr-9-主管報表)
- [4. `GET /v1/reports/trend`](#4-get-v1reportstrend--fr-7-出勤趨勢)
- [5. `GET /v1/audit`](#5-get-v1audit--fr-13-audit-trail)
- [6. `GET /v1/alerts`](#6-get-v1alerts--fr-11-異常警報)
- [7. `POST /v1/dev/login`](#7-post-v1devlogin--fr-10-jwt-issue)
- [8. `GET /v1/reports/attendance/export`](#8-get-v1reportsattendanceexport--fr-8-excel-匯出)
- [9. TypeScript 型別定義（給前端複製貼上）](#9-typescript-型別定義給前端複製貼上)
- [10. JSON Schema（給 contract test）](#10-json-schema給-contract-test)

---

## 0. 總覽

| Endpoint | 方法 | 回傳形式 | 對應 FR |
|---|---|---|---|
| `/v1/reports/attendance` | GET | `AttendanceRow[]` | FR-5 |
| `/v1/reports/manager-team` | GET | `{manager_scope, reports: AttendanceRow[]}` | FR-6 / FR-9 |
| `/v1/reports/trend` | GET | `{period, scope, trends: TrendBucket[]}` | FR-7 |
| `/v1/audit` | GET | `AccessEvent[]` | FR-13 |
| `/v1/alerts` | GET | `Alert[]` | FR-11 |
| `/v1/dev/login` | POST | `{access_token, token_type, expires_in}` | FR-10 |
| `/v1/reports/attendance/export` | GET | **binary xlsx** | FR-8 |

All endpoints under `http://localhost:8081`（直連）或 `http://localhost/`（走 nginx；需先補 `/v1/alerts` 與 `/v1/dev/login` 兩條 proxy，見 `FRONTEND_INTEGRATION.md` §5）。

---

## 1. 通用約定

### 1.1 字元集與 Content-Type
- 全部 JSON 回應為 `application/json; charset=utf-8`
- 中文（org_path、name）以 UTF-8 直接送出，**不做 unicode escape**
- 若用 Python `json.dumps` 觀察可能會看到 `\uXXXX`，那是 client 端字元；wire 上是原始 UTF-8

### 1.2 時間格式
- 所有 `timestamp / occurred_at / first_in / last_out / event_time` 用 **RFC 3339（ISO 8601）**：
  ```
  2026-05-11T01:00:00Z              // 整秒、UTC
  2026-05-11T11:57:27.084139Z       // 微秒精度、UTC
  ```
- 內部統一 UTC 儲存；前端顯示時依需要 `new Date(s).toLocaleString('zh-TW', {timeZone: 'Asia/Taipei'})`
- `work_date` / `bucket` 是 **純日期字串**：`YYYY-MM-DD`（已用 Asia/Taipei 計算）

### 1.3 統一錯誤格式
```json
{ "error": "human-readable message" }
```
某些 endpoint 會多帶診斷欄位（如 manager-team 403 帶 `badge_id`）。

| HTTP code | 意義 |
|---|---|
| 400 | request 缺必填參數 / 格式錯 |
| 401 | 缺 Authorization header 或 token 無效（`DEV_AUTH_BYPASS=0` 模式才會出現）|
| 403 | 已驗證身份但無權限（FR-9）|
| 500 | server 內部錯誤（DB query 失敗等） |

### 1.4 認證
| 模式 | 設定 | 前端行為 |
|---|---|---|
| Dev / Demo（**目前 docker-compose 預設**）| `DEV_AUTH_BYPASS=1` | 不帶 token，加 `?as=<badge_id>` query 切換 caller 視角 |
| Production / 演示 OIDC | `DEV_AUTH_BYPASS=0` | 必須帶 `Authorization: Bearer <jwt>`；無 token 401 |

`?as=` 在 dev mode 下與 token 等價，只用於 demo manager-team / trend / alerts 等需要身份的 endpoint。

### 1.5 空集合
- 若 query 沒命中任何資料，**回 `[]` 或 `{trends: [], reports: []}`**，**永遠不回 `null`**
- 前端可放心做 `.map()` / `.length` 不必檢查 null

### 1.6 數值 / 布林
- 整數欄（`swipe_count` / `head_count` / `total_swipes` / `id`）：JSON number（整數）
- 浮點數（`stay_hours` / `avg_stay_hrs`）：JSON number；精度保留小數（範例：`10.957523371944445`），前端用 `.toFixed(2)` 顯示

---

## 2. `GET /v1/reports/attendance` — FR-5 員工出勤

### 2.1 Request

| 參數 | 位置 | 必填 | 型別 | 說明 |
|---|---|:---:|---|---|
| `date` | query | - | `YYYY-MM-DD` | 限定該日（台北時區）；不帶 → 回多日全部 |
| `as` | query | - | `string` | demo 模式切換 caller，主要影響 attendance 不大（沒 scope filter） |

### 2.2 Response shape（200）

回傳 **JSON array**，每筆是 `AttendanceRow`：

| 欄位 | 型別 | Nullable | 說明 |
|---|---|:---:|---|
| `employee_id` | string | ❌ | 員工卡號 |
| `name` | string | ❌ | 員工姓名（無 employees row 時為 `"Employee <badge>"` fallback） |
| `org_path` | string | ❌ | 員工所屬部門（中文 dot-separated；無 employees row 時為 `"Unknown"`） |
| `work_date` | string `YYYY-MM-DD` | ❌ | 該員工該日的台北日期 |
| `first_in` | string RFC3339 | ⚪ | 當日首次 `direction=IN`；若該日無 IN 則 null（JSON 中欄位不存在） |
| `last_out` | string RFC3339 | ⚪ | 當日最後 `direction=OUT`；若該日無 OUT 則 null |
| `swipe_count` | int | ❌ | 該員工該日 SUCCESS 刷卡總次數 |
| `stay_hours` | float | ❌ | `(last_out - first_in)` 小時數；若 first_in 或 last_out 缺一則為 `0` |

### 2.3 範例（success）

```json
[
  {
    "employee_id": "B001",
    "name": "王小明",
    "org_path": "TSMC.Fab12.製造部",
    "work_date": "2026-05-11",
    "first_in": "2026-05-11T01:00:00Z",
    "last_out": "2026-05-11T11:57:27.084139Z",
    "swipe_count": 5,
    "stay_hours": 10.957523371944445
  },
  {
    "employee_id": "B002",
    "name": "李大華",
    "org_path": "TSMC.Fab12.品保部",
    "work_date": "2026-05-11",
    "first_in": "2026-05-11T01:00:00Z",
    "last_out": "2026-05-11T10:00:00Z",
    "swipe_count": 4,
    "stay_hours": 9
  }
]
```

### 2.4 空集合（example: `?date=2099-01-01`）

```json
[]
```

### 2.5 錯誤情境

| 條件 | HTTP | Body |
|---|---|---|
| DB query failed | 500 | `{"error": "Failed to query attendance"}` |

---

## 3. `GET /v1/reports/manager-team` — FR-6 + FR-9 主管報表

### 3.1 Request

| 參數 | 位置 | 必填 | 說明 |
|---|---|:---:|---|
| `date` | query | - | `YYYY-MM-DD` 限定該日 |
| `as` | query | demo 必填 | caller 的 badge_id；prod 模式改由 JWT `badge_id` claim 提供 |

### 3.2 Response shape（200）

```typescript
{
  manager_scope: string;           // 例 "TSMC.Fab12"，caller 的 ltree scope
  reports:       AttendanceRow[];  // 該 scope 子樹下所有員工的 attendance（同 §2.2 結構）
}
```

| 欄位 | 型別 | Nullable | 說明 |
|---|---|:---:|---|
| `manager_scope` | string (ltree path) | ❌ | caller `employees.org_path_ltree` |
| `reports` | `AttendanceRow[]` | ❌ | 子樹下所有員工的逐日 attendance（可能空陣列） |

### 3.3 範例 — 廠長 B100（scope `TSMC.Fab12`）

```json
{
  "manager_scope": "TSMC.Fab12",
  "reports": [
    {
      "employee_id": "B001",
      "name": "王小明",
      "org_path": "TSMC.Fab12.製造部",
      "work_date": "2026-05-11",
      "first_in": "2026-05-11T01:00:00Z",
      "last_out": "2026-05-11T11:57:27.084139Z",
      "swipe_count": 5,
      "stay_hours": 10.957523371944445
    }
  ]
}
```

### 3.4 範例 — 部主管 B001（scope `TSMC.Fab12.製造部`）

```json
{
  "manager_scope": "TSMC.Fab12.製造部",
  "reports": [
    // 只剩製造部員工（B001 / B011 / B012）
  ]
}
```

### 3.5 錯誤情境

| 條件 | HTTP | Body |
|---|---|---|
| caller 是非主管或不在職 | **403** | `{"badge_id": "B011", "error": "not a manager"}` |
| token 內無 badge_id（prod 模式）| 401 | `{"error": "missing badge_id in token"}` |
| caller 是 manager 但 DB 查詢失敗 | 500 | `{"error": "scope lookup failed: ..."}` / `{"error": "query failed: ..."}` |

---

## 4. `GET /v1/reports/trend` — FR-7 出勤趨勢

### 4.1 Request

| 參數 | 位置 | 必填 | 預設 | 說明 |
|---|---|:---:|---|---|
| `period` | query | - | `day` | `day` / `week` / `month` / `quarter` |
| `start_date` | query | - | - | `YYYY-MM-DD` 起始（含）|
| `end_date` | query | - | - | `YYYY-MM-DD` 結束（含）|
| `as` | query | demo 可選 | - | caller badge；若是 manager 則自動限縮 scope，否則不限 |

### 4.2 Response shape（200）

```typescript
{
  period: "day" | "week" | "month" | "quarter";
  scope:  string;             // caller 的 ltree scope；非 manager 時為 ""
  trends: TrendBucket[];
}

interface TrendBucket {
  bucket:       string;       // YYYY-MM-DD，該 bucket 的起始日
  head_count:   number;       // 該 bucket 內 distinct badge 人頭數
  avg_stay_hrs: number;       // 該 bucket 內所有員工 stay_hours 平均
  total_swipes: number;       // 該 bucket 內 sum(swipe_count)
}
```

### 4.3 範例 — 廠長 B100 day

```json
{
  "period": "day",
  "scope":  "TSMC.Fab12",
  "trends": [
    { "bucket": "2026-05-11", "head_count": 2, "avg_stay_hrs": 9.978761685972222, "total_swipes": 9 },
    { "bucket": "2026-05-10", "head_count": 2, "avg_stay_hrs": 9,                 "total_swipes": 4 },
    { "bucket": "2026-05-09", "head_count": 2, "avg_stay_hrs": 9,                 "total_swipes": 4 }
  ]
}
```

### 4.4 範例 — 非主管視角（scope 空 → 全公司）

```json
{
  "period": "month",
  "scope":  "",
  "trends": [
    { "bucket": "2026-05-01", "head_count": 58, "avg_stay_hrs": 8.55, "total_swipes": 10045 }
  ]
}
```

### 4.5 範例 — week / month / quarter

| period | bucket 規則 |
|---|---|
| `day` | bucket = 該日（`YYYY-MM-DD`）|
| `week` | bucket = 該週週一（ISO 週起點）|
| `month` | bucket = 該月 1 號 |
| `quarter` | bucket = 該季首月 1 號（01/04/07/10）|

### 4.6 錯誤情境

| 條件 | HTTP | Body |
|---|---|---|
| DB query failed | 500 | `{"error": "trend query failed: ..."}` |

### 4.7 注意：MV 延遲

底層讀 `mv_daily_attendance` materialized view，由 `mv-refresher` 每 5 min refresh。
新刷卡資料可能晚 5 min 才反映；要強制 refresh：

```bash
docker compose exec postgres psql -U pacs_user -d pacs_db \
  -c "REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance;"
```

---

## 5. `GET /v1/audit` — FR-13 Audit Trail

### 5.1 Request

| 參數 | 位置 | 必填 | 預設 | 說明 |
|---|---|:---:|---|---|
| `badge_id` | query | ✅ | - | 員工卡號 |
| `start_date` | query | - | 今天 | `YYYY-MM-DD`（包含）|
| `end_date` | query | - | 今天 | `YYYY-MM-DD`（包含）|

### 5.2 Response shape（200）

回 `AccessEvent[]`（包含 SUCCESS + REJECTED 等所有狀態）：

| 欄位 | 型別 | Nullable | 說明 |
|---|---|:---:|---|
| `id` | int | ❌ | 事件 ID（連續 sequence） |
| `badge_id` | string | ❌ | 員工卡號 |
| `site_id` | string | ❌ | 廠區 |
| `gate_id` | string | ❌ | 閘門 |
| `direction` | `"IN" \| "OUT"` | ❌ | 方向 |
| `status` | string | ❌ | `SUCCESS` / `REJECTED_APB` / `DENIED` / 其他擴充 |
| `reason` | string | ⚪ | 拒絕或備註原因（FR-3） |
| `timestamp` | string RFC3339 | ❌ | 事件發生時間（UTC） |

### 5.3 範例

```json
[
  {
    "id": 65,
    "badge_id": "B001",
    "site_id": "Site-A",
    "gate_id": "Gate-1",
    "direction": "OUT",
    "status": "REJECTED_APB",
    "reason": "Anti-Passback Violation",
    "timestamp": "2026-05-11T11:57:27.249415Z"
  },
  {
    "id": 64,
    "badge_id": "B001",
    "site_id": "Site-A",
    "gate_id": "Gate-1",
    "direction": "IN",
    "status": "SUCCESS",
    "reason": "",
    "timestamp": "2026-05-11T11:57:27.084139Z"
  }
]
```

### 5.4 錯誤情境

| 條件 | HTTP | Body |
|---|---|---|
| 缺 `badge_id` | **400** | `{"error": "badge_id is required"}` |
| DB query failed | 500 | `{"error": "Failed to query audit trail"}` |

---

## 6. `GET /v1/alerts` — FR-11 異常警報

### 6.1 Request

| 參數 | 位置 | 必填 | 預設 | 說明 |
|---|---|:---:|---|---|
| `open` | query | - | `false` | `true` 時只回未處理（`resolved_at IS NULL`） |
| `limit` | query | - | `100` | 最大筆數（cap 500） |

### 6.2 Response shape（200）

回 `Alert[]`：

| 欄位 | 型別 | Nullable | 說明 |
|---|---|:---:|---|
| `id` | int | ❌ | 警報 ID |
| `alert_type` | enum string | ❌ | `OFF_HOURS_ENTRY` / `APB_BURST` / `TAILGATING` / `STAT_OUTLIER` |
| `severity` | enum string | ❌ | `LOW` / `MEDIUM` / `HIGH` / `CRITICAL` |
| `badge_id` | string | ⚪ | 對應員工（部分異常無對應人員，此時欄位省略） |
| `site_id` | string | ⚪ | 廠區（同上） |
| `gate_id` | string | ⚪ | 閘門 |
| `details` | string (JSON)  | ❌ | 規則特定 metadata（**注意是 raw JSON string**，前端需 `JSON.parse` 才能取裡面欄位） |
| `occurred_at` | string RFC3339 | ❌ | 發生時間 |
| `resolved_at` | string RFC3339 | ⚪ | 處理時間；`null` 表示未處理 |

### 6.3 範例

```json
[
  {
    "id": 7,
    "alert_type": "APB_BURST",
    "severity": "HIGH",
    "badge_id": "V_ALERT",
    "site_id": "Site-A",
    "gate_id": "Gate-X",
    "details": "{\"count_window_minutes\": 30}",
    "occurred_at": "2026-05-11T11:58:33.769113Z"
  }
]
```

### 6.4 `details` 的內容（依 `alert_type` 不同）

| alert_type | details 內容 |
|---|---|
| `OFF_HOURS_ENTRY` | `{"event_time": "<RFC3339>", "direction": "IN"}` |
| `APB_BURST` | `{"count_window_minutes": 30}` |
| `TAILGATING` | `{"count_window_seconds": 5}` |
| `STAT_OUTLIER` | （保留，未實作） |

### 6.5 錯誤情境

| 條件 | HTTP | Body |
|---|---|---|
| DB query failed | 500 | `{"error": "alerts query failed: ..."}` |

---

## 7. `POST /v1/dev/login` — FR-10 JWT issue

### 7.1 Request

支援 JSON body 與 query string 兩種：

```http
POST /v1/dev/login HTTP/1.1
Content-Type: application/json

{"badge_id": "B100"}
```

或 `POST /v1/dev/login?badge_id=B100`。

### 7.2 Response shape（200）

| 欄位 | 型別 | 說明 |
|---|---|---|
| `access_token` | string | HS256 signed JWT |
| `token_type` | string | 固定 `"Bearer"` |
| `expires_in` | int | 秒數（預設 86400 = 24h） |

### 7.3 範例

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJiYWRnZV9pZCI6IkIxMDAiLCJpc3MiOiJwYWNzLWRldiIsInN1YiI6IkIxMDAiLCJleHAiOjE3Nzg2NjQ2OTAsImlhdCI6MTc3ODU3ODI5MH0.fLeEr12LBn5zBUHlVwfJXRbimN77gpd1MoMSQ1WVl74",
  "token_type": "Bearer",
  "expires_in": 86400
}
```

### 7.4 JWT payload（decode base64 後）

```json
{
  "badge_id": "B100",
  "iss":      "pacs-dev",
  "sub":      "B100",
  "exp":      1778664690,
  "iat":      1778578290
}
```

### 7.5 錯誤情境

| 條件 | HTTP | Body |
|---|---|---|
| 缺 `badge_id` | 400 | `{"error": "badge_id is required"}` |
| Issue 失敗 | 500 | `{"error": "<jwt sign error>"}` |

---

## 8. `GET /v1/reports/attendance/export` — FR-8 Excel 匯出

**回傳形式：binary .xlsx，不是 JSON**。但為完整起見列在這裡。

### 8.1 Request

| 參數 | 位置 | 必填 | 說明 |
|---|---|:---:|---|
| `format` | query | ✅ | 必須是 `excel` 或 `xlsx`（PDF 暫未實作）|
| `date` | query | - | 限定該日；若有 caller scope 則仍會應用 |

### 8.2 Response headers（200）

```
Content-Type:        application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
Content-Disposition: attachment; filename="attendance-YYYYMMDD-HHMMSS.xlsx"
```

### 8.3 Workbook 結構

- 一個工作表：`Attendance`
- 第 1 列表頭、第 2 列起為資料列
- 欄位順序（與 §2.2 `AttendanceRow` 對齊）：

| col | header | 範例值 |
|---|---|---|
| A | `Employee ID` | `B001` |
| B | `Name` | `王小明` |
| C | `Org Path` | `TSMC.Fab12.製造部` |
| D | `Work Date` | `2026-05-11` |
| E | `First In` | `2026-05-11T01:00:00Z`（RFC3339）|
| F | `Last Out` | `2026-05-11T11:57:27Z` |
| G | `Swipes` | `5` |
| H | `Stay Hours` | `10.96` |

### 8.4 錯誤情境

| 條件 | HTTP | Body |
|---|---|---|
| `format` 不是 `excel` | 400 | `{"error": "only format=excel supported in this phase"}` |
| DB query failed | 500 | `{"error": "query failed: ..."}` |

---

## 9. TypeScript 型別定義（給前端複製貼上）

```typescript
// ============ 通用 ============
type ISODate     = string;  // "YYYY-MM-DD"
type ISODateTime = string;  // "YYYY-MM-DDTHH:MM:SS[.ffffff]Z" (RFC 3339, UTC)

interface ApiError {
  error: string;
  [key: string]: unknown;     // manager-team 403 額外帶 badge_id
}

// ============ §2 attendance ============
interface AttendanceRow {
  employee_id: string;
  name:        string;
  org_path:    string;
  work_date:   ISODate;
  first_in?:   ISODateTime;   // optional：當日無 IN 時欄位不存在
  last_out?:   ISODateTime;
  swipe_count: number;
  stay_hours:  number;
}

// ============ §3 manager-team ============
interface ManagerTeamResponse {
  manager_scope: string;       // ltree path string
  reports:       AttendanceRow[];
}

// ============ §4 trend ============
type TrendPeriod = "day" | "week" | "month" | "quarter";

interface TrendBucket {
  bucket:       ISODate;
  head_count:   number;
  avg_stay_hrs: number;
  total_swipes: number;
}

interface TrendResponse {
  period: TrendPeriod;
  scope:  string;              // ltree path or "" for non-manager
  trends: TrendBucket[];
}

// ============ §5 audit ============
type Direction = "IN" | "OUT";

interface AccessEvent {
  id:        number;
  badge_id:  string;
  site_id:   string;
  gate_id:   string;
  direction: Direction;
  status:    string;           // "SUCCESS" | "REJECTED_APB" | ...
  reason?:   string;
  timestamp: ISODateTime;
}

// ============ §6 alerts ============
type AlertType     = "OFF_HOURS_ENTRY" | "APB_BURST" | "TAILGATING" | "STAT_OUTLIER";
type AlertSeverity = "LOW" | "MEDIUM" | "HIGH" | "CRITICAL";

interface Alert {
  id:          number;
  alert_type:  AlertType;
  severity:    AlertSeverity;
  badge_id?:   string;
  site_id?:    string;
  gate_id?:    string;
  details:     string;         // JSON-encoded string；用 JSON.parse 取裡面欄位
  occurred_at: ISODateTime;
  resolved_at?: ISODateTime;
}

// ============ §7 dev login ============
interface LoginRequest {
  badge_id: string;
}

interface LoginResponse {
  access_token: string;
  token_type:   "Bearer";
  expires_in:   number;        // seconds
}
```

---

## 10. JSON Schema（給 contract test）

下面是核心物件的 [JSON Schema Draft 2020-12](https://json-schema.org/) 定義，
可餵給 `ajv` / `jsonschema` 做 frontend → backend wire format 的 contract validation。

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://pacs/api/AttendanceRow.json",
  "type": "object",
  "required": ["employee_id", "name", "org_path", "work_date", "swipe_count", "stay_hours"],
  "properties": {
    "employee_id": { "type": "string" },
    "name":        { "type": "string" },
    "org_path":    { "type": "string" },
    "work_date":   { "type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$" },
    "first_in":    { "type": "string", "format": "date-time" },
    "last_out":    { "type": "string", "format": "date-time" },
    "swipe_count": { "type": "integer", "minimum": 0 },
    "stay_hours":  { "type": "number",  "minimum": 0 }
  }
}
```

```json
{
  "$id": "https://pacs/api/TrendBucket.json",
  "type": "object",
  "required": ["bucket", "head_count", "avg_stay_hrs", "total_swipes"],
  "properties": {
    "bucket":       { "type": "string", "pattern": "^\\d{4}-\\d{2}-\\d{2}$" },
    "head_count":   { "type": "integer", "minimum": 0 },
    "avg_stay_hrs": { "type": "number",  "minimum": 0 },
    "total_swipes": { "type": "integer", "minimum": 0 }
  }
}
```

```json
{
  "$id": "https://pacs/api/Alert.json",
  "type": "object",
  "required": ["id", "alert_type", "severity", "details", "occurred_at"],
  "properties": {
    "id":         { "type": "integer", "minimum": 1 },
    "alert_type": { "enum": ["OFF_HOURS_ENTRY", "APB_BURST", "TAILGATING", "STAT_OUTLIER"] },
    "severity":   { "enum": ["LOW", "MEDIUM", "HIGH", "CRITICAL"] },
    "badge_id":   { "type": "string" },
    "site_id":    { "type": "string" },
    "gate_id":    { "type": "string" },
    "details":    { "type": "string" },
    "occurred_at": { "type": "string", "format": "date-time" },
    "resolved_at": { "type": ["string", "null"], "format": "date-time" }
  }
}
```

```json
{
  "$id": "https://pacs/api/AccessEvent.json",
  "type": "object",
  "required": ["id", "badge_id", "site_id", "gate_id", "direction", "status", "timestamp"],
  "properties": {
    "id":        { "type": "integer", "minimum": 1 },
    "badge_id":  { "type": "string" },
    "site_id":   { "type": "string" },
    "gate_id":   { "type": "string" },
    "direction": { "enum": ["IN", "OUT"] },
    "status":    { "type": "string" },
    "reason":    { "type": "string" },
    "timestamp": { "type": "string", "format": "date-time" }
  }
}
```

---

## 11. 改動歷史

| 日期 | 改動 |
|---|---|
| 2026-05-13 | 初版，對應 PR #2 merged 後的 Phase 2 API |

未來新增 endpoint 請：
1. 加進 §0 總覽表
2. 新增一節說明 request / response shape
3. 加 TypeScript 與 JSON Schema 定義
4. 更新本表
