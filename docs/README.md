# 📚 PACS 文件索引 (Documentation Index)

本資料夾蒐集 PACS 系統的「規範書 + 設計文件 + 實作對照 + 驗收 + 整合指引」等歷史與現行文件。

## 當前重點文件

| 文件 | 用途 |
|---|---|
| [實作與修復待辦事項](TodoList.md) | 列出後續預計實作的重點功能與修復項目（包含前、後端與資料庫）。 |
| [資料模擬與測試指南](SimulationGuide.md) | 關於 1000 人大規模員工與刷卡紀錄模擬、壓力測試的詳細指引。 |
| [Docker 與 Kubernetes 部署、播種、壓測](DockerK8sDeployment.md) | Docker build、GKE 部署、Cloud SQL seed Job 與 load-test Job 操作。 |

## 歷史文件 (History)

先前的 Phase 2 實作與資料庫模組（Phase 1）相關的詳細設計與規範檔案，已歸檔至 `history/` 目錄中以保持根目錄整潔。

👉 **[點此查看歷史歸檔文件 (`history/`)](history/)**

包含：
- **資料庫相關：** `database-spec.md`, `database-erd.md`, `database-compliance.md`
- **前端相關：** `FRONTEND.md`, `FRONTEND_INTEGRATION.md`
- **後端與整合：** `BACKEND_INTEGRATION.md`, `PHASE2_CHANGES.md`, `PHASE2_VERIFICATION.md`, `REPORT_JSON_SCHEMAS.md`

## 命名約定

若未來新增其他模組文件，請參考：
- 主題式：`{module}-{type}.md`，type ∈ `{spec, erd, compliance, design, ...}`
- 階段式：`PHASE{N}_{purpose}.md`
- 角色式：`{audience}_{purpose}.md`
