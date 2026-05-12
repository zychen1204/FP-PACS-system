# Phase 2 前端組員整合指引

> 寫給：負責 `frontend/` 的組員
> 目的：把 Phase 2 後端新增的 5 個 endpoint、JWT 認證、anomaly 警報等整合進現有 UI
> 互補文件：
> - 後端做了什麼 → [`PHASE2_CHANGES.md`](PHASE2_CHANGES.md)
> - 怎麼驗收 → [`PHASE2_VERIFICATION.md`](PHASE2_VERIFICATION.md)

---

## 目錄

- [0. TL;DR](#0-tldr)
- [1. 開發環境設置](#1-開發環境設置)
- [2. 既有 endpoint 兼容性檢查](#2-既有-endpoint-兼容性檢查)
- [3. 5 個新 endpoint 完整 API 文件](#3-5-個新-endpoint-完整-api-文件)
- [4. 認證流程（dev vs production）](#4-認證流程dev-vs-production)
- [5. nginx proxy 還缺哪兩條](#5-nginx-proxy-還缺哪兩條)
- [6. UI 規劃建議](#6-ui-規劃建議)
- [7. 程式碼 snippet 範本](#7-程式碼-snippet-範本)
- [8. 常見問題與解法](#8-常見問題與解法)
- [9. 測試與驗收 checklist](#9-測試與驗收-checklist)

---

## 0. TL;DR

| 你需要做的事 | 等級 |
|---|:---:|
| 既有刷卡頁、出席報表頁 **完全不用改**（CORS / endpoint 路徑都保留） | 🟢 |
| 加 2 條 nginx proxy（`/v1/alerts`、`/v1/dev/login`） | 🟡 必要 |
| 新增「主管視野」分頁顯示 `manager-team` 結果 | 🟡 規格要求 (FR-6) |
| 新增「趨勢圖」分頁，呼叫 `/v1/reports/trend` 畫 chart | 🟡 規格要求 (FR-7) |
| 出席報表頁加「下載 Excel」按鈕 | 🟡 規格要求 (FR-8) |
| 新增「警報」分頁顯示 `/v1/alerts` 列表 | 🟡 規格要求 (FR-11) |
| Demo 模式下不用處理 JWT；frontend 帶 `?as=B100` 即可切換主管視角 | 🟢 |
| Production 模式（OIDC 真實流程）才需要做 login 頁 + 存 token + Authorization header | 🟡 未來功能 |

**重點**：Phase 1 的程式碼一行不用改 — 後端 `reporting-api` 設了 `DEV_AUTH_BYPASS=1`，所有既有呼叫都還會 200。

---

## 1. 開發環境設置

### 1.1 啟動整個 stack（含 backend + frontend）

```bash
cd final/FP-PACS-system
docker compose down -v          # 清乾淨
docker compose up -d            # 起 10 個 service
sleep 25                        # 等 migrate + 各 service ready
open http://localhost           # 看 frontend
```

### 1.2 frontend 開發迴圈（只改 frontend code）

```bash
# 改 frontend/app.js / index.html / style.css 之後：
docker compose up -d --build frontend          # 重新 build & 起 frontend container
# 不影響其他 service
```

或如果你想跳過 docker、用本機開發伺服器：

```bash
cd frontend
python3 -m http.server 8000                    # 或 npx serve
# 然後在「設定」分頁把 API 網址改成 http://localhost:8080 / http://localhost:8081
```

### 1.3 端口對照

| 服務 | 內部 port | host 對外 | 用途 |
|---|---|---|---|
| frontend (nginx) | 80 | **80** | UI + reverse proxy |
| access-api | 8080 | 8080 | `/v1/swipe` 寫入 |
| reporting-api | 8081 | 8081 | 所有 `/v1/reports/*` + `/v1/audit` + `/v1/alerts` + `/v1/dev/login` |
| anomaly-detector | 8083 | — | （後端 only，frontend 不直接打）|
| mv-refresher | 8084 | — | （後端 only）|
| org-sync | 8085 | — | （後端 only）|
| postgres | 5432 | 5432 | （後端 only）|
| redis | 6379 | 6379 | （後端 only）|

### 1.4 你的 fetch 應該打哪個 URL？

**走 nginx**（推薦，正式部署也是這條路）：
```js
fetch('/v1/swipe', ...)                 // → access-api
fetch('/v1/reports/attendance', ...)    // → reporting-api
fetch('/v1/alerts', ...)                // → reporting-api（需先補 nginx proxy，§5）
```

**直接打 backend**（dev/local 用，避免 CORS 開太多）：
```js
fetch('http://localhost:8080/v1/swipe', ...)
fetch('http://localhost:8081/v1/reports/attendance', ...)
```

CORS 已開（reporting-api / access-api 都設了 `Access-Control-Allow-Origin: *`、`Methods: POST OPTIONS GET`、`Headers: Content-Type, Authorization`），跨域 fetch 直接可以。

---

## 2. 既有 endpoint 兼容性檢查

### 2.1 全部向後相容 ✅

| Endpoint | 變動 | frontend 要改？ |
|---|---|:---:|
| `POST /v1/swipe` | request / response shape 完全沒變 | ❌ |
| `GET /v1/reports/attendance` | response shape 完全沒變；底層 query 改命中 index（更快）| ❌ |
| `GET /v1/audit` | 同上 | ❌ |
| `GET /api/healthz` | 同上 | ❌ |
| `GET /api/report-healthz` | 同上 | ❌ |

### 2.2 一個你可能會注意到的差異

`/v1/reports/attendance` 的 `stay_hours` 之前因為 dev_seed 的 timezone bug 偶爾會是 `null`。Phase 2 修了，現在永遠是數字（或 0）。如果你之前對 `null` 做了 fallback 顯示「-」，現在會永遠顯示數字（包含可能是 0）— 行為不變，但顯示「0.0 hr」比「-」明顯。

### 2.3 認證影響

`/v1/reports/*` 與 `/v1/audit` 現在掛了 `auth.Middleware()`，但 `docker-compose.yml` 設了：

```yaml
reporting-api:
  environment:
    - DEV_AUTH_BYPASS=1
```

效果：middleware 直接放行；額外可以用 `?as=B100` 或 `?as=B001` query 切換「主管視角」。**你的既有程式碼不需要改任何東西**。

---

## 3. 5 個新 endpoint 完整 API 文件

> 統一 base URL：`http://localhost:8081`（直接打）或 `http://localhost/`（走 nginx，需先補 proxy）
> 所有 endpoint 在 demo 模式下不需帶 JWT；想切換視角加 `?as=<badge_id>` query。

### 3.1 `POST /v1/dev/login` — 簽發 JWT（FR-10）

**用途**：Demo IdP，給定 `badge_id` 換一個 HS256 JWT。正式環境會被替換成真 OIDC redirect。

**Request**
```http
POST /v1/dev/login HTTP/1.1
Content-Type: application/json

{"badge_id": "B100"}
```

也接 query string：`POST /v1/dev/login?badge_id=B100`

**Response 200**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "token_type": "Bearer",
  "expires_in": 86400
}
```

**Response 400**：missing `badge_id`

**前端用法（demo 階段不需要，但留底以備未來 OIDC）**
```js
const res = await fetch('/v1/dev/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ badge_id: 'B100' })
});
const { access_token } = await res.json();
localStorage.setItem('pacs_token', access_token);
```

### 3.2 `GET /v1/reports/manager-team` — 階層團隊報表（FR-6 / FR-9）

**用途**：主管查看自己所屬團隊（含子部門）的出席資料。非主管 caller 會被擋下回 403。

**Query params**
| 參數 | 必填 | 說明 |
|---|---|---|
| `as` | demo 模式可選 | 模擬 caller badge（DEV_AUTH_BYPASS=1 時生效）|
| `date` | 選 | 限定日期，格式 `YYYY-MM-DD`；不傳則全部 |

**Response 200**（廠長 B100 視野）
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
      "last_out": "2026-05-11T10:00:00Z",
      "swipe_count": 4,
      "stay_hours": 9
    },
    ...
  ]
}
```

**Response 403**（非主管）
```json
{"badge_id": "B011", "error": "not a manager"}
```

**前端用法**
```js
// 廠長視角（看整個 Fab12）
fetch('/v1/reports/manager-team?as=B100')
// 部主管視角（只看自己部門）
fetch('/v1/reports/manager-team?as=B001')
// 一般員工 → 403
fetch('/v1/reports/manager-team?as=B011')
```

### 3.3 `GET /v1/reports/trend` — 出勤趨勢（FR-7）

**用途**：依日 / 週 / 月 / 季維度看人力趨勢。底層讀 `mv_daily_attendance` materialized view，所以即時但有 ~5 min 延遲。

**Query params**
| 參數 | 必填 | 說明 |
|---|---|---|
| `period` | 選（預設 `day`）| `day` / `week` / `month` / `quarter` |
| `as` | demo 模式可選 | caller badge；若是主管，trend 會自動限縮在其 ltree scope |
| `start_date` | 選 | `YYYY-MM-DD` |
| `end_date` | 選 | `YYYY-MM-DD` |

**Response 200**
```json
{
  "period": "day",
  "scope": "TSMC.Fab12",
  "trends": [
    {"bucket": "2026-05-11", "head_count": 2, "avg_stay_hrs": 9, "total_swipes": 8},
    {"bucket": "2026-05-10", "head_count": 2, "avg_stay_hrs": 9, "total_swipes": 4},
    {"bucket": "2026-05-09", "head_count": 2, "avg_stay_hrs": 9, "total_swipes": 4}
  ]
}
```

`bucket` 是該 period 的起始日期。Week 用 ISO 週一、month 用月初、quarter 用季初。

**畫圖建議**
- X 軸：`bucket`（日期）
- Y 軸：可選 `head_count`（人頭數）/ `avg_stay_hrs`（平均停留）/ `total_swipes`（總刷卡次數）
- 用 Chart.js / ECharts / Recharts 都可

### 3.4 `GET /v1/reports/attendance/export` — Excel 匯出（FR-8）

**用途**：把 `/v1/reports/attendance` 的內容打包成 .xlsx 下載。

**Query params**
| 參數 | 必填 | 說明 |
|---|---|---|
| `format` | 必 | 必須是 `excel`（PDF 暫未實作）|
| `date` | 選 | 限定日期 |

**Response 200**
- `Content-Type: application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`
- `Content-Disposition: attachment; filename="attendance-YYYYMMDD-HHMMSS.xlsx"`
- Body：binary .xlsx 檔案

**Response 400**：`format` 不是 `excel`

**前端用法（觸發瀏覽器下載）**
```js
function downloadExcel(date) {
  const url = date
    ? `/v1/reports/attendance/export?format=excel&date=${date}`
    : `/v1/reports/attendance/export?format=excel`;

  // 最簡單：用 <a> 觸發下載
  const link = document.createElement('a');
  link.href = url;
  link.click();
}
```

或如果需要先帶 JWT：
```js
async function downloadExcelWithAuth(date) {
  const res = await fetch(`/v1/reports/attendance/export?format=excel&date=${date}`, {
    headers: { 'Authorization': `Bearer ${localStorage.getItem('pacs_token')}` }
  });
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = res.headers.get('Content-Disposition')?.match(/filename="([^"]+)"/)?.[1] || 'attendance.xlsx';
  a.click();
  URL.revokeObjectURL(url);
}
```

### 3.5 `GET /v1/alerts` — 異常警報列表（FR-11）

**用途**：列 anomaly-detector 寫進 `alerts` 表的異常記錄。

**Query params**
| 參數 | 必填 | 說明 |
|---|---|---|
| `open` | 選 | `true` 只列未處理的（`resolved_at IS NULL`）|
| `limit` | 選（預設 100、上限 500） | 最大筆數 |

**Response 200**
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

**`alert_type` 列舉**

| 值 | 意思 | 觸發條件 |
|---|---|---|
| `OFF_HOURS_ENTRY` | 非工時進入 | 台北 22:00 ~ 06:00 SUCCESS IN |
| `APB_BURST` | APB 連發 | 同 badge 30 分鐘內 REJECTED_APB ≥ 3 |
| `TAILGATING` | 尾隨 | 同 gate 5 秒內 SUCCESS IN ≥ 3 |
| `STAT_OUTLIER` | 統計離群（保留，未實作）| — |

**`severity` 列舉**：`LOW` / `MEDIUM` / `HIGH` / `CRITICAL`

**`details` 注意**：DB 端是 JSONB，但 API 回成 raw JSON string。前端要再 `JSON.parse` 才能取裡面欄位：
```js
const detailObj = JSON.parse(alert.details);
// detailObj.count_window_minutes
```

---

## 4. 認證流程（dev vs production）

### 4.1 Demo / Dev 模式（目前的設定，frontend 不用做任何事）

```yaml
# docker-compose.yml
reporting-api:
  environment:
    - DEV_AUTH_BYPASS=1
```

效果：
- 所有 `/v1/reports/*` 與 `/v1/audit` 直接放行
- 額外好處：query 加 `?as=B100` 可切換主管視角（demo manager-team 必用）
- frontend 不需要存 token、不需要帶 `Authorization` header

### 4.2 演示 OIDC 完整流程（給組長 / TA 看時用）

如果想 demo「沒帶 token → 401、帶 token → 200」這條 FR-10 的故事線：

```bash
# 1) 換 token
TOKEN=$(curl -sX POST -H "Content-Type: application/json" \
  -d '{"badge_id":"B100"}' http://localhost:8081/v1/dev/login \
  | jq -r .access_token)

# 2) 起一個 sidecar reporting-api 用 DEV_AUTH_BYPASS=0
docker compose run --rm -d -e DEV_AUTH_BYPASS=0 -p 8091:8081 \
  --name rpt-auth reporting-api
sleep 4

# 3) 演示 401
curl http://localhost:8091/v1/reports/manager-team
# {"error":"missing or malformed Authorization header"}

# 4) 演示 200
curl -H "Authorization: Bearer $TOKEN" http://localhost:8091/v1/reports/manager-team

# 5) 收拾
docker rm -f rpt-auth
```

### 4.3 真實 production 模式（未來工作，不在這次 Phase 2 scope）

把 `DEV_AUTH_BYPASS=0` 並接真 OIDC provider 時，frontend 要：

1. **加 login 頁**（按鈕 redirect 到 IdP）
2. **callback 處理**：拿到 `access_token` 存 `localStorage`
3. **fetch wrapper** 自動帶 `Authorization: Bearer <token>` header
4. **401 處理**：清 token、回 login 頁
5. **token expiry**：解 JWT 的 `exp`、過期前 refresh

範本程式碼見 §7.3。

---

## 5. nginx proxy 還缺哪兩條

`frontend/nginx.conf` 目前已 cover：
- `/v1/swipe` → access-api ✅
- `/v1/reports/` → reporting-api ✅（含 `/v1/reports/manager-team`、`/v1/reports/trend`、`/v1/reports/attendance/export`，因為前綴吃 `/v1/reports/`）
- `/v1/audit` → reporting-api ✅
- `/api/healthz`、`/api/report-healthz` ✅

**還沒 proxy 的兩條**（請你補）：

```nginx
# Proxy: Alerts (FR-11)
location /v1/alerts {
    proxy_pass http://reporting-api:8081;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}

# Proxy: Dev login (FR-10 demo IdP)
location /v1/dev/login {
    proxy_pass http://reporting-api:8081;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

加完後：

```bash
docker compose up -d --build frontend
# 驗證
curl http://localhost/v1/alerts
curl -X POST -H "Content-Type: application/json" \
  -d '{"badge_id":"B100"}' http://localhost/v1/dev/login
```

> 如果暫時不想動 nginx，frontend code 也可以直接打 `http://localhost:8081/v1/alerts`（CORS 已開），但走 nginx 比較統一。

---

## 6. UI 規劃建議

### 6.1 既有 tab（沿用，不用改）

- 🔑 **刷卡模擬**
- 📊 **出席報表**（加一顆「下載 Excel」按鈕即可，見 §3.4）
- ⚙️ **設定**

### 6.2 建議新增 4 個 tab

#### 👔 主管視野（FR-6 / FR-9）
- 上方下拉選單：切換 caller badge（B100 廠長 / B001~B005 部主管 / B011 員工等）
  - 對應 fetch `?as=<badge_id>`
- 顯示 `manager_scope`（目前以誰的視角）
- 表格：與「出席報表」格式類似但 scope 限縮
- 切到非主管（如 B011）→ 顯示「您不是主管，無權查看」紅字（對應 403 response）

#### 📈 趨勢圖（FR-7）
- 上方 toolbar：period 下拉（day/week/month/quarter）、date range picker、caller `as` 下拉
- 主畫面：折線圖 / 長條圖
  - 三條線 / 系列：`head_count`、`avg_stay_hrs`、`total_swipes`（可獨立 toggle）
- 提示：MV 5 分鐘 refresh 一次，最新刷卡可能晚 5 分鐘才反映
- 推薦套件：[Chart.js](https://www.chartjs.org/)（最輕量、vanilla JS 友善）

#### 🚨 警報（FR-11）
- 預設只顯示 `open=true`（未處理）
- 上方 toggle：「只顯示未處理 / 顯示全部」
- 表格欄：時間、類型（含徽章顯示嚴重度顏色）、badge、地點、details（縮排 JSON）
- 嚴重度顏色建議：
  - `LOW` → 灰
  - `MEDIUM` → 黃
  - `HIGH` → 橙
  - `CRITICAL` → 紅
- 自動 refresh：每 30 秒 fetch 一次

#### 🔐 登入（未來 OIDC 流程，現在可以先預備 UI）
- 「以 badge ID 登入」輸入框 + 按鈕
- 按下後 POST `/v1/dev/login`、把 token 存 localStorage、顯示「目前以 BadgeID 登入」
- 預備 production OIDC 切換

### 6.3 推薦 UI mockup

```
┌──────────────────────────────────────────────────────┐
│ 🔐 PACS - 分散式門禁管理系統          [● 線上] B100▼  │
├──────────────────────────────────────────────────────┤
│ 🔑 刷卡  📊 報表  👔 主管  📈 趨勢  🚨 警報  🔐 設定 │
├──────────────────────────────────────────────────────┤
│  (各分頁內容)                                         │
└──────────────────────────────────────────────────────┘
```

右上 `B100▼` 是「目前 caller」下拉，所有 tab 的 fetch 都跟著它（傳 `as=`）。

---

## 7. 程式碼 snippet 範本

### 7.1 共用 API client（簡化 fetch + 自動帶 token）

```js
// frontend/api.js
const API_BASE = ''; // 走 nginx；本機開發改成 'http://localhost:8081'

function authHeaders() {
  const token = localStorage.getItem('pacs_token');
  return token ? { 'Authorization': `Bearer ${token}` } : {};
}

function asQuery() {
  const as = localStorage.getItem('pacs_as_badge');  // 目前選定的 caller
  return as ? `as=${encodeURIComponent(as)}` : '';
}

async function apiGet(path, params = {}) {
  const qs = new URLSearchParams(params);
  const asQ = asQuery();
  if (asQ) qs.append('as', localStorage.getItem('pacs_as_badge'));
  const url = `${API_BASE}${path}?${qs.toString()}`;
  const res = await fetch(url, { headers: { ...authHeaders() } });
  if (res.status === 401) { handle401(); throw new Error('Unauthorized'); }
  if (res.status === 403) return { __forbidden: true, ...(await res.json()) };
  if (!res.ok) throw new Error(`API ${res.status}: ${await res.text()}`);
  return res.json();
}

async function apiPost(path, body) {
  const res = await fetch(`${API_BASE}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(`API ${res.status}`);
  return res.json();
}
```

### 7.2 取主管團隊報表

```js
async function loadManagerTeam() {
  const data = await apiGet('/v1/reports/manager-team');
  if (data.__forbidden) {
    showError('您不是主管，無權查看（HTTP 403）');
    return;
  }
  renderManagerScope(data.manager_scope);  // 例如 "TSMC.Fab12"
  renderReports(data.reports);
}
```

### 7.3 取趨勢資料 + 用 Chart.js 畫圖

```js
async function loadTrend(period = 'day') {
  const data = await apiGet('/v1/reports/trend', { period });
  drawChart(data.trends);
}

function drawChart(trends) {
  new Chart(document.getElementById('trend-chart'), {
    type: 'line',
    data: {
      labels: trends.map(t => t.bucket).reverse(),
      datasets: [
        { label: '人頭數', data: trends.map(t => t.head_count).reverse() },
        { label: '平均停留(hr)', data: trends.map(t => t.avg_stay_hrs).reverse() },
        { label: '總刷卡次數', data: trends.map(t => t.total_swipes).reverse() },
      ],
    },
  });
}
```

### 7.4 警報列表 + 嚴重度標籤

```js
const SEVERITY_COLOR = { LOW: '#888', MEDIUM: '#f0a020', HIGH: '#f06020', CRITICAL: '#d02020' };

async function loadAlerts(openOnly = true) {
  const alerts = await apiGet('/v1/alerts', { open: openOnly });
  const tbody = document.getElementById('alerts-tbody');
  tbody.innerHTML = alerts.map(a => {
    const details = JSON.parse(a.details || '{}');
    return `
      <tr>
        <td>${new Date(a.occurred_at).toLocaleString('zh-TW')}</td>
        <td><span class="badge" style="background:${SEVERITY_COLOR[a.severity]}">${a.severity}</span> ${a.alert_type}</td>
        <td>${a.badge_id ?? '-'}</td>
        <td>${a.site_id ?? '-'} / ${a.gate_id ?? '-'}</td>
        <td><code>${JSON.stringify(details)}</code></td>
      </tr>
    `;
  }).join('') || '<tr><td colspan="5">目前沒有未處理警報</td></tr>';
}

// 自動每 30s refresh
setInterval(() => loadAlerts(true), 30_000);
```

### 7.5 切換 caller 視角（demo 用 `?as=` 機制）

```js
function setActiveCaller(badgeId) {
  localStorage.setItem('pacs_as_badge', badgeId);
  // 觸發各分頁 refetch
  document.dispatchEvent(new CustomEvent('caller-changed'));
}

// drop-down 範例
document.getElementById('caller-select').addEventListener('change', e => {
  setActiveCaller(e.target.value);
});

document.addEventListener('caller-changed', () => {
  // 各分頁監聽這個事件、自己 refetch
  if (currentTab === 'manager') loadManagerTeam();
  if (currentTab === 'trend')   loadTrend();
});
```

### 7.6 真實 OIDC 登入流程（未來用）

```js
// 第 1 步：login button → POST /v1/dev/login
async function login(badgeId) {
  const { access_token } = await apiPost('/v1/dev/login', { badge_id: badgeId });
  localStorage.setItem('pacs_token', access_token);
  document.dispatchEvent(new CustomEvent('login-success'));
}

// 第 2 步：401 處理
function handle401() {
  localStorage.removeItem('pacs_token');
  showLoginModal();
}

// 第 3 步：token expiry 檢查（解 JWT 的 exp claim）
function isTokenExpired() {
  const token = localStorage.getItem('pacs_token');
  if (!token) return true;
  try {
    const payload = JSON.parse(atob(token.split('.')[1]));
    return Date.now() / 1000 > payload.exp;
  } catch { return true; }
}
```

---

## 8. 常見問題與解法

### Q1：fetch `/v1/alerts` 回 404

A：nginx 還沒 proxy `/v1/alerts`。見 §5 補 proxy，或暫時改打 `http://localhost:8081/v1/alerts`。

### Q2：manager-team 永遠回 200，從來不會 403

A：你目前的 `?as=` 是主管 badge（B100 / B001~B005）。試 `?as=B011` 或 `?as=B012`（部員）應該回 403。

### Q3：trend 圖一直空

A：mv-refresher 每 5 分鐘才 refresh 一次 MV。新刷卡資料要等下次 tick 才會出現。強制 refresh：
```bash
docker compose exec postgres psql -U pacs_user -d pacs_db \
  -c "REFRESH MATERIALIZED VIEW CONCURRENTLY mv_daily_attendance;"
```

### Q4：CORS 錯誤

A：Phase 2 後端兩個 API 都已開 `Access-Control-Allow-Origin: *`、`Methods: POST OPTIONS GET`、`Headers: Content-Type, Authorization`。如果還是出問題，檢查：
- frontend 從哪個 origin 發（`localhost:80` vs `localhost:8000` vs `file://`）
- preflight OPTIONS 是否被 nginx 攔住 — 不會（沒設攔截）

### Q5：怎麼觸發一個 alert 來測警報頁？

A：連刷 4 次同方向：
```bash
for i in 1 2 3 4; do
  curl -X POST -H "Content-Type: application/json" \
    -d '{"badge_id":"DEMO_ALERT","site_id":"Site-A","gate_id":"Gate-1","direction":"IN"}' \
    http://localhost:8080/v1/swipe
done
sleep 2
curl http://localhost:8081/v1/alerts | jq    # 應該看到 APB_BURST
```

### Q6：Excel 下載空白

A：先確認後端真有資料（`curl http://localhost:8081/v1/reports/attendance`）。如果有，但匯出空，看 `Content-Length` 是否非 0；多半問題在 `<a download>` 寫法。範本見 §3.4。

### Q7：B100 / B001 ~ B100 這些 badge 是哪裡來的？

A：dev_seed (`scripts/migrations/0099_dev_seed.up.sql`) 與 org-sync (`backend/cmd/org-sync/main.go`) 內建這些員工。完整列表：

| Badge | 姓名 | org_path | 角色 |
|---|---|---|---|
| B100 | 黃廠長 | TSMC.Fab12 | 廠長（看整 Fab12）|
| B001 | 王小明 | TSMC.Fab12.製造部 | 部主管 |
| B002 | 李大華 | TSMC.Fab12.品保部 | 部主管 |
| B003 | 張美玲 | TSMC.Fab15.研發部 | 部主管 |
| B004 | 陳志偉 | TSMC.Fab15.設備部 | 部主管 |
| B005 | 林雅婷 | TSMC.總部.人資部 | 部主管 |
| B011 | 林員工 | TSMC.Fab12.製造部 | 員工（非主管）|
| B012 | 趙員工 | TSMC.Fab12.製造部 | 員工 |
| B013 | 鄭新進 | TSMC.Fab12.製造部 | 員工（由 org-sync 新增）|

### Q8：org_path 顯示亂碼？

A：Phase 2 後 DB 是 UTF-8 + ltree 接受中文 label。可能是：
- nginx config 沒設 `charset utf-8`（一般 default 是）
- HTML 沒設 `<meta charset="UTF-8">` — 既有 index.html 第 4 行已設

### Q9：「設定」分頁裡的「Reporting API 網址」我應該寫什麼？

A：
- 走 nginx：留空或寫 `/`（fetch 用相對路徑）
- 直連 backend：`http://localhost:8081`

`getReportUrl()` 內部 fallback 是 `window.location.origin`，所以留空也 work。

---

## 9. 測試與驗收 checklist

### 9.1 整合進度

| 項目 | 狀態 |
|---|:---:|
| 既有刷卡頁 + 出席報表頁仍正常 | ☐ |
| 補 nginx proxy（`/v1/alerts`、`/v1/dev/login`）| ☐ |
| 加「主管視野」tab + caller 切換 | ☐ |
| 加「趨勢圖」tab + Chart.js | ☐ |
| 加「警報」tab + 自動 refresh | ☐ |
| 出席報表頁加 Excel 下載 | ☐ |
| 主管視野 403 處理（非主管顯示錯誤）| ☐ |
| 嚴重度顏色分級 | ☐ |
| caller 切換用 localStorage 持久化 | ☐ |

### 9.2 功能驗證命令（給你自測）

```bash
# 主管視野：B100 應該看到 Fab12 全部
curl 'http://localhost:8081/v1/reports/manager-team?as=B100' | jq .manager_scope
# → "TSMC.Fab12"

# 主管視野：B011 應該被擋
curl -w 'http=%{http_code}\n' 'http://localhost:8081/v1/reports/manager-team?as=B011'
# → http=403

# 趨勢：day
curl 'http://localhost:8081/v1/reports/trend?as=B100&period=day' | jq '.trends[0]'

# Excel 下載
curl -o /tmp/test.xlsx 'http://localhost:8081/v1/reports/attendance/export?format=excel'
file /tmp/test.xlsx
# → Microsoft OOXML

# 警報（先觸發再查）
for i in 1 2 3 4; do
  curl -X POST -H "Content-Type: application/json" \
    -d '{"badge_id":"DEMO","site_id":"Site-A","gate_id":"Gate-1","direction":"IN"}' \
    http://localhost:8080/v1/swipe
done
sleep 2
curl http://localhost:8081/v1/alerts | jq 'length'
```

### 9.3 跨瀏覽器測試建議

- Chrome / Safari / Firefox 都跑一次刷卡 + 報表（既有功能）
- Excel 下載：macOS Numbers / Office / LibreOffice 都能開
- 中文字元（員工姓名、org_path）顯示正常

---

## 10. 不明白就問

- **API 行為不確定** → [`PHASE2_VERIFICATION.md`](PHASE2_VERIFICATION.md) §4-9 有每個 endpoint 的完整實測輸出
- **設計理由不確定** → [`PHASE2_CHANGES.md`](PHASE2_CHANGES.md) §4.5 (reporting-api endpoints) / §6 (設計決策)
- **後端 bug** → 開 issue tagged `backend`
- **規格不確定** → 看 `final/HW2_Architecture_Design_Group15.pdf` §2 FR / §5.3 Phase 2

祝整合順利。
