# PACS 開發待辦事項 (TODO)



## 4. K8s GKE

- [✅] **部署所有專案上 GKE**
- [✅] **跑 `k8s/07-k6-load-test.yaml` 驗 HPA** — `kubectl apply` 後 `kubectl get hpa access-api -w`，預期 60 秒內 replicas 擴展（NFR-4）。

- [✅] **執行雲端 90k seed (`0104_cloud_seed`)** — `scripts/cloud_migrations/0104_cloud_seed.up.sql` 是 Phase 3 規模播種，**手動執行**：
  ```bash
  gcloud sql connect <INSTANCE> --user=pacs_user --database=pacs_db \
    < scripts/cloud_migrations/0104_cloud_seed.up.sql
  ```
  執行完跑 k6 `shift_burst.js`（BADGE_COUNT=90000）驗 NFR-1。



- [✅] 傳輸安全優化：已加入 GKE HTTPS 部署模式。設定 `DOMAIN_NAME=pacs.example.com make gke-deploy` 後，部署腳本會建立 global static IP、套用 `ManagedCertificate`、啟用 `FrontendConfig` HTTP-to-HTTPS redirect。仍需在 DNS provider 將網域 A record 指向 static IP，並等待憑證 `CertificateStatus: Active`。

- [✅] **內嵌 IP/端點（動態 URL 優化）**：移除程式碼或設定檔中硬編碼的固定 IP，全面改由環境變數（Environment Variables）、K8s ConfigMap 或透過 K8s Service / Ingress 內部域名進行動態解析，確保上雲後部署彈性。
  - 移除 `k8s/08-ingress.yaml` 中的 `static-ip-name` annotation，改由 GKE Load Balancer 自動分配動態 IP
  - 移除 `k8s/01-config.yaml` 中硬編碼的密碼，Secret 改由 `deploy-to-gke.sh` 動態生成注入
  - `deploy-to-gke.sh` 部署時動態抓取 Memorystore IP 與 Cloud SQL 連線名，透過 `kubectl create configmap` 注入，不依賴靜態設定檔
  - 驗證：`gcloud compute addresses list --project=extreme-water-497313-j8` 回傳空，確認無靜態 IP 保留，目前雲端 IP `8.233.246.41` 為動態分配

- [✅] **k6 上雲壓測&Prometheus呈現**：目前 k6 用 console summary（已能 pass/fail thresholds）。進階：讓 k6 metrics 也 push 到 `monitoring/prometheus`，在 Grafana 看 P99 趨勢線。

---
