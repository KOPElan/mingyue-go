# 架构决策记录（ADR）索引

本目录记录 mingyue-go 项目的架构决策记录（Architecture Decision Records）。  
每个 ADR 描述一项重要的架构或技术决策，包含背景、决策内容、备选方案与后果分析。

## ADR 列表

| 编号 | 标题 | 状态 | 日期 |
|------|------|------|------|
| [ADR-001](./0001-cli-daemon-dual-mode.md) | 采用 CLI + 守护进程双运行形态 | 已接受 | 2026-02-27 |
| [ADR-002](./0002-shared-business-logic.md) | CLI 与 API 共享同源业务逻辑层 | 已接受 | 2026-02-27 |
| [ADR-003](./0003-restful-api-openapi.md) | 采用 RESTful HTTP+JSON API 并提供 OpenAPI 规范 | 已接受 | 2026-02-27 |
| [ADR-004](./0004-unified-error-structure.md) | 统一错误结构与错误码 | 已接受 | 2026-02-27 |
| [ADR-005](./0005-minimal-privilege-linux-capabilities.md) | 最小权限原则与 Linux Capabilities | 已接受 | 2026-02-27 |
| [ADR-006](./0006-audit-logging.md) | 审计日志设计 | 已接受 | 2026-02-27 |
| [ADR-007](./0007-system-info-collection.md) | 系统信息采集方案选型（/proc + gopsutil） | 已接受 | 2026-02-27 |
| [ADR-008](./0008-path-security.md) | 路径安全与目录穿越防护 | 已接受 | 2026-02-27 |

## ADR 状态说明

| 状态 | 含义 |
|------|------|
| **草案（Draft）** | 决策正在讨论中，尚未最终确定 |
| **已接受（Accepted）** | 决策已确定并落地执行 |
| **已废弃（Deprecated）** | 决策曾被接受，但已被新决策取代 |
| **已取代（Superseded）** | 被指定的新 ADR 取代，附带取代说明 |

## 决策关系图

```
ADR-001（双运行形态）
    ├── ADR-002（同源业务逻辑）──► ADR-004（统一错误结构）
    ├── ADR-003（RESTful API）  ──► ADR-004（统一错误结构）
    ├── ADR-005（最小权限）     ──► ADR-006（审计日志）
    │                                   ▲
    ├── ADR-007（系统信息采集）          │
    └── ADR-008（路径安全）    ──────────┘
```

## 如何新增 ADR

1. 在本目录新建文件，命名格式：`NNNN-short-title.md`（编号递增，连字符分隔的小写英文）。
2. 使用以下标准模板：

```markdown
# ADR-XXX: [标题]

**状态**: 草案（Draft）
**日期**: YYYY-MM-DD
**决策者**: 开发团队

## 背景

[描述问题背景与驱动因素]

## 决策

[具体决策内容]

## 备选方案

[列出其他备选方案及被拒绝原因]

## 后果

### 正面影响
- ...

### 负面影响 / 权衡
- ...

### 关联
- 相关 ADR 或需求引用
```

3. 在本 README 的"ADR 列表"表格中添加对应条目。
4. 如果新 ADR 取代了已有 ADR，将被取代 ADR 的状态更新为"已取代（Superseded by ADR-XXX）"。
