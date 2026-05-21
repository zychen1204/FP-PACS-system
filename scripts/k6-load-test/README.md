# PACS k6 Load Test

對齊 HW2 與系統 spec 的即時 HTTP 壓測腳本。**不是 seed**——歷史資料請改用
[`../seed-generator/`](../seed-generator/)。

## 場景對應

| 腳本 | 對應 spec / HW2 | 主要驗證 threshold |
|---|---|---|
| `shift_burst.js` | HW2 §4.2 換班尖峰、spec「Shift Change spike」 | `http_req_duration{endpoint:swipe} p(99)<50` (NFR-1) |
| `steady_baseline.js` | HW2 §4.2 Phase 2 平均 ~3.5 QPS 基準 | 同上，作為對照組 |
| `mixed_read_write.js` | spec 核心矛盾「write-heavy + read-heavy 解耦」 | NFR-1 + `http_req_duration{endpoint:report} p(95)<200` (NFR-2) |

## 快速開始（本地 docker compose）

```bash
# 1. 啟服務
docker compose up -d --build

# 2. 灌一點員工資料（給 badge pool 用）
cd scripts/seed-generator && go run . --mode local --days 7
docker compose exec -T postgres psql -U pacs_user -d pacs_db < seed_history_events.sql
cd -

# 3. 跑 k6
cd scripts/k6-load-test
BADGE_COUNT=1000 ACCESS_API=http://localhost:8080 \
  docker run --rm --network=host -v "$PWD":/scripts -w /scripts \
  grafana/k6:0.50.0 run shift_burst.js
```

預期輸出末段：

```
     ✓ http_req_duration{endpoint:swipe}.p(99)  < 50ms
     ✓ http_req_failed{endpoint:swipe}........: rate < 5%
```

## 環境變數

| Env | 預設 | 用途 |
|---|---|---|
| `ACCESS_API` | `http://localhost:8080` | access-api endpoint |
| `REPORT_API` | `http://localhost:8081` | reporting-api endpoint（只 mixed 用） |
| `BADGE_COUNT` | `1000` | 隨機 badge pool 大小，**必須 ≤ 已灌員工數** |

## Grafana 整合（觀察 shift spike）

`monitoring/prometheus.yml` 已 scrape access-api 的 `/metrics`，跑 k6 同時打開
http://localhost:3000 (admin/admin) → "PACS Overview" dashboard，
觀察 QPS 圖在 ramp-up/plateau 期間的尖峰形狀。

進階：用 `k6 run --out experimental-prometheus-rw=...` 讓 k6 自己 push metrics
到 Prometheus（需配置 remote-write receiver）。MVP 階段先用內建 console summary。

## K8s 上跑

```bash
kubectl apply -f ../../k8s/07-k6-load-test.yaml
kubectl logs -f -n pacs job/k6-shift-burst
# 觀察 HPA：kubectl get hpa -n pacs access-api -w
```

## NFR 對應 (HW2 §3)

| NFR | 規範 | k6 驗證 |
|---|---|---|
| NFR-1 | 寫入 P99 < 50ms | `shift_burst.js` threshold |
| NFR-2 | 報表 P95 < 200ms | `mixed_read_write.js` report threshold |
| NFR-4 | HPA 60s 內擴展 | 跑 `shift_burst.js`，觀察 `kubectl get hpa -w` |
| NFR-5 | DB 故障時事件不丟 | 跑 `steady_baseline.js`，期間 `docker compose stop postgres`，觀察 swipe 仍 200 |
