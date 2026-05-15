# `docs/` — PACS 規範與設計文件

本資料夾蒐集 PACS 系統的「規範書 + 設計文件 + 實作對照 + 驗收 + 整合指引」等歷史與現行文件。

## 當前重點文件

| 文件 | 用途 |
|---|---|
| [`TodoList.md`](TodoList.md) | 列出後續預計實作的重點功能與修復項目（包含前、後端與資料庫）。 |
| [`SimulationGuide.md`](SimulationGuide.md) | 關於員工與刷卡紀錄模擬指南。 |

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
