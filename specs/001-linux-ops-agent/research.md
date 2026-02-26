# Phase 0 Research: Linux 系统操作代理（CLI + REST API + OpenAPI）

> 目标：收敛关键技术选型与不确定点，形成可追溯的决策记录。

## Decision 1: CLI 框架选型（Cobra）

- Decision: 使用 `spf13/cobra` 组织 CLI 命令与帮助体系，并统一提供 `--json` 输出能力。
- Rationale: 适配复杂子命令结构、帮助/补全成熟；易于保持命令一致性。
- Alternatives considered:
  - 标准库 `flag`: 适合简单命令，但子命令/帮助体验与扩展性较弱。
  - `urfave/cli`: 也可行，但生态与团队熟悉度需再评估。

## Decision 2: HTTP 框架选型（net/http + Chi）

- Decision: 基于标准库 `net/http` 实现服务，使用轻量路由（`go-chi/chi`）组织路由与中间件。
- Rationale: 依赖轻、可控；便于与 OpenAPI-first 生成/校验工具结合；性能与可观测扩展空间充足。
- Alternatives considered:
  - Gin: 开箱即用但偏重，且部分约束可能不利于契约优先。
  - 纯 `net/http`: 可行但路由/中间件组织成本更高。

## Decision 3: OpenAPI 同步策略（OpenAPI-first）

- Decision: 以 `contracts/openapi-v1.yaml` 作为契约源，优先采用生成/校验机制确保实现与文档不漂移。
- Rationale: 该项目明确要求“标准 API + OpenAPI 交付物”；OpenAPI-first 最能降低对接成本与漂移风险。
- Alternatives considered:
  - Code-first 注解生成（如 swag 风格）: 容易随代码演进出现文档缺口或不一致。

## Decision 4: 系统指标采集（优先 gopsutil，保留可替换接口）

- Decision: v1 优先采用 `gopsutil` 获取 CPU/内存/磁盘/进程等信息；通过 `internal/sys` 抽象接口保留未来替换为 `/proc` 直读的可能。
- Rationale: 兼容性与开发速度更优；接口抽象可避免后续锁死实现细节。
- Alternatives considered:
  - 直接读 `/proc`/`/sys`: 性能更可控但实现复杂、字段兼容成本高。

## Decision 5: 挂载/卸载策略（系统能力适配 + 幂等）

- Decision: 将挂载/卸载、CIFS/NFS、SMART 读取等能力统一抽象在 `internal/sys`；对外保证幂等语义与可定位错误。
- Rationale: 不同发行版/环境差异较大，需要隔离边界并统一错误语义；高危操作必须可控。
- Alternatives considered:
  - 直接在 handler/CLI 中调用系统命令：会导致逻辑重复且难以测试与审计。

## Decision 6: Samba/NFS 共享管理（最小可用 + 可插拔）

- Decision: v1 先定义最小可用的共享“查询/创建/修改/删除”契约；具体实现按发行版/配置路径可插拔，并允许配置共享配置文件路径/重载方式。
- Rationale: samba/nfs 的配置管理在不同发行版差异显著；契约先行可保障 API 稳定。
- Alternatives considered:
  - 强依赖某发行版默认路径与命令：会导致可移植性差。

## Decision 7: 权限与最小权限（root/capabilities 按需）

- Decision: 默认以非特权运行；仅对确需特权的操作要求 root 或指定 capabilities（例如 CAP_SYS_ADMIN、CAP_NET_ADMIN）。
- Rationale: 满足最小权限原则与审计要求，降低误操作与攻击面。
- Alternatives considered:
  - 全程 root: 实现简单但风险过高。

## Decision 8: 审计与日志（结构化审计事件）

- Decision: 对变更类操作产出结构化审计事件（时间、来源、对象、结果、错误码），与普通日志区分；敏感信息不入日志。
- Rationale: 满足宪章与 PRD 的可追溯要求，便于上层收集与检索。
- Alternatives considered:
  - 仅记录文本日志：难检索、难用于合规审计。

## Decision 9: 测试策略（单元 + 契约优先，集成测试可选）

- Decision: 优先保证：service 层单元测试 + API 契约测试（错误结构、关键端点）；涉及系统特权/外部依赖的集成测试可在有条件环境中启用。
- Rationale: CI 环境可能缺少特权与依赖；契约与单元测试可提供稳定回归能力。
- Alternatives considered:
  - 全量集成测试：可靠性依赖环境，维护成本高。

## Decision 10: 鉴权方式（v1 默认 API Key）

- Decision: v1 默认使用 API Key（`X-API-Key` header）；`/health` 允许匿名访问，其它端点默认需要鉴权。
- Rationale: 面向内网/受控环境的最小可用方案，落地成本低，易于在 CLI 与 HTTP 客户端统一实现。
- Alternatives considered:
  - Bearer Token: 也可行，但需要额外的 token 生命周期管理策略。
  - mTLS: 安全性更强但部署与运维门槛更高，不适合作为 v1 默认。

## Open questions（后续细化，不阻塞 Phase 1 产物）

- 共享管理实现的首批发行版目标（Debian/Ubuntu vs RHEL/CentOS）。
- SMART 数据来源与依赖（是否允许依赖系统工具，或完全通过设备接口）。
