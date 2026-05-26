# 🏢 PACS — 分散式實體門禁控制系統
> **Cloud-Native Physical Access Control System**

[![Status](https://img.shields.io/badge/Status-Phase_2_Completed-success.svg)]()
[![Backend](https://img.shields.io/badge/Backend-Go_1.21-blue.svg)]()
[![Database](https://img.shields.io/badge/Database-PostgreSQL_16-blue.svg)]()

本專案是一個現代化、具備多層級廠區與權限管理的實體門禁控制系統。它利用微服務架構，結合 Redis 進行防跟隨 (Anti-Passback) 高速驗證，並使用 PostgreSQL 進行安全的不可變稽核日誌 (Immutable Audit Logs) 儲存。

## 🚀 快速啟動

第一次啟動或前端有更新時，請務必加上 `--build` 以重建容器：

```bash
docker compose down -v   # 1. 下掉所有服務與容器（含volumes）
docker compose up -d --build  # 2. 重新啟動所有服務與容器
sleep 25  # 等待 migrate 與各服務就緒
```

- **前端介面**: <http://localhost>
- **Access API**: <http://localhost:8080>
- **Reporting API**: <http://localhost:8081>

## ☁️ GKE HTTPS Demo

Current cloud endpoint:

```text
https://34-107-166-43.sslip.io
```

To configure or repair the demo HTTPS endpoint on GKE:

```bash
make gke-https-demo
```

This reserves or reuses the global static IP `pacs-ingress-ip`, derives an
`sslip.io` hostname from that IP, applies the GKE managed certificate and
Ingress redirect resources, waits for the certificate to become `Active`, and
runs an HTTPS smoke test.

Useful checks:

```bash
make gke-https-generate-demo-domain
make gke-https-status DOMAIN_NAME=34-107-166-43.sslip.io
make gke-https-smoke DOMAIN_NAME=34-107-166-43.sslip.io
```

## 🏗 系統架構簡介

系統為讀寫分離的高效能設計：
- **[寫入] Access API**: 直接與 Redis Cache 互動進行低延遲（<50ms）的 APB 驗證，並將成功或拒絕的事件丟入 Redis Stream，由後端的 Event Processor 非同步寫入資料庫。
- **[讀取] Reporting API**: 提供 JWT 保護的報表查詢，連接 PostgreSQL。配合 Materialized View (`mv_daily_attendance`) 提供極速的主管視野統計。

👉 **[查看完整架構圖與設計細節](docs/ArchitectureDesign.md#架構phase-2)**

## 🛠 技術棧

| 領域 | 使用技術 |
|------|------|
| **前端** | HTML5, CSS3, JavaScript, Nginx |
| **後端** | Go 1.21, Gin Framework, golang-jwt/v5, Excelize |
| **資料庫** | PostgreSQL 16 (C.UTF-8, ltree, pg_stat_statements) |
| **快取 / MQ** | Redis 7 (Cache, Streams, DLQ) |
| **基建** | Docker, Docker Compose, Grafana/Prometheus |


### 🌟 開發重點
- **待辦清單**: 👉 [待辦事項](docs/TodoList.md)
- **資料模擬**: 👉 [模擬指南](docs/SimulationGuide.md)

## 🧪 測試與驗證

本系統提供豐富的單元與端到端 (E2E) 測試機制。
👉 **[測試詳細](docs/TestingGuide.md)**
