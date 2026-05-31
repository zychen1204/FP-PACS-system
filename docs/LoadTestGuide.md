# 即時壓力測試指南 (k6 Load Test)

> 對齊 HW2 §4 / spec「Shift Change spike」與 NFR-1 / NFR-2 / NFR-4。
> 「灌歷史 demo data」請看 [`SimulationGuide.md`](SimulationGuide.md)。

## 為什麼用 k6（而不是繼續用 Go seed-generator）

| 議題 | seed-generator (Go) | k6 |
|---|---|---|
| 主要用途 | 產 SQL 種子 | 即時 HTTP 打 access-api |
| 是否走 access-api hot path | ❌ 直接 SQL 灌 DB | ✅ 真實 HTTP POST |
| 統計輸出 | 無 P50/P95/P99 | 內建 |
| Threshold 自動 pass/fail | 無 | 有（CI 友善） |
| Burst 模式 | Gaussian smooth | `ramping-arrival-rate` 原生支援 |
| 整合 Prometheus | 無 | `--out experimental-prometheus-rw` |


## 場景對應 spec / HW2

| 腳本 | 對應規範 | 主要驗證 |
|---|---|---|
| `shift_burst.js` | HW2 §4.2 換班 8:00 ~20K events / 15 min；spec「Shift Change spike」 | NFR-1 `p(99)<50ms` |
| `steady_baseline.js` | HW2 §4.2 Phase 2 平均 ~3.5 QPS 基準 | NFR-1（對照組） |
| `mixed_read_write.js` | spec 核心矛盾「write-heavy + read-heavy 解耦」 | NFR-1 + NFR-2 `p(95)<200ms` |


## 快速開始（本地 docker compose）

```bash
# 1. 一鍵重置 + 灌過去 7 天歷史（seed 不含今天，今天留給 k6 即時打）
./scripts/demo-reset.sh 7

# 2. 跑 k6 shift burst（主驗收場景）
cd scripts/k6-load-test
docker run --rm --network=host \
  -v "$PWD":/scripts -w /scripts \
  -e ACCESS_API=http://localhost:8080 \
  -e BADGE_COUNT=1000 \
  grafana/k6:0.50.0 run shift_burst.js
```

預期輸出末段：
```
     ✓ http_req_duration{endpoint:swipe}.p(99)  ........: p(99)=...ms     < 50ms
     ✓ http_req_failed{endpoint:swipe}...........: rate=...%        < 5%
```

## 觀察 Grafana 尖峰

跑 k6 同時開 http://localhost:3000 (admin/admin)，「PACS Overview」dashboard 應看到：
- access-api QPS 圖在 30s ramp-up → 2 min plateau → 30s ramp-down 的 burst shape
- 對應 spec「Shift Change spike」可視化要求

## 場景參數

`shift_burst.js`（demo 用 2 min plateau；production 場景可改 15 min 對齊 HW2）：
```text
Stage 1  0 –  30s : ramp 1 →   5 QPS  (常態 baseline)
Stage 2 30 –  60s : ramp 5 → 100 QPS  (ramp-up)
Stage 3 60 – 180s : 維持   100 QPS    (換班尖峰 plateau)
Stage 4 180 – 210s: ramp 100 →  5 QPS (ramp-down)
```

`mixed_read_write.js`：兩個並行 scenario
- `swipe_burst` (write path，ramping 5 → 50 → 5 QPS over 3 min)
- `report_queries` (read path，constant 5 QPS for 3.5 min)


## NFR 對應驗證

| NFR | 規範 | 怎麼跑 / 看哪裡 |
|---|---|---|
| NFR-1 | 寫入 P99 < 50ms | `shift_burst.js` console 末段；threshold 自動 pass/fail |
| NFR-2 | 報表 P95 < 200ms | `mixed_read_write.js` `{endpoint:report}` |
| NFR-4 | HPA 60s 內擴展 | K8s 環境跑 `k8s/07-k6-load-test.yaml`，`kubectl get hpa access-api -w` |
| NFR-5 | DB 故障時事件不丟 | 跑 `steady_baseline.js`，期間 `docker compose stop postgres`，觀察 swipe 仍 200 |


## 環境變數

| Env | 預設 | 用途 |
|---|---|---|
| `ACCESS_API` | `http://localhost:8080` | access-api endpoint |
| `REPORT_API` | `http://localhost:8081` | reporting-api endpoint（`mixed_read_write.js` 用） |
| `BADGE_COUNT` | `1000` | 隨機 badge pool 大小，**必須 ≤ 已灌員工數** |


## K8s 部署

```bash
# 1. 建立 ConfigMap（k6 腳本）+ 部署 Job
kubectl create configmap pacs-k6-scripts -n pacs \
  --from-file=scripts/k6-load-test/ \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f k8s/07-k6-load-test.yaml

# 2. 看結果
kubectl logs -f -n pacs job/k6-shift-burst

# 3. HPA 監控（另開 terminal）
kubectl get hpa -n pacs access-api -w
```


## 故障排除

- **P99 超標**：先看 Grafana access-api `/metrics`，是 `pacs_swipe_total` 飆高還是 Redis 延遲？
- **APB rate 過高（>10%）**：`BADGE_COUNT` 太小導致 badge 衝突，調高即可
- **k6 connection refused**：確認 `docker compose ps` 所有 service healthy
- **threshold 紅綠燈不顯示**：k6 在所有 iterations 結束後才印；確認沒被 `--quiet` 或 timeout 切掉

---

## 上雲壓力測試（GKE + Grafana 視覺化）

> 測試目標：對 GKE 雲端 access-api 執行 shift-burst 場景，驗證 p99 < 50ms（NFR-1）。
> k6 從本地執行，metrics 推送至本地 Prometheus，在本地 Grafana 視覺化呈現。

### 前置需求

| 項目 | 確認方式 |
|------|---------|
| GKE 叢集運行中 | `kubectl get nodes` |
| Docker Desktop 啟動中 | `docker ps` |
| k6 已安裝（WSL） | `k6 version` |

#### 安裝 k6（WSL Ubuntu，第一次才需要）

```bash
sudo gpg --no-default-keyring \
  --keyring /usr/share/keyrings/k6-archive-keyring.gpg \
  --keyserver hkp://keyserver.ubuntu.com:80 \
  --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69

echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
  | sudo tee /etc/apt/sources.list.d/k6.list

sudo apt-get update && sudo apt-get install k6
```

### 步驟 1：取得雲端 GKE Ingress IP

```bash
# 先連接 GKE（若 kubectl 出現 connection refused 時執行）
gcloud container clusters get-credentials pacs-cluster \
  --region=asia-east1 \
  --project=extreme-water-497313-j8

# 取得目前 Ingress IP（每次部署後 IP 可能不同，不要硬編碼）
kubectl get ingress pacs-ingress -n pacs
```

記下 `ADDRESS` 欄位的 IP，例如 `34.107.166.43`。

### 步驟 2：啟動本地 Prometheus + Grafana

```bash
docker-compose up -d prometheus grafana
```

### 步驟 3：執行 k6 壓測

> 將 `<GKE_IP>` 替換為步驟 1 取得的 IP。

```bash
k6 run \
  --out experimental-prometheus-rw \
  -e K6_PROMETHEUS_RW_SERVER_URL=http://localhost:9090/api/v1/write \
  -e ACCESS_API=http://<GKE_IP> \
  scripts/k6-load-test/shift_burst.js
```

測試場景（總時長 3.5 分鐘）：

| 時間段 | 持續時間 | QPS | 說明 |
|--------|---------|-----|------|
| 0:00 ~ 0:30 | 30s | 1 → 5 | 常態 baseline |
| 0:30 ~ 1:00 | 30s | 5 → 100 | Ramp-up 換班尖峰 |
| 1:00 ~ 3:00 | 2min | 100 | 維持換班尖峰（burst plateau） |
| 3:00 ~ 3:30 | 30s | 100 → 5 | Ramp-down |

### 步驟 4：Grafana 觀察重點

開啟 `http://localhost:3000`，進入 **k6 Prometheus** dashboard，調整時間範圍為 **Last 15 minutes**：

| Panel | 觀察重點 |
|-------|---------|
| **Performance Overview**（折線圖） | VUs 與 QPS ramp-up / plateau / ramp-down 曲線 |
| **HTTP Request Rate** | QPS 從 0 爬升到 ~100 再降回 |
| **HTTP Latency Stats** | p99 全程應維持在 50ms 以下（NFR-1） |
| **Requests by URL** | 顯示雲端 IP `/v1/swipe`，p99 數值 |
| **Checks: status 200 or 403_APB** | Success Rate 應為 100% |

### 通過標準（NFR-1）

| 指標 | 門檻 | 預期結果 |
|------|------|---------|
| p99 延遲 | < 50ms | ✅ 通過 |
| Error rate（5xx） | < 5% | ✅ 通過 |
| Checks success rate | 100% | ✅ 通過 |

---

## HPA 自動擴縮驗證（NFR-4）

> 測試目標：在 k6 壓測期間觀察 access-api HPA 自動增加 Pod 數量，驗證水平自動擴縮功能。
> HPA 設定：CPU 65% 觸發，minReplicas=3，maxReplicas=20，scaleUp 穩定窗口 60 秒。

### 步驟 1：暫時調低 HPA 觸發門檻（demo 用）

正常 100 QPS 下 CPU 未必超過 65%，調低至 10% 讓擴縮容易被觀察到：

```bash
kubectl patch hpa access-api-hpa -n pacs \
  --type=merge \
  -p '{"spec":{"metrics":[{"type":"Resource","resource":{"name":"cpu","target":{"type":"Utilization","averageUtilization":10}}}]}}'

kubectl get hpa access-api-hpa -n pacs
```

### 步驟 2：開啟 HPA 監控視窗

另開一個 terminal，每 3 秒刷新顯示 HPA 與 Pod 狀態：

```bash
watch -n 3 kubectl get hpa,pods -n pacs
```

### 步驟 3：執行 k6 壓測觸發擴縮

```bash
k6 run \
  --out experimental-prometheus-rw \
  -e K6_PROMETHEUS_RW_SERVER_URL=http://localhost:9090/api/v1/write \
  -e ACCESS_API=http://<GKE_IP> \
  scripts/k6-load-test/shift_burst.js
```

### 步驟 4：觀察擴縮行為

k6 進入 burst 階段（約 1 分鐘後），應觀察到 REPLICAS 從 3 增加：

```
NAME             REFERENCE               TARGETS       MINPODS   MAXPODS   REPLICAS
access-api-hpa   Deployment/access-api   cpu: 0%/10%   3         20        3    ← 初始
access-api-hpa   Deployment/access-api   cpu: 13%/10%  3         20        3    ← CPU 超過門檻
access-api-hpa   Deployment/access-api   cpu: 14%/10%  3         20        4    ← 擴縮觸發！
access-api-hpa   Deployment/access-api   cpu: 0%/10%   3         20        4    ← 負載結束
access-api-hpa   Deployment/access-api   cpu: 0%/10%   3         20        3    ← 自動縮回
```

> HPA 有 60 秒 stabilizationWindowSeconds，觸發後約 1 分鐘才開始擴縮。REPLICAS 增加即代表驗證成功。

### 步驟 5：還原 HPA 門檻（測試完必做）

```bash
kubectl patch hpa access-api-hpa -n pacs \
  --type=merge \
  -p '{"spec":{"metrics":[{"type":"Resource","resource":{"name":"cpu","target":{"type":"Utilization","averageUtilization":65}}}]}}'

kubectl get hpa access-api-hpa -n pacs
# TARGETS 欄位應回到 xx%/65%
```

### Demo 畫面配置建議

| 視窗 | 顯示內容 |
|------|---------|
| 瀏覽器 | Grafana — HTTP Request Rate + HTTP Latency Stats |
| Terminal 1 | `watch -n 3 kubectl get hpa,pods -n pacs`（HPA 擴縮） |
| Terminal 2 | k6 執行輸出（即時 threshold 狀態） |
