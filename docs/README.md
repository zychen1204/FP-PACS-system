# `docs/` — PACS 規範與設計文件

本資料夾蒐集 PACS 系統各模組的「規範書 + 設計文件 + 實作對照」三件組。
目前已交付資料庫模組；其他模組（access-api、event-processor、reporting-api、frontend）
依需要再補。

## 資料庫模組

| 文件 | 用途 | 目標讀者 |
|---|---|---|
| [database-spec.md](database-spec.md) | 從 spec PDF 蒸餾出**只與 DB 有關**的 FR / NFR / 容量估算 / 已選型架構 | DB owner、reviewer、Phase 2 規劃者 |
| [database-erd.md](database-erd.md) | 資料表 ERD（Mermaid）、欄位字典、約束、索引、觸發器、角色權限 | 任何要讀懂 schema 的人 |
| [database-compliance.md](database-compliance.md) | 每條 FR / NFR ↔ 實作位置 ↔ **實測輸出** ↔ 階段對照 | 助教 / reviewer / 自我稽核 |

三者關係：

```
database-spec.md      ──┐
   (該做什麼)            ├──► database-compliance.md
database-erd.md       ──┘   (做了沒、實測證明)
   (怎麼做的)
```

## 與其他文件的關係

| 既有文件 | 角色 |
|---|---|
| `../README.md` | 專案首頁、快速啟動、技術棧 |
| `../TESTING.md` | 端到端測試步驟（手動 demo 用） |
| `../scripts/README.md` | Migration 命名規範、新增流程、Phase 2 partitioning playbook |

`docs/` 與上面三者**無重複內容**：

- 規範與設計 → `docs/`
- 操作步驟 → `TESTING.md`
- Migration 工程規範 → `scripts/README.md`
- Onboarding 摘要 → 根 `README.md`

## 命名約定

`{module}-{type}.md`，type ∈ `{spec, erd, compliance, design, ...}`。
未來新增其他模組文件時請沿用此前綴。
