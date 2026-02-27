# CLI 命令契约（Phase 1–4）

本文件定义已实现的 CLI 命令的参数、输出字段与退出码约定，作为 CLI 行为的稳定契约。

---

## 全局标志

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--json` | bool | false | 将所有输出格式化为 JSON |
| `--config` | string | `/etc/mingyue/mingyue.yaml` | 配置文件路径 |
| `-h, --help` | bool | — | 显示帮助信息 |

所有子命令均继承以上全局标志。

---

## mingyue version

输出应用版本信息。

```sh
mingyue version [--json]
```

**人类可读输出示例**

```
mingyue version dev
```

**JSON 输出示例**

```json
{
  "version": "dev"
}
```

**退出码**：`0`（始终成功）

---

## mingyue agent

守护进程生命周期管理命令组。

### mingyue agent start

启动 HTTP 守护进程（前台运行，阻塞直至收到信号）。

```sh
mingyue agent start [--listen ADDR]
```

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--listen` | string | `:7070` | HTTP 监听地址 |

**人类可读输出示例**

```
Starting mingyue daemon on :7070 (pid file: /var/run/mingyue/mingyue.pid)
```

**JSON 输出示例**

```json
{
  "status": "starting",
  "address": ":7070",
  "pidFile": "/var/run/mingyue/mingyue.pid"
}
```

**退出码**：`0` 正常退出；`1` 启动失败（端口占用、PID 文件写入失败等）

### mingyue agent stop

向运行中的守护进程发送 SIGTERM。

```sh
mingyue agent stop [--json]
```

**人类可读输出**：`mingyue daemon stopped.`

**JSON 输出**：`{"status":"stopped"}`

**退出码**：`0` 成功；`1` 守护进程未运行或无法发送信号

### mingyue agent status

查询守护进程状态。

```sh
mingyue agent status [--json]
```

**人类可读输出示例**：`running (pid 12345)` 或 `stopped (no pid file)`

**JSON 输出示例**：`{"status":"running (pid 12345)"}`

**退出码**：`0`（始终成功，状态通过输出内容区分）

---

## mingyue system

系统资源监控命令组。

### mingyue system overview

输出当前主机的 CPU、内存与运行时间概览。

```sh
mingyue system overview [--json]
```

**人类可读输出示例**

```
Timestamp  : 2026-02-27T05:49:56Z
CPU        : 42.0%
Memory     : 4.0 GiB / 8.0 GiB (50.0%)
Uptime     : 1h 0m 0s
```

**JSON 输出示例**

```json
{
  "timestamp": "2026-02-27T05:49:56.324Z",
  "cpu_percent": 42.0,
  "mem_total": 8589934592,
  "mem_used": 4294967296,
  "mem_percent": 50.0,
  "uptime": 3600
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `timestamp` | RFC3339 string | 采集时刻（UTC） |
| `cpu_percent` | float64 | 全系统 CPU 使用率（0–100） |
| `mem_total` | uint64 | 总物理内存（字节） |
| `mem_used` | uint64 | 已用物理内存（字节） |
| `mem_percent` | float64 | 内存使用率（0–100） |
| `uptime` | uint64 | 系统运行时间（秒） |

**退出码**：`0` 成功；`1` 采集失败（系统接口不可用）

---

## mingyue process

进程管理命令组。

### mingyue process list

列出当前运行的进程，支持分页。

```sh
mingyue process list [--limit N] [--page N] [--json]
```

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--limit` | int | `0` | 最多返回 N 条（0 = 不限制） |
| `--page` | int | `1` | 页码（与 --limit 配合使用，1 起始） |

**人类可读输出示例**

```
PID      NAME                 STATUS       CPU%   MEM(RSS)  USER
1        systemd              sleep         0.4   13.8 MiB  root
2        kthreadd             sleep         0.0        0 B  root

(2 of 171 processes shown)
```

**JSON 输出示例**

```json
{
  "total": 171,
  "processes": [
    {
      "pid": 1,
      "name": "systemd",
      "status": "sleep",
      "cpu_percent": 0.4,
      "mem_rss": 14434304,
      "user": "root",
      "cmdline": "/sbin/init"
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `total` | int | 系统中进程总数 |
| `processes` | array | 当前页的进程列表 |
| `processes[].pid` | int32 | 进程 ID |
| `processes[].name` | string | 进程名称 |
| `processes[].status` | string | 进程状态（sleep/running/zombie 等） |
| `processes[].cpu_percent` | float64 | CPU 使用率（0–100） |
| `processes[].mem_rss` | uint64 | RSS 内存（字节） |
| `processes[].user` | string | 进程所属用户名 |
| `processes[].cmdline` | string | 完整命令行 |

**退出码**：`0` 成功；`1` 进程列表获取失败

### mingyue process get

查询指定 PID 的进程详情。

```sh
mingyue process get <pid> [--json]
```

**人类可读输出示例**

```
PID     : 1
Name    : systemd
Status  : sleep
CPU%    : 0.40
Mem RSS : 13.8 MiB
User    : root
Cmdline : /sbin/init
```

**JSON 输出**：单个 [Process 对象](#process-对象字段)

**退出码**：`0` 成功；`1` 进程不存在或访问失败

### mingyue process kill

向指定 PID 发送 SIGTERM 信号。

```sh
mingyue process kill <pid> [--json]
```

**人类可读输出**：`SIGTERM sent to process 1234`

**JSON 输出**：

```json
{
  "pid": 1234,
  "result": "signal sent"
}
```

**退出码**：`0` 信号发送成功；`1` 进程不存在（NOT_FOUND）或权限不足（FORBIDDEN）

> 无论成功或失败，`process.kill` 操作均产生一条审计日志（写入 `/var/log/mingyue/audit.log`）。

---

## 错误输出格式

所有命令在 `--json` 模式下的错误信息统一写入 **stderr**，格式如下：

```json
{
  "code": "FORBIDDEN",
  "message": "failed to kill process 1: operation not permitted"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `code` | string | 机器可解析的错误码（见下表） |
| `message` | string | 人类可读错误说明 |

**错误码一览**

| 错误码 | 含义 |
|--------|------|
| `NOT_FOUND` | 目标资源不存在（进程、文件、挂载点等） |
| `FORBIDDEN` | 权限不足（缺少所需系统能力或角色）、路径穿越/越界 |
| `UNAUTHORIZED` | 未提供有效认证令牌（API 模式）|
| `INVALID_INPUT` | 参数格式或值无效 |
| `INTERNAL` | 内部错误（系统调用失败） |

---

## Phase 4 — mingyue file

文件管理命令组。所有子命令均受 `--root` 标志约束，路径越界时返回 `FORBIDDEN` 错误。

### 公共标志（file 命令组持久化标志）

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--root` | string | `/` | 将所有文件操作限制在该目录子树内（防止路径穿越） |

> **安全建议**：在生产环境中始终设置 `--root` 为最小必要目录（如 `/var/lib/mingyue/data`），避免使用默认值 `/`。

---

### mingyue file list

列出目录内容。

```sh
mingyue file list <path> [--root DIR] [--json]
```

**人类可读输出示例**

```
TYPE     SIZE         MODE         NAME
file     200.0 KiB    -rw-r--r--   syslog
dir      0 B          drwxr-xr-x   nginx
```

**JSON 输出示例**

```json
{
  "path": "/var/log",
  "entries": [
    {
      "name": "syslog",
      "path": "/var/log/syslog",
      "is_dir": false,
      "size": 204800,
      "mode": "-rw-r--r--",
      "mod_time": "2026-02-27T06:00:00Z",
      "owner": "0"
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `path` | string | 请求的目录路径 |
| `entries` | array | 目录条目列表 |
| `entries[].name` | string | 文件基名 |
| `entries[].path` | string | 绝对路径 |
| `entries[].is_dir` | bool | 是否为目录 |
| `entries[].size` | int64 | 文件大小（字节） |
| `entries[].mode` | string | 权限字符串 |
| `entries[].mod_time` | RFC3339 string | 最后修改时间（UTC） |
| `entries[].owner` | string | 属主 UID |
| `entries[].unreadable` | bool | 若为 `true`，该条目不可读 |

**退出码**：`0` 成功；`1` 目录不存在或路径越界

---

### mingyue file stat

查看文件或目录元信息。

```sh
mingyue file stat <path> [--root DIR] [--json]
```

**人类可读输出示例**

```
Path    : /var/log/syslog
Type    : file
Size    : 200.0 KiB
Mode    : -rw-r--r--
ModTime : 2026-02-27T06:00:00Z
Owner   : 0
```

**JSON 输出**：单个 FileEntry 对象（字段同 `file list` entries 条目）

**退出码**：`0` 成功；`1` 路径不存在或越界

---

### mingyue file read

打印文件内容（原始字节到标准输出；`--json` 模式下内容为字符串）。

```sh
mingyue file read <path> [--root DIR] [--json]
```

**人类可读输出**：文件内容直接写入 stdout

**JSON 输出示例**

```json
{
  "path": "/var/log/syslog",
  "content": "Feb 27 06:00:00 host kernel: ..."
}
```

**退出码**：`0` 成功；`1` 文件不存在或越界

---

### mingyue file mkdir

创建目录（含所有不存在的父级目录）。

```sh
mingyue file mkdir <path> [--root DIR] [--json]
```

**人类可读输出**：`Directory created: /tmp/mydir`

**JSON 输出示例**

```json
{ "path": "/tmp/mydir", "result": "created" }
```

**退出码**：`0` 成功；`1` 失败

---

### mingyue file rm

删除文件或目录。

```sh
mingyue file rm <path> [-r] [--root DIR] [--json]
```

| 标志 | 说明 |
|------|------|
| `-r, --recursive` | 递归删除目录及其所有内容 |

**人类可读输出**：`Removed: /tmp/mydir`

**JSON 输出示例**

```json
{ "path": "/tmp/mydir", "result": "removed" }
```

**退出码**：`0` 成功；`1` 路径不存在或越界

---

### mingyue file mv

移动（重命名）文件或目录。

```sh
mingyue file mv <src> <dst> [--root DIR] [--json]
```

**人类可读输出**：`Moved: /tmp/old.txt → /tmp/new.txt`

**JSON 输出示例**

```json
{ "src": "/tmp/old.txt", "dst": "/tmp/new.txt", "result": "moved" }
```

**退出码**：`0` 成功；`1` 源不存在或越界

---

### mingyue file cp

复制文件。

```sh
mingyue file cp <src> <dst> [--root DIR] [--json]
```

**人类可读输出**：`Copied: /tmp/a.txt → /tmp/b.txt`

**JSON 输出示例**

```json
{ "src": "/tmp/a.txt", "dst": "/tmp/b.txt", "result": "copied" }
```

**退出码**：`0` 成功；`1` 源不存在或越界

---

### mingyue file write

将内容写入文件（不存在则创建，已存在则覆盖）。

```sh
mingyue file write <path> --content <text> [--root DIR] [--json]
```

| 标志 | 说明 |
|------|------|
| `--content` | 要写入的文本内容 |

**人类可读输出**：`Written: /tmp/hello.txt`

**JSON 输出示例**

```json
{ "path": "/tmp/hello.txt", "result": "written" }
```

**退出码**：`0` 成功；`1` 失败（权限不足、路径越界等）

---

## Phase 4 — mingyue share

网络共享管理命令组。

> **注意**：当前共享后端为内存存储（placeholder）。`create`/`update`/`delete` 的变更**不会持久化到磁盘**，进程重启后将丢失。真实 Samba/NFS 配置文件支持为后续版本计划。

---

### mingyue share list

列出所有已配置的网络共享。

```sh
mingyue share list [--json]
```

**人类可读输出示例**

```
NAME                 TYPE   ENABLED   PATH
myshare              smb    yes       /srv/myshare
nfsdata              nfs    yes       /data/nfs
```

**JSON 输出示例**

```json
{
  "shares": [
    {
      "name": "myshare",
      "type": "smb",
      "path": "/srv/myshare",
      "comment": "",
      "read_only": false,
      "enabled": true
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `shares` | array | 共享列表（可为空数组） |
| `shares[].name` | string | 共享名称 |
| `shares[].type` | string | `"smb"` 或 `"nfs"` |
| `shares[].path` | string | 本地共享目录 |
| `shares[].comment` | string | 描述 |
| `shares[].read_only` | bool | 是否只读 |
| `shares[].enabled` | bool | 是否启用 |

**退出码**：`0` 成功；`1` 失败

---

### mingyue share get

查看指定共享的详情。

```sh
mingyue share get <name> [--json]
```

**人类可读输出示例**

```
Name     : myshare
Type     : smb
Path     : /srv/myshare
Comment  :
ReadOnly : false
Enabled  : true
```

**JSON 输出**：单个 Share 对象（字段同 `share list` shares[] 条目）

**退出码**：`0` 成功；`1` 共享不存在

---

### mingyue share create

创建新的网络共享。

```sh
mingyue share create <name> --path <dir> [--type smb|nfs] [--comment TEXT]
                            [--read-only] [--enabled] [--json]
```

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--path` | string | — | 本地共享目录（**必填**） |
| `--type` | string | `smb` | 共享类型：`smb` 或 `nfs` |
| `--comment` | string | `""` | 可选描述 |
| `--read-only` | bool | `false` | 是否只读导出 |
| `--enabled` | bool | `true` | 是否立即启用 |

**人类可读输出**：`Share created: myshare`

**JSON 输出示例**

```json
{ "name": "myshare", "result": "created" }
```

**退出码**：`0` 成功；`1` 校验失败（名称含 `/`、路径为空、类型不支持）或服务重载失败

---

### mingyue share update

更新已有共享。

```sh
mingyue share update <name> --path <dir> [--type smb|nfs] [--comment TEXT]
                            [--read-only] [--enabled] [--json]
```

标志同 `share create`。`--path` 为必填。

**人类可读输出**：`Share updated: myshare`

**JSON 输出示例**

```json
{ "name": "myshare", "result": "updated" }
```

**退出码**：`0` 成功；`1` 共享不存在、校验失败或重载失败

---

### mingyue share delete

删除指定共享。

```sh
mingyue share delete <name> [--json]
```

**人类可读输出**：`Share deleted: myshare`

**JSON 输出示例**

```json
{ "name": "myshare", "result": "deleted" }
```

**退出码**：`0` 成功；`1` 共享不存在或重载失败
