# `docs/` — PACS 規範與設計文件

本資料夾蒐集 PACS 系統的「規範書 + 設計文件 + 實作對照 + 驗收 + 整合指引」五件套。
資料庫模組（Phase 1）與 Phase 2 後端升級皆已交付。

## 文件總覽

### 資料庫模組（spec / erd / compliance 三件組）

| 文件 | 用途 | 目標讀者 |
|---|---|---|
| [`database-spec.md`](database-spec.md) | 從 spec PDF 蒸餾出**只與 DB 有關**的 FR / NFR / 容量估算 / 已選型架構 | DB owner、reviewer、Phase 升級規劃者 |
| [`database-erd.md`](database-erd.md) | 資料表 ERD（Mermaid）、欄位字典、約束、索引、觸發器、角色權限 | 任何要讀懂 schema 的人 |
| [`database-compliance.md`](database-compliance.md) | 每條 FR / NFR ↔ 實作位置 ↔ **實測輸出** ↔ 階段對照 | 助教 / reviewer / 自我稽核 |

### Phase 2 後端落地文件

| 文件 | 用途 | 目標讀者 |
|---|---|---|
| [`PHASE2_CHANGES.md`](PHASE2_CHANGES.md) | Phase 2 後端設計改動記錄（10 section，4 commit / 21 檔案 / +2216 行的「why & how」）| 看 PR、review code、要理解設計取捨的人 |
| [`PHASE2_VERIFICATION.md`](PHASE2_VERIFICATION.md) | 完整 19-section 驗收劇本：每節含對應 FR/NFR、可重現命令、實測輸出、結論 | TA、組長、自我回顧 |
| [`FRONTEND_INTEGRATION.md`](FRONTEND_INTEGRATION.md) | 前端組員整合指引：5 個新 endpoint API 字典、認證流程、UI 規劃、JS snippet、FAQ | 前端組員 |
| [`FRONTEND.md`](FRONTEND.md) | 前端 v2.0 改良總結：UI/UX 升級、兩層門禁系統、報表系統、API 集成、完整測試覆蓋 | 所有人 |
| [`REPORT_JSON_SCHEMAS.md`](REPORT_JSON_SCHEMAS.md) | API 數據格式與響應示例 | API 調用者 |

### 三組文件關係

```
規範要求          落地設計與決策          落地後實測證明
database-spec.md → PHASE2_CHANGES.md → PHASE2_VERIFICATION.md
       │                  │                       │
       └──── database-erd.md (schema 字典) ────────┤
                          │                       │
                          └── database-compliance.md (FR/NFR ↔ 實作 ↔ 證據)

                       前端組員另看 → FRONTEND_INTEGRATION.md
                       前端改良總結 → FRONTEND.md
```

## 與其他文件的關係

| 既有文件 | 角色 |
|---|---|
| `../README.md` | 專案首頁、架構圖、技術棧、Schema 摘要 |
| `../TESTING.md` | 端到端測試步驟（手動 demo 用） |
| `../scripts/README.md` | Migration 命名規範、新增流程、Phase 2 partition 維護 |
| `../frontend/` | 前端完整代碼 |

`docs/` 與上面三者**無重複內容**：

- 規範與設計 → `docs/`（database-spec/erd/compliance）
- 前端設計與整合 → `docs/FRONTEND*.md`（FRONTEND.md、FRONTEND_INTEGRATION.md）
- 操作步驟 → `TESTING.md`
- Migration 工程規範 → `scripts/README.md`
- Onboarding 摘要 → 根 `README.md`

## 命名約定

- 主題式：`{module}-{type}.md`，type ∈ `{spec, erd, compliance, design, ...}`
- 階段式：`PHASE{N}_{purpose}.md`（如 `PHASE2_CHANGES.md`、`PHASE2_VERIFICATION.md`）
- 角色式：`{audience}_{purpose}.md`（如 `FRONTEND_INTEGRATION.md`）

未來新增其他模組文件時請沿用此前綴。
