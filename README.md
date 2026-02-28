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
| **网络管理** | 网络接口只读查询（list/get）及受控变更（up/down/dhcp）；变更需 admin 角色 | P1 ✅ |
| **权限/ACL** | 文件/目录权限与 POSIX ACL 查询（getfacl 优雅降级）与设置（setfacl）；变更需 operator 角色 | P1 ✅ |
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

# 查看文件元信息
./mingyue file stat /var/log/syslog

# 读取文件内容
./mingyue file read /var/log/syslog

# 创建目录
./mingyue file mkdir /tmp/mydir

# 写入文件（--content 指定内容）
./mingyue file write /tmp/myfile.txt --content "hello"

# 移动文件
./mingyue file mv /tmp/old.txt /tmp/new.txt

# 复制文件
./mingyue file cp /tmp/a.txt /tmp/b.txt

# 删除文件（-r 递归删除目录）
./mingyue file rm /tmp/mydir -r

# 限制文件操作根目录（防止访问根目录以外的路径）
./mingyue file list /data --root /data

# 共享列表
./mingyue share list

# 查看共享详情
./mingyue share get myshare

# 创建 Samba 共享
./mingyue share create myshare --type smb --path /srv/myshare

# 更新共享
./mingyue share update myshare --type smb --path /srv/newpath --read-only

# 删除共享
./mingyue share delete myshare

# 查看网络接口列表
./mingyue network list

# 查看指定接口详情
./mingyue network get eth0

# 启用网络接口（需要 admin 权限）
./mingyue network up eth0

# 禁用网络接口（需要 admin 权限）
./mingyue network down eth0

# 刷新 DHCP 租约（需要 admin 权限）
./mingyue network dhcp eth0

# 查询文件/目录权限与 ACL
./mingyue acl get /srv/data

# 设置 POSIX ACL（需要安装 setfacl）
./mingyue acl set /srv/data --entry u:alice:rwx --entry g:devs:r-x

# 限制 ACL 操作根目录（防止访问根目录以外的路径）
./mingyue acl get /data/file.txt --root /data
```

### 运行（守护进程模式）

以守护进程方式启动，对外提供 RESTful HTTP+JSON API（默认端口 `7070`，路径前缀 `/api/v1`）：

```sh
# 启动守护进程
./mingyue agent start

# 验证健康检查（无需认证）
curl http://localhost:7070/api/v1/health

# 获取系统概览（需要 Bearer Token）
curl -H "Authorization: Bearer <api-key>" http://localhost:7070/api/v1/system/overview

# 获取进程列表（支持分页）
curl -H "Authorization: Bearer <api-key>" "http://localhost:7070/api/v1/processes?limit=20&page=1"

# 查询指定进程
curl -H "Authorization: Bearer <api-key>" http://localhost:7070/api/v1/processes/1

# 终止进程（需要 operator 或 admin 角色）
curl -X DELETE -H "Authorization: Bearer <api-key>" http://localhost:7070/api/v1/processes/1234

# 查看守护进程状态
./mingyue agent status

# 停止守护进程
./mingyue agent stop
```

### 文件管理 API 示例

```sh
# 列出目录
curl -H "Authorization: Bearer <api-key>" "http://localhost:7070/api/v1/files?path=/var/log"

# 查询文件元信息
curl -H "Authorization: Bearer <api-key>" "http://localhost:7070/api/v1/files/stat?path=/var/log/syslog"

# 读取文件内容（响应 content 字段为 base64 编码）
curl -H "Authorization: Bearer <api-key>" "http://localhost:7070/api/v1/files/read?path=/var/log/syslog"

# 创建文件（需要 operator/admin 角色，content 为 base64 编码）
curl -X POST -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
     -d '{"path":"/tmp/hello.txt","type":"file","content":"aGVsbG8="}' \
     http://localhost:7070/api/v1/files

# 创建目录（需要 operator/admin 角色）
curl -X POST -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
     -d '{"path":"/tmp/mydir","type":"dir"}' \
     http://localhost:7070/api/v1/files

# 移动文件（需要 operator/admin 角色）
curl -X PUT -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
     -d '{"src":"/tmp/old.txt","dst":"/tmp/new.txt"}' \
     http://localhost:7070/api/v1/files/move

# 复制文件（需要 operator/admin 角色）
curl -X PUT -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
     -d '{"src":"/tmp/a.txt","dst":"/tmp/b.txt"}' \
     http://localhost:7070/api/v1/files/copy

# 删除文件（需要 operator/admin 角色；加 recursive=true 递归删除目录）
curl -X DELETE -H "Authorization: Bearer <api-key>" \
     "http://localhost:7070/api/v1/files?path=/tmp/mydir&recursive=true"
```

### 共享管理 API 示例

```sh
# 列出所有共享（需要 viewer 或以上角色）
curl -H "Authorization: Bearer <api-key>" http://localhost:7070/api/v1/shares

# 查询指定共享
curl -H "Authorization: Bearer <api-key>" http://localhost:7070/api/v1/shares/myshare

# 创建共享（需要 operator/admin 角色）
curl -X POST -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
     -d '{"name":"myshare","type":"smb","path":"/srv/myshare","enabled":true}' \
     http://localhost:7070/api/v1/shares

# 更新共享（需要 operator/admin 角色）
curl -X PUT -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
     -d '{"type":"smb","path":"/srv/newpath","read_only":true,"enabled":true}' \
     http://localhost:7070/api/v1/shares/myshare

# 删除共享（需要 operator/admin 角色）
curl -X DELETE -H "Authorization: Bearer <api-key>" http://localhost:7070/api/v1/shares/myshare
```

### 网络管理 API 示例

```sh
# 列出所有网络接口（需要 viewer 或以上角色）
curl -H "Authorization: Bearer <api-key>" http://localhost:7070/api/v1/network/interfaces

# 查询单个接口详情
curl -H "Authorization: Bearer <api-key>" http://localhost:7070/api/v1/network/interfaces/eth0

# 启用接口（需要 admin 角色）
curl -X PUT -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
     -d '{"action":"up"}' \
     http://localhost:7070/api/v1/network/interfaces/eth0

# 禁用接口（需要 admin 角色）
curl -X PUT -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
     -d '{"action":"down"}' \
     http://localhost:7070/api/v1/network/interfaces/eth0

# 刷新 DHCP 租约（需要 admin 角色）
curl -X PUT -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
     -d '{"action":"dhcp"}' \
     http://localhost:7070/api/v1/network/interfaces/eth0
```

### 权限/ACL 管理 API 示例

```sh
# 查询文件/目录权限与 POSIX ACL（需要 viewer 或以上角色）
curl -H "Authorization: Bearer <api-key>" "http://localhost:7070/api/v1/acl?path=/srv/data"

# 设置 POSIX ACL 条目（需要 operator/admin 角色；setfacl 需已安装）
curl -X PUT -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
     -d '{"path":"/srv/data","entries":["u:alice:rwx","g:devs:r-x"]}' \
     http://localhost:7070/api/v1/acl
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

## API 认证

守护进程使用 **Bearer Token（API Key）** 认证。未认证请求返回 `401 UNAUTHORIZED`，权限不足返回 `403 FORBIDDEN`。

**注册 API Key（程序初始化时调用）**

```go
auth.RegisterAPIKey("my-secret-key", auth.Token{
    Raw:     "my-secret-key",
    Role:    auth.RoleOperator,
    Subject: "ops-user",
})
```

**请求示例**

```sh
curl -H "Authorization: Bearer my-secret-key" \
     http://localhost:7070/api/v1/system/overview
```

**角色说明**

| 角色 | 允许操作 |
|------|----------|
| `viewer` | 所有只读操作（系统概览、进程列表/查询、文件列表/stat/read、共享列表/查询、网络接口查询、ACL 查询等） |
| `operator` | 只读操作、进程终止、文件写/删/移/复制（POST/DELETE/PUT /files）、共享创建/更新/删除、ACL 设置（PUT /acl） |
| `admin` | 全部操作（包含网络接口变更：up/down/dhcp） |

详细 API 契约请参阅 [`specs/001-linux-ops-agent/contracts/api-routes.md`](specs/001-linux-ops-agent/contracts/api-routes.md)。

## API 文档

详细端点契约文档位于：

```
specs/001-linux-ops-agent/contracts/
├── api-routes.md       # HTTP API 路由契约（请求/响应/错误结构）
└── cli-commands.md     # CLI 命令契约（参数/输出字段/退出码）
```

项目预留 OpenAPI v3 规范文件路径（Phase 6 交付）：

```
docs/api/openapi.yaml    # YAML 格式（主要版本）
docs/api/openapi.json    # JSON 格式（按需使用）
```

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
│   ├── share.go                 # 共享管理端点（兼容旧路由）
│   ├── smb.go                   # SMB 共享端点
│   ├── nfs.go                   # NFS 导出端点
│   ├── network.go               # 网络接口端点
│   └── acl.go                   # 权限/ACL 端点
├── cli/                         # CLI 子命令处理器
│   ├── root.go                  # 根命令（--json / --config 全局 flag）
│   ├── system.go                # mingyue system ...
│   ├── process.go               # mingyue process ...
│   ├── disk.go                  # mingyue disk ...
│   ├── file.go                  # mingyue file ...
│   ├── smb.go                   # mingyue smb ...
│   ├── nfs.go                   # mingyue nfs ...
│   ├── share.go                 # mingyue share ...（已废弃，保留兼容）
│   ├── network.go               # mingyue network ...
│   └── acl.go                   # mingyue acl ...
├── domain/                      # 领域模型（纯数据结构）
├── service/                     # 核心业务逻辑（CLI 与 API 共用同源实现）
│   ├── system/                  # 系统监控
│   ├── process/                 # 进程管理
│   ├── disk/                    # 磁盘与挂载管理
│   ├── file/                    # 文件管理（含路径安全校验）
│   ├── share/                   # 共享管理（Samba/NFS）
│   ├── network/                 # 网络管理（接口查询与受控变更）
│   └── acl/                     # 权限/ACL 管理（getfacl/setfacl）
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
| `/var/lib/mingyue/` | 运行时状态（PID 文件、共享状态 `shares.json` 等） |
| `/var/log/mingyue/` | 运行日志与审计日志（`audit.log`） |
| `/etc/samba/smb.conf.d/mingyue.conf` | 自动生成的 Samba 配置片段（由共享管理服务维护） |
| `/etc/exports.d/mingyue.exports` | 自动生成的 NFS exports 片段（由共享管理服务维护） |

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
| 网络接口查询（只读） | 普通用户即可 |
| 网络接口变更（up/down/dhcp） | 需要 `CAP_NET_ADMIN` 或 root |
| 文件/目录权限查询（ACL get） | 普通用户即可（文件可读时） |
| POSIX ACL 设置（ACL set） | 需要文件所有者权限或 root，以及 `setfacl` 工具 |

权限不足时，系统返回可解释的错误信息并提示所需权限，不会静默失败。

> CIFS 凭据通过环境变量或 `/run` 下临时文件传递，**绝不出现在命令行参数、日志或 API 响应中**。

## 里程碑

| 阶段 | 内容 | 状态 |
|------|------|------|
| **Phase 1** ✅ | 基础骨架：CLI 框架、守护进程框架、HTTP API 基础、统一错误结构、认证鉴权草案、审计日志骨架、CI 基础 | 已完成 |
| **Phase 2** ✅ | 系统监控 + 进程管理：CPU/内存/磁盘概览、进程列表与终止、CLI/API 对齐、Bearer Token 认证 | 已完成 |
| **Phase 3** ✅ | 磁盘管理：本地挂载/卸载、CIFS/NFS 挂载/卸载、SMART 信息、幂等与审计 | 已完成 |
| **Phase 4** ✅ | 文件管理 + 共享管理：文件操作（路径安全+符号链接防护）、Samba/NFS 共享 CRUD（内存占位）、审计日志完善 | 已完成 |
| **Phase 4.x** ✅ | 共享管理持久化：JSON 状态文件落盘（`/var/lib/mingyue/shares.json`）、自动生成 Samba 配置片段（`/etc/samba/smb.conf.d/mingyue.conf`）、自动生成 NFS exports 片段（`/etc/exports.d/mingyue.exports`）、服务重载（`smbcontrol all reload-config` / `exportfs -ra`）、进程重启后状态自动恢复 | 已完成 |
| **Phase 5** ✅ | 网络管理 + 权限/ACL：网络接口只读查询与受控变更（up/down/dhcp）、文件/目录权限与 POSIX ACL 查询（getfacl）与设置（setfacl）、审计日志 | 已完成 |
| **Phase 6** | OpenAPI 规范 + CI 文档同步 + 安装脚本（`install.sh`）+ README | 待定 |

## License

MIT
