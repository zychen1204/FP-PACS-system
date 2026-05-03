# PACS — 分散式實體門禁控制系統

Cloud-Native Physical Access Control System

## 架構

```
Badge Readers / Frontend
        │
        ▼
  ┌─────────────┐     ┌───────┐     ┌──────────────┐
  │ access-api  │────▶│ Redis │────▶│    Redis      │
  │  (Port 8080)│     │ Cache │     │   Streams     │
  └─────────────┘     │  APB  │     │  (pacs:events)│
                      └───────┘     └──────┬───────┘
                                           │
                                           ▼
                                   ┌───────────────┐
                                   │event-processor│
                                   └───────┬───────┘
                                           │
                                           ▼
                                   ┌───────────────┐
                                   │  PostgreSQL   │
                                   │ (append-only) │
                                   └───────┬───────┘
                                           │
                                           ▼
                                   ┌───────────────┐
                                   │ reporting-api │
                                   │  (Port 8081)  │
                                   └───────────────┘
```

## 快速啟動

```bash
docker-compose up --build
```

- **前端介面**: http://localhost
- **Access API**: http://localhost:8080
- **Reporting API**: http://localhost:8081

## 技術棧

| 元件 | 技術 |
|------|------|
| 前端 | HTML5 + CSS3 + JavaScript + Nginx |
| 後端 | Go 1.21 + Gin Framework |
| 資料庫 | PostgreSQL 15 |
| 快取/MQ | Redis 7 (Cache + Streams) |
| 容器化 | Docker + Docker Compose |

## 詳細測試流程

請參閱 [TESTING.md](TESTING.md)
