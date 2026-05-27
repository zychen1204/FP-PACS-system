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
