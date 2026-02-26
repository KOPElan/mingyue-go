# mingyue-go

`mingyue` 是一个面向 Linux 主机的系统操作代理，以 **CLI 命令行工具**与**常驻守护进程（HTTP+JSON API）**两种形态提供系统监控、进程管理、磁盘管理、文件管理、共享管理等能力，可作为 Web 可视化管理平台的后端 agent 组件。

## 功能特性

| 模块 | 说明 | 优先级 |
|------|------|--------|
| **系统监控** | CPU、内存、磁盘使用概览与进程列表 | P0 (v1) |
| **进程管理** | 进程查询、按 PID 终止进程 | P0 (v1) |
| **磁盘管理** | 本地磁盘 / CIFS / NFS 挂载与卸载、SMART 健康信息 | P0 (v1) |
| **文件管理** | 目录列表、文件元信息、创建/删除/移动/复制/读写（含路径安全校验） | P0 (v1) |
| **共享管理** | Samba / NFS 共享的查询、创建、修改、删除 | P0 (v1) |
| **网络管理** | 网络接口、地址、路由查询与受控变更 | P1 (后续) |
| **权限/ACL** | 文件/目录权限与 ACL 查询与设置 | P1 (后续) |
| **OpenAPI 规范** | 所有 API 端点的 OpenAPI v3 规范文件，供平台集成与客户端生成 | P0 (v1) |
| **审计日志** | 所有变更类操作的结构化审计记录（时间/来源/操作/结果） | P0 (v1) |

## 快速开始

### 构建

```sh
go build ./...
```

编译后在 `cmd/mingyue/` 目录生成 `mingyue` 二进制文件，或使用：

```sh
go build -o mingyue ./cmd/mingyue
```

### 运行（CLI 模式）

无需启动守护进程，直接执行二进制完成常见操作：

```sh
# 查看帮助
./mingyue --help

# 查看版本
./mingyue version

# 查看系统概览（人类可读）
./mingyue system overview

# 查看系统概览（JSON 输出，适用于脚本化）
./mingyue system overview --json

# 查看进程列表
./mingyue process list --limit 20

# 终止进程
./mingyue process kill <pid>

# 查看挂载列表
./mingyue disk list

# 挂载本地磁盘
./mingyue disk mount --type local --source /dev/sdb1 --target /mnt/data

# 挂载 CIFS 共享
./mingyue disk mount --type cifs --source //server/share --target /mnt/cifs

# 挂载 NFS 共享
./mingyue disk mount --type nfs --source server:/export --target /mnt/nfs

# 卸载
./mingyue disk umount /mnt/data

# 查看磁盘 SMART 信息
./mingyue disk smart /dev/sda

# 文件列表
./mingyue file list /var/log

# 共享列表
./mingyue share list
```

### 运行（守护进程模式）

以守护进程方式启动，对外提供 RESTful HTTP+JSON API（默认端口 `8080`，路径前缀 `/api/v1`）：

```sh
# 启动守护进程
./mingyue agent start

# 验证健康检查
curl http://localhost:8080/api/v1/health

# 获取系统概览
curl http://localhost:8080/api/v1/system/overview

# 获取进程列表
curl http://localhost:8080/api/v1/processes?limit=20

# 查看挂载信息
curl http://localhost:8080/api/v1/disks/mounts

# 查看守护进程状态
./mingyue agent status

# 停止守护进程
./mingyue agent stop
```

### 安装为系统服务

使用提供的安装脚本将 `mingyue` 注册为 systemd 服务，实现开机自启：

```sh
# 安装（需要 root 权限）
sudo bash scripts/install.sh

# 验证服务状态
systemctl status mingyue

# 卸载
sudo bash scripts/uninstall.sh
```

安装脚本会自动完成：检测 systemd 环境、复制二进制文件、创建 systemd service unit、启动服务。

## API 文档

项目提供 OpenAPI v3 规范文件，位于：

```
docs/api/openapi.yaml    # YAML 格式（主要版本）
docs/api/openapi.json    # JSON 格式（按需使用）
```

规范文件覆盖 v1 全量端点，包含认证方式、请求/响应结构与错误结构，可直接用于：
- 生成强类型客户端（`oapi-codegen`、`openapi-generator` 等）
- 在 Swagger UI / Redoc 中渲染交互式文档
- 集成到 Web 可视化管理平台

所有 API 端点位于路径前缀 `/api/v1`，返回 HTTP+JSON，错误响应包含机器可解析的错误码与人类可读信息。

## 项目结构

```text
cmd/
└── mingyue/
    └── main.go                  # CLI 入口（cobra root command）

internal/
├── agent/                       # 守护进程核心逻辑（启动/停止/信号处理）
├── api/                         # HTTP API 路由注册与处理器
│   ├── router.go                # 路由注册（/api/v1/...）
│   ├── middleware/              # 认证、限流、审计中间件
│   ├── system.go                # 系统监控端点
│   ├── process.go               # 进程管理端点
│   ├── disk.go                  # 磁盘与挂载端点
│   ├── file.go                  # 文件管理端点
│   └── share.go                 # 共享管理端点
├── cli/                         # CLI 子命令处理器
│   ├── root.go                  # 根命令（--json / --config 全局 flag）
│   ├── system.go                # mingyue system ...
│   ├── process.go               # mingyue process ...
│   ├── disk.go                  # mingyue disk ...
│   ├── file.go                  # mingyue file ...
│   └── share.go                 # mingyue share ...
├── domain/                      # 领域模型（纯数据结构）
├── service/                     # 核心业务逻辑（CLI 与 API 共用同源实现）
│   ├── system/                  # 系统监控
│   ├── process/                 # 进程管理
│   ├── disk/                    # 磁盘与挂载管理
│   ├── file/                    # 文件管理（含路径安全校验）
│   ├── share/                   # 共享管理
│   ├── network/                 # 网络管理（P1 后续）
│   └── acl/                     # 权限/ACL 管理（P1 后续）
├── auth/                        # 认证鉴权（token/API key；viewer/operator/admin 角色）
├── audit/                       # 审计日志（结构化写入 /var/log/mingyue/audit.log）
└── errors/                      # 统一错误结构（错误码 + 人类可读信息）

pkg/
└── linux/                       # Linux 系统底层封装（隔离平台专用能力）
    ├── exec.go                  # exec.CommandContext 封装
    ├── proc.go                  # /proc 与 /sys 读取工具
    └── capabilities.go          # Linux capabilities 检测

docs/
└── api/
    ├── openapi.yaml             # OpenAPI v3 规范（v1 全量端点）
    └── openapi.json             # 同上，JSON 格式

scripts/
├── install.sh                   # 安装守护进程（systemd service 注册）
└── uninstall.sh                 # 卸载脚本

tests/
├── contract/                    # API 契约测试（httptest）
├── integration/                 # 集成测试（需 Linux 环境）
└── unit/                        # 跨包单元测试工具

specs/
└── 001-linux-ops-agent/
    ├── spec.md                  # 功能规格说明
    ├── plan.md                  # 实现计划
    └── ...                      # 其他设计文档
```

> **设计原则**：CLI 与 API 共享同一套 `internal/service/` 业务层实现，确保行为一致（详见 [ADR-002](docs/adr/0002-shared-business-logic.md)）。Linux 专用系统调用全部隔离在 `pkg/linux/` 中（详见 [ADR-005](docs/adr/0005-minimal-privilege-linux-capabilities.md)）。

## 开发指南

```sh
# 构建
go build ./...

# 运行测试
go test ./...

# 整理依赖
go mod tidy

# 代码检查
go vet ./...
```

**配置与日志路径**：

| 路径 | 用途 |
|------|------|
| `/etc/mingyue/` | 配置文件 |
| `/var/lib/mingyue/` | 运行时状态（PID 文件等） |
| `/var/log/mingyue/` | 运行日志与审计日志（`audit.log`） |

**CI 质量门禁**：每个 PR 自动执行 `go mod tidy` + `go build ./...` + `go test ./...` + `go vet`，以及 OpenAPI 规范同步检查。

## 架构决策

项目的重要架构决策记录在 [docs/adr/README.md](docs/adr/README.md)，涵盖：

| ADR | 决策 |
|-----|------|
| [ADR-001](docs/adr/0001-cli-daemon-dual-mode.md) | 采用 CLI + 守护进程双运行形态 |
| [ADR-002](docs/adr/0002-shared-business-logic.md) | CLI 与 API 共享同源业务逻辑层 |
| [ADR-003](docs/adr/0003-restful-api-openapi.md) | 采用 RESTful HTTP+JSON API 并提供 OpenAPI 规范 |
| [ADR-004](docs/adr/0004-unified-error-structure.md) | 统一错误结构与错误码 |
| [ADR-005](docs/adr/0005-minimal-privilege-linux-capabilities.md) | 最小权限原则与 Linux Capabilities |
| [ADR-006](docs/adr/0006-audit-logging.md) | 审计日志设计 |
| [ADR-007](docs/adr/0007-system-info-collection.md) | 系统信息采集方案选型（/proc + gopsutil） |
| [ADR-008](docs/adr/0008-path-security.md) | 路径安全与目录穿越防护 |

## 权限要求

遵循最小权限原则，默认以最低权限运行，仅在必要场景使用 root 或 Linux capabilities：

| 功能 | 所需权限 |
|------|----------|
| 系统监控（CPU/内存/磁盘/进程列表，只读） | 普通用户即可 |
| 进程终止（kill） | 需要目标进程同 UID 或 `CAP_KILL` |
| 本地磁盘挂载/卸载 | 需要 root 或 `CAP_SYS_ADMIN` |
| CIFS/NFS 挂载/卸载 | 需要 root 或 `CAP_SYS_ADMIN` |
| SMART 信息读取 | 需要 root 或 `CAP_SYS_RAWIO`，以及 `smartctl` 工具 |
| 共享管理（Samba/NFS 变更） | 需要 root，以及相应服务可用（`smbd`/`exportfs`） |
| 网络接口变更 | 需要 `CAP_NET_ADMIN` |

权限不足时，系统返回可解释的错误信息并提示所需权限，不会静默失败。

> CIFS 凭据通过环境变量或 `/run` 下临时文件传递，**绝不出现在命令行参数、日志或 API 响应中**。

## 里程碑

| 阶段 | 内容 | 预计周期 |
|------|------|----------|
| **Phase 1** | 基础骨架：CLI 框架、守护进程框架、HTTP API 基础、统一错误结构、认证鉴权草案、审计日志骨架、CI 基础 | 2 周 |
| **Phase 2** | 系统监控 + 进程管理：CPU/内存/磁盘概览、进程列表与终止、CLI/API 对齐 | 2 周 |
| **Phase 3** | 磁盘管理：本地挂载/卸载、CIFS/NFS 挂载/卸载、SMART 信息、幂等与审计 | 2 周 |
| **Phase 4** | 文件管理 + 共享管理：文件操作（路径安全）、Samba/NFS 共享 CRUD、审计日志完善 | 2 周+ |
| **Phase 5** | 网络管理 + 权限/ACL（P1 迭代） | 待定 |
| **Phase 6** | OpenAPI 规范 + CI 文档同步 + 安装脚本（`install.sh`）+ README | 1 周 |

## License

MIT
