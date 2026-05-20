# Docker 與 Kubernetes 部署、播種、壓測

本文說明如何將 PACS 服務打包成 Docker images，部署到 GKE，並用 Kubernetes Job 執行 PostgreSQL 大規模資料播種與歷史刷卡資料壓測匯入。

## 本機 Docker Compose

```bash
docker compose down -v
docker compose up -d --build
docker compose ps
```

服務預設位置：

- Frontend: <http://localhost>
- Access API: <http://localhost:8080>
- Reporting API: <http://localhost:8081>

## GKE 部署

```bash
./deploy-to-gke.sh <PROJECT_ID> [REGION] [CLUSTER_NAME]
```

部署腳本會：

- 先檢查目標 project 是否已啟用 Billing
- 建立或重用 GKE、Cloud SQL PostgreSQL 16、Memorystore Redis
- 建立並推送 backend、frontend、load-generator Docker images
- 建立 Kubernetes Secret/ConfigMap
- 執行 schema migration Job
- 部署 API、processors、frontend、ingress、HPA、PDB

部署後也可以自動執行 seed / load-test：

```bash
AUTO_CLOUD_SEED=1 AUTO_LOAD_TEST=1 ./deploy-to-gke.sh <PROJECT_ID> [REGION] [CLUSTER_NAME]
```

可調整等待時間：

```bash
CLOUD_SEED_TIMEOUT=30m LOAD_TEST_TIMEOUT=90m \
AUTO_CLOUD_SEED=1 AUTO_LOAD_TEST=1 \
./deploy-to-gke.sh <PROJECT_ID>
```

## PostgreSQL 大規模播種

部署完成後，手動執行 90,000 人 cloud seed：

```bash
kubectl delete job pacs-cloud-seed -n pacs --ignore-not-found
kubectl apply -f k8s/12-cloud-seed.yaml
kubectl logs -f job/pacs-cloud-seed -n pacs
```

為什麼會是 90,000 人：

- [scripts/cloud_migrations/0104_cloud_seed.up.sql](../scripts/cloud_migrations/0104_cloud_seed.up.sql) 先停用舊 demo/local seed 員工，避免 active total 混入 `B001`、`B100` 等測試帳號。
- Phase 1 固定建立 `B-000001` 這 1 位 `MANAGER_L1`。
- Phase 2 用 `generate_series(2, 151)` 建立 `B-000002` 到 `B-000151`，共 150 位 `MANAGER_L2`。
- Phase 3 用 `generate_series(152, 90000)` 建立 `B-000152` 到 `B-090000`，共 89,849 位 `STAFF`。
- 最後 SQL 會檢查 active employees 總數必須剛好等於 90,000；不符就 `RAISE EXCEPTION` 讓 Job 失敗。

確認結果：

```bash
kubectl get job pacs-cloud-seed -n pacs
kubectl logs job/pacs-cloud-seed -n pacs
kubectl run psql-check --rm -it -n pacs --image=postgres:16-alpine --restart=Never -- \
  psql -h <db-host-or-proxy> -U pacs_user -d pacs_db \
  -c "SELECT job_level, COUNT(*) FROM employees WHERE is_active GROUP BY job_level ORDER BY job_level;"
```

## PostgreSQL 歷史刷卡壓測匯入

`pacs-load-test` Job 會在 Pod 內產生 `seed_history_events.sql`，再用 `psql` 匯入 Cloud SQL。預設為 cloud 模式、7 天、50 workers。

```bash
PROJECT_ID=<PROJECT_ID>
kubectl delete job pacs-load-test -n pacs --ignore-not-found
sed "s|gcr.io/PROJECT_ID|gcr.io/$PROJECT_ID|g" k8s/07-load-tester.yaml | kubectl apply -f -
kubectl logs -f job/pacs-load-test -n pacs
```

調整規模可直接編輯 [k8s/07-load-tester.yaml](../k8s/07-load-tester.yaml) 的 `LOAD_TEST_DAYS`、`LOAD_TEST_WORKERS`、`LOAD_TEST_QPS_SCALE`。

## Review 分支

不要直接 push 到 `main`。建議使用 review branch：

```bash
git switch -c <review-branch>
git push -u <remote> <review-branch>
```

等其他人 review 並同意後再合併。
