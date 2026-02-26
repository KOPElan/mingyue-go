# Implementation Plan: Linux 系统操作代理（CLI + REST API + OpenAPI）

**Branch**: `001-linux-ops-agent` | **Date**: 2026-02-27 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `specs/001-linux-ops-agent/spec.md`

**Note**: 本文件由 `/speckit.plan` 流程填充，用于指导后续实现；此阶段仅产出“计划与契约”，不直接提交实现代码。

## Summary

- 目标：实现 Linux 主机操作代理，提供 CLI 与 RESTful HTTP+JSON API（含 OpenAPI/Swagger 交付物），覆盖 v1 必做的监控+进程、磁盘管理（含 CIFS/NFS/SMART）、文件管理、共享管理（samba/nfs）。
- 关键约束：最小权限原则（root/capabilities 仅在必要时使用）；CLI 与 API 同源逻辑、错误码与输出语义一致；高危操作具备审计记录。
- 技术路线（来自 research）：采用“领域能力内聚 + 适配器隔离系统差异”的分层结构；OpenAPI 作为 API 契约源（OpenAPI-first）；CLI/API 复用同一 service 层。

## Technical Context

**Language/Version**: Go 1.25.7（见 go.mod）  
**Primary Dependencies**: 
- CLI：Cobra（命令组织与帮助/补全生态）
- HTTP：标准库 net/http + 轻量路由（Chi）
- 系统信息：优先 gopsutil（并为后续替换为 /proc 直读保留接口）
- OpenAPI：OpenAPI-first（openapi-v1.yaml 作为源），配合代码生成或校验工具防漂移
- 日志：标准库 log/slog（结构化日志 + 审计日志）
**Storage**: 以文件为主（配置文件、审计日志、运行时状态可选），不引入数据库作为 v1 前提  
**Testing**: `go test ./...`；单元测试（service 逻辑）+ 契约测试（对照 OpenAPI 与错误结构）；集成测试（可选，依赖 CI 能力与权限）  
**Target Platform**: Linux（systemd 作为主要常驻形态目标；不同发行版差异通过适配层吸收）
**Project Type**: CLI + daemon（常驻）+ web-service（REST API）  
**Performance Goals**: 只读查询在典型主机规模下 P95 < 1s；高频查询不拖垮系统（采样/缓存/分页）  
**Constraints**: 最小权限；高危操作审计；错误结构稳定；CLI `--json` 与 API 核心字段对齐；鉴权默认 `X-API-Key`（`/health` 例外）  
**Scale/Scope**: 单机 agent；API 主要服务于内网/受控环境；v1 范围以四大模块为主

## Constitution Check

*GATE: Phase 0 前必须通过；Phase 1 设计后需要复核。*

依据 `.specify/memory/constitution.md` 的门禁：

- Gate A（体验一致性）：CLI 与 API 必须复用同一 service 层；统一错误码；CLI 支持 `--json` 且字段与 API 对齐。
- Gate B（代码质量）：Linux 专用实现隔离在边界清晰的包中；错误必须显式处理且不泄露敏感信息。
- Gate C（测试标准）：所有对外行为变更必须有测试（至少单元测试 + 核心契约测试）；典型失败路径可回归。
- Gate D（性能与可靠性）：只读接口可控（分页/限量/缓存）；长耗时操作支持超时/取消；破坏性操作幂等与失败恢复策略明确。

结论（当前）：通过。Phase 1 输出的 contracts 与 data-model 需再次对照 Gate A/C/D 校验。

## Project Structure

### Documentation (this feature)

```text
specs/001-linux-ops-agent/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
cmd/
├── mingyue/              # CLI 入口
└── mingyued/             # 守护进程入口（HTTP API）

internal/
├── api/                  # HTTP handlers（仅做协议适配）
├── auth/                 # 认证/授权（P0：最小可用）
├── audit/                # 审计事件记录
├── domain/               # 领域模型（与 contracts/data-model 对齐）
├── service/              # 业务编排（CLI/API 复用）
└── sys/                  # Linux 系统交互适配层（进程/挂载/共享/文件等）

pkg/
└── client/               # （可选）API 客户端库，供集成测试或外部集成

test/
├── contract/             # API 契约测试（对照 OpenAPI 与错误结构）
└── integration/          # （可选）依赖权限/环境的集成测试
```

**Structure Decision**: 单仓库单项目结构；通过 `cmd/` 区分 CLI 与 daemon；`internal/service` 作为同源逻辑核心；`internal/sys` 隔离 Linux 交互与发行版差异。

## Complexity Tracking

无。

## Phase 0: Outline & Research (output: research.md)

目标：完成技术选型与关键不确定点收敛，输出可执行的决策记录。

- 产物：`research.md`
- 覆盖：CLI/HTTP 框架、OpenAPI 同步策略、系统信息采集方式、挂载/共享实现策略、最小权限与 capabilities 方案、审计方案、测试策略、鉴权方式。

## Phase 1: Design & Contracts (outputs: data-model.md, contracts/, quickstart.md)

目标：将 spec 转化为可实现的接口契约与数据模型，并提供最小 quickstart。

- `data-model.md`：定义实体、字段、关系、验证规则、状态机（挂载/共享/审计）。
- `contracts/`：API 契约（端点、错误结构、认证方式、主要 schema），并提供 OpenAPI v1 骨架。
- `quickstart.md`：开发者快速上手（构建、运行 CLI/daemon、OpenAPI 位置、运行测试）。

## Phase 1: Constitution Re-check

复核结论（基于已生成产物：`research.md`、`data-model.md`、`contracts/`、`quickstart.md`）：通过。

- Gate A：`data-model.md` 明确统一错误结构（ErrorResponse）与核心实体；`contracts/openapi-v1.yaml` 明确 OpenAPI-first；鉴权方式已收敛为 `X-API-Key`（`/health` 例外），减少 CLI/API 漂移风险。
- Gate C：计划已明确“service 单元测试 + 契约测试”的优先级；实现阶段需对核心端点与典型失败路径（权限不足、目标不存在、参数非法）补齐回归测试。
- Gate D：`data-model.md` 与 `contracts/http-api.md` 已标注分页/限量与幂等语义要求；实现阶段需在 service/sys 层用 `context` 贯穿超时/取消，并将超时错误码语义固化到契约。

## Phase 2: Planning (stop here)

本阶段仅形成后续 tasks 的拆分原则（不生成 tasks.md）：

- 先打通骨架：命令框架、daemon 启动、/health、统一错误结构、鉴权/授权占位、审计日志管道。
- 再按“独立可交付切片”推进：监控+进程 → 挂载/SMART → 文件 → 共享。
- 每个切片都必须同时落地：CLI 命令 + API 端点 + OpenAPI 更新 + 测试（单元/契约）。
