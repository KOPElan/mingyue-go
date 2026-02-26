# Implementation Plan: Linux 系统操作代理（CLI + 标准 API）

**Branch**: `001-linux-ops-agent` | **Date**: 2026-02-27 | **Spec**: [spec.md](./spec.md)  
**Input**: Feature specification from `/specs/001-linux-ops-agent/spec.md`

---

## Summary

本项目以 Go 实现一个面向 Linux 主机的"系统操作代理（mingyue）"，同时以 **CLI 命令行工具**与**常驻守护进程（HTTP+JSON API）**两种形态对外提供能力。核心诉求：

- **CLI**：运维人员可在终端通过统一命令完成系统监控、进程管理、磁盘挂载、文件管理、共享管理等操作；支持默认人类可读输出与 `--json` 机器可读输出。
- **API**：守护进程暴露 RESTful HTTP+JSON API（路径前缀 `/api/v1`），供 Web 可视化平台或自动化工具调用；OpenAPI/Swagger 规范作为标准交付物随版本一同发布。
- **同源逻辑**：CLI 与 API 必须共享同一套 `internal/service` 层实现，禁止逻辑复制导致行为漂移（宪章 I）。
- **安全基线**：最小权限原则、路径安全校验（防目录穿越）、审计日志（时间/来源/操作/结果，不含敏感信息）。

技术方案：使用 **cobra** 构建 CLI 框架，**net/http + 路由层（gin 或标准库 mux）** 构建 HTTP API，**gopsutil** 采集系统指标，`context` 贯穿所有长耗时/阻塞系统调用以支持超时与取消。

---

## Technical Context

**Language/Version**: Go 1.25.7  
**Primary Dependencies**:
- `github.com/spf13/cobra` — CLI 框架，子命令组织
- `github.com/gin-gonic/gin` 或 `net/http` 标准库 — HTTP API 路由与处理器
- `github.com/shirou/gopsutil/v3` — CPU/内存/磁盘/进程系统信息采集
- 标准库 `os/exec` — 系统命令封装（mount/umount/smartctl 等）
- 标准库 `context` — 超时与取消传播

**Storage**:
- 配置文件：`/etc/mingyue/`
- 运行时状态：`/var/lib/mingyue/`
- 审计与运行日志：`/var/log/mingyue/`

**Testing**: `go test ./...`（标准库 `testing`，表驱动单元测试 + 契约集成测试）  
**Target Platform**: Linux server（x86_64；各主流发行版：Ubuntu/Debian/RHEL/CentOS）  
**Project Type**: CLI + daemon/service（单机 agent，非跨主机调度）  
**Performance Goals**:
- 只读查询（概览/挂载列表/共享列表）P95 < 1s（规格来源：spec.md SC-002）
- 守护进程空闲与高频调用下 CPU/内存占用可控

**Constraints**:
- 最小权限原则：默认非 root，仅在必要场景（挂载/SMART/共享变更等）以 root 或最小化 Linux capabilities（CAP_SYS_ADMIN / CAP_NET_ADMIN）运行
- 审计日志：所有变更类操作必须记录，日志不得包含敏感信息（密码/密钥）
- 路径安全：文件与共享相关操作需严格校验路径，防止目录穿越（`../` 等）
- API 向后兼容：破坏性变更必须声明并提供迁移路径（宪章 II）
- 并发安全：共享状态必须可证明无数据竞争（宪章 IV）

**Scale/Scope**: 单机部署；v1 范围覆盖监控+进程、磁盘管理、文件管理、共享管理；网络管理与权限/ACL 为后续 P1 迭代。

---

## Constitution Check

*GATE：Phase 1（骨架）开始前必须通过；Phase 2 设计完成后复查。*

| # | 宪章条款 | 检查结果 | 落地要求 |
|---|----------|----------|----------|
| C-01 | **I. CLI 与 API 同源**：禁止复制业务逻辑 | ✅ 通过 | `internal/service` 统一服务层，CLI handler 与 HTTP handler 均调用同一函数 |
| C-02 | **I. 输出规范**：CLI 默认人类可读；`--json` 机器可读且结构稳定 | ✅ 通过 | cobra 根命令注册全局 `--json` flag；输出格式通过测试锁定 |
| C-03 | **I. 错误语义一致**：CLI `--json` 输出与 API 响应在核心字段对齐 | ✅ 通过 | `internal/errors` 统一错误结构（错误码 + 人类可读信息），CLI/API 共用 |
| C-04 | **II. 平台约束隔离**：Linux 专用能力必须隔离在明确包边界 | ✅ 通过 | `pkg/linux/` 封装所有与 `/proc`、`mount`、`smartctl` 等的直接交互 |
| C-05 | **II. 错误显式处理**：错误信息可定位但不泄露敏感信息 | ✅ 通过 | CIFS 凭据等敏感字段在日志与响应中脱敏；错误信息含关键上下文 |
| C-06 | **III. 测试标准前置**：新增对外行为必须有测试覆盖 | ✅ 通过 | 每个 Phase 的 DoD 包含：单元测试（表驱动）+ API 契约测试（成功/典型失败路径） |
| C-07 | **III. 集成测试契约**：进程管理/挂载/共享等入口需有可重复验证策略 | ✅ 通过 | 使用 mock/stub 隔离系统调用；集成测试在 Linux CI 环境中运行 |
| C-08 | **IV. 超时与取消**：长耗时系统调用必须支持 `context` 贯穿 | ✅ 通过 | `mount`/`umount`/`smartctl`/共享服务重载等通过 `exec.CommandContext` 传递 ctx |
| C-09 | **IV. 幂等语义**：破坏性操作需有明确幂等语义与失败恢复策略 | ✅ 通过 | 挂载/卸载/删除共享等在 spec 验收场景中已定义幂等语义；Phase 3/4 落地 |
| C-10 | **Quality Gates**：`go mod tidy` + `go build ./...` + `go test ./...` 必须通过 | ✅ 通过 | CI pipeline 在每个 PR 中强制执行三项门禁 |

**结论**：无宪章违规项，无需填写 Complexity Tracking。

---

## Project Structure

### 文档产出（本 Feature）

```text
specs/001-linux-ops-agent/
├── plan.md              # 本文件（/speckit.plan 产出）
├── research.md          # Phase 0 产出（技术选型与系统调用调研）
├── data-model.md        # Phase 1 产出（领域模型与 JSON 结构定义）
├── quickstart.md        # Phase 1 产出（5 分钟快速上手指南）
├── contracts/           # Phase 1 产出（CLI/API 契约文件）
│   ├── cli-commands.md  # CLI 命令契约（参数/输出字段/退出码）
│   └── api-routes.md    # HTTP API 路由契约（请求/响应/错误结构）
└── tasks.md             # Phase 2 产出（/speckit.tasks 命令生成，本文件不创建）
```

### 源码结构（仓库根目录）

```text
cmd/
└── mingyue/
    └── main.go                  # CLI 入口（cobra root command）

internal/
├── agent/                       # 守护进程核心逻辑（启动/停止/信号处理）
│   ├── daemon.go
│   └── daemon_test.go
├── api/                         # HTTP API 路由注册与处理器
│   ├── router.go                # 路由注册（/api/v1/...）
│   ├── middleware/              # 认证、限流、审计中间件
│   │   ├── auth.go
│   │   └── audit.go
│   ├── system.go                # 系统监控端点
│   ├── process.go               # 进程管理端点
│   ├── disk.go                  # 磁盘与挂载端点
│   ├── file.go                  # 文件管理端点
│   ├── share.go                 # 共享管理端点
│   └── *_test.go                # 各模块契约测试
├── cli/                         # CLI 子命令处理器
│   ├── root.go                  # 根命令（--json / --config 全局 flag）
│   ├── system.go                # mingyue system ...
│   ├── process.go               # mingyue process ...
│   ├── disk.go                  # mingyue disk ...
│   ├── file.go                  # mingyue file ...
│   └── share.go                 # mingyue share ...
├── domain/                      # 领域模型（只含纯数据结构，无业务逻辑）
│   ├── snapshot.go              # HostSnapshot
│   ├── process.go               # Process
│   ├── mount.go                 # Mount
│   ├── disk.go                  # DiskHealth
│   ├── file.go                  # FileEntry
│   ├── share.go                 # Share
│   └── audit.go                 # AuditEvent
├── service/                     # 核心业务逻辑（CLI 与 API 共用）
│   ├── system/                  # 系统监控（CPU/内存/磁盘概览）
│   │   ├── monitor.go
│   │   └── monitor_test.go
│   ├── process/                 # 进程查询与管理
│   │   ├── manager.go
│   │   └── manager_test.go
│   ├── disk/                    # 磁盘与挂载管理（本地 + CIFS/NFS + SMART）
│   │   ├── mount.go
│   │   ├── smart.go
│   │   └── *_test.go
│   ├── file/                    # 文件管理（路径安全校验内聚于此）
│   │   ├── manager.go
│   │   └── manager_test.go
│   ├── share/                   # 共享管理（samba/nfs）
│   │   ├── manager.go
│   │   └── manager_test.go
│   ├── network/                 # 网络管理（P1 后续迭代）
│   └── acl/                     # 权限/ACL 管理（P1 后续迭代）
├── auth/                        # 认证鉴权（token/API key；角色：viewer/operator/admin）
│   ├── auth.go
│   └── auth_test.go
├── audit/                       # 审计日志（结构化写入 /var/log/mingyue/audit.log）
│   ├── logger.go
│   └── logger_test.go
└── errors/                      # 统一错误结构（错误码 + 人类可读信息）
    ├── errors.go
    └── errors_test.go

pkg/
└── linux/                       # Linux 系统底层封装（隔离平台专用能力）
    ├── exec.go                  # exec.CommandContext 封装
    ├── proc.go                  # /proc 与 /sys 读取工具
    └── capabilities.go          # Linux capabilities 检测

docs/
└── api/
    ├── openapi.yaml             # OpenAPI v3 规范（v1 全量端点）
    └── openapi.json             # 同上，JSON 格式（可按需保留一份）

scripts/
├── install.sh                   # 安装守护进程（systemd service 注册）
└── uninstall.sh                 # 卸载脚本

tests/
├── contract/                    # API 契约测试（httptest）
├── integration/                 # 集成测试（需 Linux 环境，CI 中运行）
└── unit/                        # 跨包单元测试工具

.github/
└── workflows/
    ├── ci.yaml                  # go mod tidy / go build / go test / lint
    └── openapi-sync.yaml        # OpenAPI 规范与实现同步检查
```

**结构决策**：采用标准 Go 项目布局（`cmd/` 入口 + `internal/` 不对外暴露的业务包 + `pkg/` 可复用平台封装）。CLI 与 API 的"能力处理器"分别位于 `internal/cli/` 与 `internal/api/`，共同调用 `internal/service/` 中的业务函数，确保同源逻辑（宪章 I）。Linux 专用系统调用全部隔离在 `pkg/linux/` 中（宪章 II）。

---

## 阶段划分

> 每个 Phase 完成后需满足：`go build ./...` + `go test ./...` 全部通过，且 Phase DoD 中所有验收条件满足方可进入下一阶段。

---

### Phase 1：基础骨架（预计 2 周）

**目标**：搭建可运行的项目骨架，确立所有后续模块的基础约定。

**交付物**：
1. **CLI 框架**：`cmd/mingyue/main.go` + cobra 根命令，全局 `--json`/`--config` flag，`mingyue version` 与 `mingyue help` 可用。
2. **守护进程框架**：`internal/agent/` 实现 `start`/`stop`/`status`，处理 SIGTERM/SIGINT，输出 PID 文件。
3. **HTTP API 基础**：`internal/api/router.go` 注册 `/api/v1/health` 健康检查端点；中间件占位（认证/审计）。
4. **统一错误结构**：`internal/errors/` 定义 `AppError`（错误码 + 消息 + 可选 cause），CLI JSON 输出与 API 响应复用同一结构。
5. **认证鉴权草案**：`internal/auth/` 实现 token/API key 校验骨架，定义 `viewer`/`operator`/`admin` 角色常量。
6. **审计日志骨架**：`internal/audit/` 实现结构化日志写入（JSON Lines），定义 `AuditEvent` 字段。
7. **CI 基础**：`.github/workflows/ci.yaml` 执行 `go mod tidy` + `go build ./...` + `go test ./...` + `go vet`。

**DoD（完成标准）**：
- `mingyue --help` 可输出帮助；`mingyue version` 输出版本信息。
- `mingyue agent start` 启动守护进程，`curl /api/v1/health` 返回 200 JSON。
- `internal/errors` 单元测试覆盖错误创建与 JSON 序列化。
- CI pipeline 首次通过。

**关键 FR 对应**：FR-001（同时提供 CLI 与 API）、FR-002（两种运行方式）、FR-015（统一错误语义）

---

### Phase 2：系统监控 + 进程管理（预计 2 周）

**目标**：实现第一个可交付的用户场景（spec User Story 1 / GH-003 / GH-004 / GH-005），完成 MVP。

**交付物**：
1. **系统监控**：`internal/service/system/` 使用 gopsutil 采集 CPU/内存/磁盘概览，封装为 `HostSnapshot`。
2. **进程管理**：`internal/service/process/` 实现进程列表（支持分页/限制）、按 PID 查询、终止进程（kill），记录审计日志。
3. **CLI 命令**：
   - `mingyue system overview` — 默认人类可读/`--json`
   - `mingyue process list [--limit N] [--json]`
   - `mingyue process get <pid> [--json]`
   - `mingyue process kill <pid>`
4. **API 端点**：
   - `GET /api/v1/system/overview`
   - `GET /api/v1/processes?limit=N&page=N`
   - `GET /api/v1/processes/:pid`
   - `DELETE /api/v1/processes/:pid`（需 operator/admin 角色）
5. **性能基线**：`/system/overview` 与 `/processes` 在本地基准测试中 P95 < 200ms。

**DoD**：
- CLI/API 返回字段一致（JSON 结构对齐）。
- `process kill` 成功与失败均产生 AuditEvent（不含进程敏感内容）。
- 权限不足时 API 返回 `403 + AppError`，CLI 返回非 0 退出码 + stderr 错误信息。
- 单元测试覆盖：CPU/内存/磁盘采集 mock、进程列表分页边界、kill 权限拒绝场景。
- 契约测试覆盖：`/system/overview` 与 `/processes` 成功/典型失败路径。

**关键 FR 对应**：FR-003、FR-004、FR-005、FR-013、FR-014、FR-017

---

### Phase 3：磁盘管理（本地 + CIFS/NFS + SMART）（预计 2 周）

**目标**：实现 spec User Story 2（GH-006 / GH-007 / GH-008），满足磁盘与挂载管理核心需求。

**交付物**：
1. **挂载信息查询**：`internal/service/disk/mount.go` 读取 `/proc/mounts`，封装为 `[]Mount`。
2. **本地挂载/卸载**：调用 `mount`/`umount` 系统命令（via `exec.CommandContext`），实现幂等语义。
3. **CIFS/NFS 挂载/卸载**：凭据通过进程环境变量或临时文件传递，**绝不写入日志或响应**。
4. **SMART 信息**：调用 `smartctl`（降级：无权限/无工具时返回可解释错误，不给出误导性健康结论）。
5. **CLI 命令**：
   - `mingyue disk list`（等价 `mingyue mount list`）
   - `mingyue disk mount --type local|cifs|nfs ...`
   - `mingyue disk umount <mountpoint>`
   - `mingyue disk smart <device>`
6. **API 端点**：
   - `GET /api/v1/disks/mounts`
   - `POST /api/v1/disks/mounts`
   - `DELETE /api/v1/disks/mounts/:mountpoint`
   - `GET /api/v1/disks/:device/smart`

**DoD**：
- 幂等验证：重复挂载返回"已挂载"（200 或 409 + 明确语义），重复卸载返回"未挂载"语义。
- CIFS 凭据不出现在日志、审计记录或 API 响应中（测试覆盖此约束）。
- SMART 无权限场景返回 `AppError`（错误码 + 建议信息），非 panic。
- 所有挂载/卸载操作记录审计日志。
- `exec.CommandContext` 超时测试覆盖（模拟命令超时，验证 ctx 取消与错误返回）。

**关键 FR 对应**：FR-006、FR-007、FR-008、FR-009、FR-013、FR-018

---

### Phase 4：文件管理 + 共享管理（预计 2 周+）

**目标**：实现 spec User Story 3/4（GH-009 / GH-010），完成 v1 全量功能范围。

**交付物**：
1. **文件管理**：`internal/service/file/` 实现目录列表、文件元信息查询、创建/删除/移动/复制/读写，**路径安全校验**内聚于此包（`filepath.Clean` + 白名单/根路径边界校验，防止 `../` 穿越）。
2. **共享管理（Samba/NFS）**：`internal/service/share/` 实现查询/创建/修改/删除共享配置，调用 `smbd`/`exportfs` 等服务命令，提供失败回滚或恢复建议。
3. **审计日志完善**：文件写操作与共享变更操作写入 `AuditEvent`。
4. **CLI 命令**：
   - `mingyue file list <path>`、`mingyue file stat <path>`
   - `mingyue file mkdir|rm|mv|cp|read|write <path> [args]`
   - `mingyue share list`、`mingyue share get <name>`
   - `mingyue share create|update|delete <name> [flags]`
5. **API 端点**：
   - `GET /api/v1/files?path=...`、`GET /api/v1/files/stat?path=...`
   - `POST /api/v1/files`（创建/写入）、`DELETE /api/v1/files`、`PUT /api/v1/files/move`、`PUT /api/v1/files/copy`
   - `GET /api/v1/shares`、`GET /api/v1/shares/:name`
   - `POST /api/v1/shares`、`PUT /api/v1/shares/:name`、`DELETE /api/v1/shares/:name`

**DoD**：
- 路径穿越测试：`../etc/passwd`、绝对路径跳出根等输入均返回 `403 + AppError`（不执行操作）。
- 共享变更幂等/回滚测试：服务重载失败时回滚至变更前状态（或返回明确恢复建议）。
- 所有破坏性文件操作与共享变更记录审计日志（含来源：CLI 本地/API 客户端）。
- 共享管理集成测试在 CI Linux 环境中可重复运行（使用 mock 或轻量测试 fixture）。

**关键 FR 对应**：FR-010、FR-011、FR-012、FR-013

---

### Phase 5：网络管理 + 权限/ACL（P1 后续优先级）

**目标**：实现 GH-016 / GH-017，满足后续平台对接需求（P1 迭代，v1 范围外）。

**交付物**：
1. **网络管理**：`internal/service/network/` 提供网络接口/地址/路由只读查询；受控的变更操作（启停接口、刷新 DHCP）需 admin 角色与审计。
2. **权限/ACL**：`internal/service/acl/` 提供文件/目录权限查询与 ACL 查询/设置，路径安全校验复用 `internal/service/file/` 校验逻辑。
3. **CLI 命令**：`mingyue network ...`、`mingyue acl ...`
4. **API 端点**：`GET /api/v1/network/interfaces`、`GET /api/v1/acl?path=...`、`PUT /api/v1/acl?path=...`

**DoD**：
- 网络变更操作需 admin 角色；只读查询支持 viewer 角色。
- ACL 设置需路径安全校验，变更记录审计日志。
- 单元测试与契约测试完整覆盖。

**关键 FR 对应**：FR-003（扩展网络信息）、FR-013、FR-015

---

### Phase 6：OpenAPI 规范 + CI 文档同步 + 安装脚本（预计 1 周）

**目标**：完成 spec User Story 5（GH-011），交付标准 API 文档与安装能力，达成 SC-004。

**交付物**：
1. **OpenAPI 规范**：`docs/api/openapi.yaml`，覆盖 v1 全量端点（含认证方式描述、请求/响应结构、错误结构）。
2. **CI 同步机制**：`.github/workflows/openapi-sync.yaml`，在 PR 合并时校验 API 路由变更是否同步更新了 OpenAPI 文件（例如使用 `oapi-codegen` 或 diff 检查）。
3. **安装脚本**：`scripts/install.sh` — 检测 systemd、复制二进制、创建 systemd service unit、启动服务；`scripts/uninstall.sh` — 反向操作。
4. **README 更新**：包含安装说明、CLI 快速上手、API 访问示例、权限说明。

**DoD**：
- `openapi.yaml` 静态校验通过（使用 `spectral` 或 `redocly lint`）。
- CI 中执行 3 个关键端点冒烟测试（概览/挂载列表/共享列表），验证响应结构与 OpenAPI 文档一致（SC-004）。
- `install.sh` 在 Ubuntu 22.04 测试环境中执行无报错，守护进程可开机自启。

**关键 FR 对应**：FR-016（OpenAPI 交付物）

---

## 关键风险与缓解策略

| 风险 | 影响 | 缓解策略 |
|------|------|----------|
| 最小权限落地困难（不同发行版 capabilities 支持差异） | Phase 3/4 | 文档明确哪些操作需要 root/capabilities；在条件不满足时返回可解释错误，不静默失败 |
| samba/nfs 配置变更回滚复杂 | Phase 4 | 变更前备份配置文件；失败时自动回滚；回滚失败给出恢复步骤 |
| SMART 依赖 `smartctl` 系统工具 | Phase 3 | 优雅降级：检测工具是否存在；无权限/无工具时返回 `AppError` 而非 panic |
| CIFS 凭据安全 | Phase 3 | 凭据通过环境变量或 `/run` 下临时文件传递，命令行参数中不含凭据；日志中脱敏处理 |
| CLI 与 API 行为漂移 | 全阶段 | 强制同源逻辑（宪章 I）；契约测试覆盖 CLI `--json` 输出与 API 响应字段对齐 |
| 并发挂载/卸载竞态 | Phase 3 | 对挂载操作加互斥锁（按设备/挂载点粒度），避免并发状态不一致 |
