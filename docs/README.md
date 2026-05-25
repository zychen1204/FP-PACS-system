# 📚 PACS 文件索引 (Documentation Index)

本資料夾蒐集 PACS 系統的「規範書 + 設計文件 + 實作對照 + 驗收 + 整合指引」。

## 現役文件（規範與驗證流程）

| 文件 | 用途 |
|---|---|
| [系統架構](ArchitectureDesign.md) | Phase 2 完整微服務 + DB 架構圖、角色矩陣、設計理念 |
| [實作與修復待辦事項](TodoList.md) | 後續預計實作的重點功能與修復項目（前 / 後端 / 資料庫 / K8s） |
| [完整執行與測試流程](TestingGuide.md) | 端到端驗收劇本（含 FR-12 immutability、最小權限、ltree 主管查詢） |
| [歷史資料模擬指南](SimulationGuide.md) | seed-generator 灌歷史 demo data（純 SQL 直灌；對應 HW2 三個 phase 規模） |
| [即時壓力測試指南](LoadTestGuide.md) | k6 即時 HTTP 壓測（shift burst、NFR-1/2/4 threshold 驗證） |
| [GKE 上雲部署報告](GKEDeploymentReport.md) | GKE/Cloud SQL/Memorystore 上雲架構、部署決策、驗證結果與關閉資源方式 |

> **seed vs k6 分工**：歷史資料用 seed-generator（一次性，SQL 直灌）；NFR threshold 驗證用 k6（即時 HTTP，自動 pass/fail）。詳見上述兩份指南開頭。

## 歷史 / 設計脈絡文件 (`history/`)

Phase 1 / Phase 2 落地時的詳細設計、實作對照、驗收歸檔。仍是 **single source of truth**（敘述版），不是棄用文件。

👉 **[`history/`](history/)**

包含：
- **資料庫：** `database-spec.md`、`database-erd.md`、`database-compliance.md`
- **前端：** `FRONTEND.md`、`FRONTEND_INTEGRATION.md`
- **後端與整合：** `BACKEND_INTEGRATION.md`、`PHASE2_CHANGES.md`、`PHASE2_VERIFICATION.md`、`REPORT_JSON_SCHEMAS.md`

## 命名約定

若未來新增其他模組文件，請參考：
- 主題式：`{module}-{type}.md`，type ∈ `{spec, erd, compliance, design, ...}`
- 階段式：`PHASE{N}_{purpose}.md`
- 角色式：`{audience}_{purpose}.md`
