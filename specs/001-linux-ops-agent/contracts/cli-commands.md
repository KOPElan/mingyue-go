# CLI 命令契约（Phase 1–7）

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
mingyue agent start [--listen ADDR] [--keystore PATH]
```

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--listen` | string | `:7070` | HTTP 监听地址 |
| `--keystore` | string | `/var/lib/mingyue/apikeys.json` | API 密钥文件路径 |

启动时行为：
1. 从 `--keystore` 文件加载所有已持久化的 API 密钥
2. 若密钥文件为空（首次运行），自动生成一个 admin 角色密钥，保存到文件并打印到 stdout
3. 启动 UDP 多播公告（`224.0.0.251:7071`），让局域网内的 web 前端能自动发现此 agent

**人类可读输出示例（首次运行）**

```
Starting mingyue daemon on :7070 (pid file: /var/run/mingyue/mingyue.pid)

*** Initial admin API key (save this) ***
a3f1...9c2e (64-char hex)
```

**人类可读输出示例（已有密钥）**

```
Starting mingyue daemon on :7070 (pid file: /var/run/mingyue/mingyue.pid)
```

**JSON 输出示例（首次运行）**

```json
{
  "status": "starting",
  "address": ":7070",
  "pidFile": "/var/run/mingyue/mingyue.pid",
  "initialKey": "a3f1...9c2e",
  "initialRole": "admin"
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

### mingyue agent discover

扫描局域网内所有正在运行的 mingyue agent。

```sh
mingyue agent discover [--timeout DURATION] [--json]
```

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--timeout` | duration | `3s` | 等待响应的最长时间 |

Agent 启动后每 3 秒向多播组 `224.0.0.251:7071` 广播自身信息。此命令收集所有响应并输出列表。

**人类可读输出示例**

```
Scanning for mingyue agents (3s)...
HOSTNAME                        ADDRESS               VERSION
------------------------------  --------------------  -------
myserver                        :7070                 dev
```

**JSON 输出示例**

```json
[
  {
    "hostname": "myserver",
    "addr": ":7070",
    "version": "dev"
  }
]
```

**退出码**：`0`（始终成功；空列表不视为错误）

---

## mingyue auth

API 密钥管理命令组。密钥保存在 `/var/lib/mingyue/apikeys.json`（权限 0600）。

### mingyue auth keygen

生成并保存一个新的 API 密钥，打印到 stdout。

```sh
mingyue auth keygen [--role ROLE] [--subject LABEL] [--keystore PATH] [--json]
```

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--role` | string | `viewer` | 角色：`viewer`、`operator` 或 `admin` |
| `--subject` | string | `default` | 标识密钥用途的标签（如 `web-frontend`） |
| `--keystore` | string | `/var/lib/mingyue/apikeys.json` | 密钥文件路径 |

**人类可读输出示例**

```
API key generated successfully:
  Key:     a3f1...9c2e (64-char hex，完整显示)
  Role:    admin
  Subject: web-frontend

Keep this key secret — it grants admin access to the agent.
```

**JSON 输出示例**

```json
{
  "key": "a3f1c2...9c2e",
  "role": "admin",
  "subject": "web-frontend",
  "created_at": "2026-01-01T00:00:00Z"
}
```

**退出码**：`0` 成功；`1` 无效角色或文件写入失败

### mingyue auth list

列出所有已保存的 API 密钥（密钥值部分掩码）。

```sh
mingyue auth list [--keystore PATH] [--json]
```

**人类可读输出示例**

```
KEY (masked)        ROLE        SUBJECT                   CREATED (UTC)
------------------  ----------  ------------------------  -------------------
a3f1...9c2e         admin       web-frontend              2026-01-01 00:00:00
```

**JSON 输出示例**

```json
[
  {
    "key": "a3f1...9c2e",
    "role": "admin",
    "subject": "web-frontend",
    "created_at": "2026-01-01T00:00:00Z"
  }
]
```

**退出码**：`0`（始终成功；空列表不视为错误）

### mingyue auth revoke

撤销一个 API 密钥（从文件和内存中同时删除）。

```sh
mingyue auth revoke <key> [--keystore PATH] [--json]
```

**人类可读输出示例**：`Revoked API key: a3f1...9c2e`

**JSON 输出示例**：`{"status":"revoked","key":"a3f1...9c2e"}`

**退出码**：`0` 成功；`1` 密钥不存在或文件操作失败

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

## mingyue disk

磁盘与挂载管理命令组。

### mingyue disk list

列出当前所有挂载点（读取 `/proc/mounts`）。

```sh
mingyue disk list [--json]
```

**人类可读输出示例**

```
MOUNT POINT                    DEVICE                         FS TYPE     OPTIONS
/                              /dev/sda1                      ext4        rw,relatime
/mnt/data                      /dev/sdb1                      ext4        rw,relatime

(2 mount(s) found)
```

**JSON 输出示例**

```json
{
  "mounts": [
    {
      "device": "/dev/sda1",
      "mount_point": "/",
      "fs_type": "ext4",
      "options": "rw,relatime",
      "total": 0,
      "used": 0,
      "free": 0
    }
  ]
}
```

**退出码**：`0` 成功；`1` 读取挂载表失败

---

### mingyue disk mount

挂载文件系统。

```sh
mingyue disk mount --type <fstype> [--read-only] [--options <opts>] \
  [--username <u>] [--password <p>] [--domain <d>] \
  <source> <mountpoint> [--json]
```

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--type` | string | `""` | 文件系统类型（`ext4`/`cifs`/`nfs`/`auto`） |
| `--read-only` | bool | false | 只读挂载 |
| `--options` | string | `""` | 附加挂载选项（逗号分隔，不含凭据） |
| `--username` | string | `""` | CIFS 用户名（不记录到日志） |
| `--password` | string | `""` | CIFS 密码（不记录到日志） |
| `--domain` | string | `""` | CIFS 域（可选） |

**人类可读输出**：`Mounted /dev/sdb1 at /mnt/data`

**JSON 输出**：

```json
{
  "source": "/dev/sdb1",
  "mount_point": "/mnt/data",
  "result": "mounted"
}
```

**退出码**：`0` 成功；`1` 已挂载（CONFLICT）或挂载失败

> `disk.mount` 操作均产生一条审计日志。

---

### mingyue disk umount

卸载指定挂载点。

```sh
mingyue disk umount <mountpoint> [--json]
```

**人类可读输出**：`Unmounted /mnt/data`

**JSON 输出**：

```json
{
  "mount_point": "/mnt/data",
  "result": "unmounted"
}
```

**退出码**：`0` 成功；`1` 挂载点未挂载（NOT_FOUND）或卸载失败

> `disk.umount` 操作均产生一条审计日志。

---

### mingyue disk smart

查询块设备 SMART 健康信息（需 root 或 CAP_SYS_RAWIO，依赖 `smartmontools`）。

```sh
mingyue disk smart <device> [--json]
```

| 参数 | 说明 |
|------|------|
| `device` | 设备路径，如 `/dev/sda` 或简写 `sda` |

**人类可读输出示例**

```
Device        : /dev/sda
Model         : Samsung SSD 860 EVO 250GB
Serial        : S3EVNX0K123456
Health        : PASSED
Temperature   : 26°C
Power-On Hours: 8765 h
```

**JSON 输出示例**

```json
{
  "device": "/dev/sda",
  "model": "Samsung SSD 860 EVO 250GB",
  "serial": "S3EVNX0K123456",
  "health_ok": true,
  "temperature_c": 26,
  "power_on_hours": 8765
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `device` | string | 设备路径 |
| `model` | string | 硬盘型号 |
| `serial` | string | 序列号 |
| `health_ok` | bool | SMART 自检是否通过 |
| `temperature_c` | int | 当前温度（摄氏度） |
| `power_on_hours` | uint64 | 累计通电时间（小时） |

**退出码**：`0` 成功；`1` smartctl 未安装（NOT_FOUND）、权限不足（FORBIDDEN）或查询失败

---

### mingyue disk devices

列出系统上所有块设备（含未挂载设备，依赖 `lsblk`）。

```sh
mingyue disk devices [--json]
```

**人类可读输出示例**

```
NAME         TYPE     SIZE         MOUNT POINT                    MODEL                     RM
sda          disk     500.1 GiB                                   Samsung SSD 860 EVO 500GB false
sda1         part     512.0 MiB    /boot                                                    false
sda2         part     499.6 GiB    /                                                        false
sdb          disk     2.0 TiB                                     WD Blue 2TB               false
```

**JSON 输出示例**

```json
{
  "devices": [
    {
      "name": "sda",
      "size_bytes": 500107862016,
      "type": "disk",
      "mount_point": "",
      "model": "Samsung SSD 860 EVO 500GB",
      "removable": false
    },
    {
      "name": "sda1",
      "size_bytes": 536870912,
      "type": "part",
      "mount_point": "/boot",
      "removable": false
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 设备短名称（如 `sda`、`sda1`） |
| `size_bytes` | uint64 | 设备大小（字节，0 表示未知） |
| `type` | string | 设备类型：`disk`、`part`、`rom`、`loop` 等 |
| `mount_point` | string | 当前挂载点（未挂载则为空） |
| `model` | string | 设备型号（分区或虚拟设备可能为空） |
| `removable` | bool | 是否为可移动设备（如 USB） |

**退出码**：`0` 成功；`1` lsblk 未安装或查询失败

---

### mingyue disk power

查询或设置块设备的电源/睡眠状态（依赖 `hdparm`，需要 root 或 `CAP_SYS_RAWIO`）。

```sh
mingyue disk power <device> [--standby | --sleep] [--json]
```

| 参数/标志 | 说明 |
|----------|------|
| `device` | 设备路径（如 `/dev/sda`）或简写（如 `sda`） |
| `--standby` | 将磁盘置于待机模式（停转）|
| `--sleep` | 将磁盘强制进入睡眠模式 |

`--standby` 与 `--sleep` 互斥；不指定任何标志则仅查询当前电源模式。

**人类可读输出示例（查询）**

```
Device    : /dev/sda
Power Mode: active
```

**JSON 输出示例（查询）**

```json
{
  "device": "/dev/sda",
  "power_mode": "active"
}
```

**JSON 输出示例（设置）**

```json
{
  "device": "/dev/sda",
  "action": "standby",
  "result": "ok"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `device` | string | 设备路径 |
| `power_mode` | string | 当前电源模式：`active`、`standby`、`sleeping`、`unknown` |

**退出码**：`0` 成功；`1` hdparm 未安装（NOT_FOUND）、权限不足（FORBIDDEN）或操作失败

> `disk.power set` 操作均产生一条审计日志。

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
| `NOT_FOUND` | 目标资源不存在（进程、文件、挂载点、smartctl 等） |
| `FORBIDDEN` | 权限不足（缺少所需系统能力或角色）、路径穿越/越界 |
| `UNAUTHORIZED` | 未提供有效认证令牌（API 模式）|
| `INVALID_INPUT` | 参数格式或值无效 |
| `CONFLICT` | 目标已存在或状态冲突（如重复挂载）|
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

## Phase 4 — mingyue share *(已弃用)*

> **已弃用**：请改用协议专属命令 `mingyue smb` 或 `mingyue nfs`。
> 运行任何 `share` 子命令时，系统会自动打印弃用提示。

共享配置持久化至 `/var/lib/mingyue/shares.json`，进程重启后自动恢复。

子命令（`list`、`get`、`create`、`update`、`delete`）语义与下文 `smb`/`nfs` 对应命令相同，但 `create`/`update` 须通过 `--type smb|nfs` 明确指定协议类型。

---

## Phase 4 — mingyue smb

Samba (SMB/CIFS) 共享管理与用户管理命令组。

> 共享配置持久化至 `/var/lib/mingyue/shares.json`。
> `create`/`update`/`delete` 操作会自动重新生成 `/etc/samba/smb.conf.d/mingyue.conf` 并触发 `smbcontrol all reload-config`。
> 一次性前置操作：在 `/etc/samba/smb.conf` 中添加 `include = /etc/samba/smb.conf.d/mingyue.conf`。

---

### mingyue smb list

列出所有 Samba 共享。

```sh
mingyue smb list [--json]
```

**人类可读输出示例**

```
NAME                 ENABLED   PATH
myshare              yes       /srv/myshare
data                 yes       /data
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
      "enabled": true,
      "valid_users": "alice @staff",
      "write_list": "alice",
      "create_mask": "0644",
      "directory_mask": "0755"
    }
  ]
}
```

**退出码**：`0` 成功；`1` 失败

---

### mingyue smb get

查看指定 Samba 共享的详情。

```sh
mingyue smb get <name> [--json]
```

**人类可读输出示例**

```
Name          : myshare
Path          : /srv/myshare
Comment       :
ReadOnly      : false
Enabled       : true
ValidUsers    : alice @staff
WriteList     : alice
CreateMask    : 0644
DirectoryMask : 0755
```

**JSON 输出**：单个 Share 对象（字段同 `smb list` shares[] 条目）

**退出码**：`0` 成功；`1` 共享不存在

---

### mingyue smb create

创建新的 Samba 共享。

```sh
mingyue smb create <name> --path <dir> [--comment TEXT]
                          [--read-only] [--enabled]
                          [--valid-users USERS] [--write-list USERS]
                          [--create-mask MASK] [--dir-mask MASK]
                          [--json]
```

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--path` | string | — | 本地共享目录（**必填**） |
| `--comment` | string | `""` | 可选描述 |
| `--read-only` | bool | `false` | 是否只读导出 |
| `--enabled` | bool | `true` | 是否立即启用 |
| `--valid-users` | string | `""` | 允许连接的用户或 `@组`（空表示所有认证用户） |
| `--write-list` | string | `""` | 拥有写权限的用户或 `@组` |
| `--create-mask` | string | `""` | 新建文件权限掩码（如 `0644`） |
| `--dir-mask` | string | `""` | 新建目录权限掩码（如 `0755`） |

**人类可读输出**：`Samba share created: myshare`

**JSON 输出示例**

```json
{ "name": "myshare", "result": "created" }
```

**退出码**：`0` 成功；`1` 校验失败、名称已存在（`CONFLICT`）或服务重载失败

---

### mingyue smb update

更新已有 Samba 共享。

```sh
mingyue smb update <name> --path <dir> [标志同 smb create]
```

所有字段均以提供值替换。`--path` 为必填。

**人类可读输出**：`Samba share updated: myshare`

**JSON 输出示例**

```json
{ "name": "myshare", "result": "updated" }
```

**退出码**：`0` 成功；`1` 共享不存在、校验失败或重载失败

---

### mingyue smb delete

删除指定 Samba 共享。

```sh
mingyue smb delete <name> [--json]
```

**人类可读输出**：`Samba share deleted: myshare`

**JSON 输出示例**

```json
{ "name": "myshare", "result": "deleted" }
```

**退出码**：`0` 成功；`1` 共享不存在或重载失败

---

### mingyue smb user list

列出所有 Samba 用户（来自 Samba 密码数据库 tdbsam）。

```sh
mingyue smb user list [--json]
```

**人类可读输出示例**

```
alice
bob
```

**JSON 输出示例**

```json
{ "users": [{ "username": "alice" }, { "username": "bob" }] }
```

**退出码**：`0` 成功；`1` 失败（pdbedit 不可用等）

---

### mingyue smb user add

将 Linux 用户添加到 Samba 数据库并设置初始密码（从 stdin 读取）。

```sh
echo "password" | mingyue smb user add <username> [--json]
```

Linux 系统账号必须已存在才能添加为 Samba 用户。

**人类可读输出**：`Samba user added: alice`

**JSON 输出示例**

```json
{ "username": "alice", "result": "added" }
```

**退出码**：`0` 成功；`1` 用户不存在、密码未提供或命令失败

---

### mingyue smb user remove

从 Samba 数据库删除用户。

```sh
mingyue smb user remove <username> [--json]
```

**人类可读输出**：`Samba user removed: alice`

**JSON 输出示例**

```json
{ "username": "alice", "result": "removed" }
```

**退出码**：`0` 成功；`1` 用户不存在或命令失败

---

### mingyue smb user passwd

修改 Samba 用户密码（从 stdin 读取）。

```sh
echo "newpassword" | mingyue smb user passwd <username> [--json]
```

**人类可读输出**：`Samba password updated for: alice`

**JSON 输出示例**

```json
{ "username": "alice", "result": "password updated" }
```

**退出码**：`0` 成功；`1` 用户不存在、密码未提供或命令失败

---

## Phase 4 — mingyue nfs

NFS (Network File System) 导出管理命令组。

> 导出配置持久化至 `/var/lib/mingyue/shares.json`。
> `create`/`update`/`delete` 操作会自动重新生成 `/etc/exports.d/mingyue.exports` 并触发 `exportfs -ra`。
> 前置条件：确保 `/etc/exports.d/` 被 `/etc/exports` 包含（多数发行版默认已配置）。

---

### mingyue nfs list

列出所有 NFS 导出。

```sh
mingyue nfs list [--json]
```

**人类可读输出示例**

```
NAME                 ENABLED   HOSTS             PATH
nfsdata              yes       *                 /data/nfs
restricted           yes       192.168.1.0/24    /srv/data
```

**JSON 输出示例**

```json
{
  "exports": [
    {
      "name": "nfsdata",
      "type": "nfs",
      "path": "/data/nfs",
      "comment": "",
      "read_only": false,
      "enabled": true,
      "hosts": ""
    }
  ]
}
```

**退出码**：`0` 成功；`1` 失败

---

### mingyue nfs get

查看指定 NFS 导出的详情。

```sh
mingyue nfs get <name> [--json]
```

**人类可读输出示例**

```
Name     : nfsdata
Path     : /data/nfs
Comment  :
ReadOnly : false
Enabled  : true
Hosts    : *
```

**JSON 输出**：单个 Share 对象（含 `hosts` 字段）

**退出码**：`0` 成功；`1` 导出不存在

---

### mingyue nfs create

创建新的 NFS 导出。

```sh
mingyue nfs create <name> --path <dir> [--comment TEXT]
                          [--read-only] [--enabled]
                          [--hosts "HOST1 HOST2"]
                          [--json]
```

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--path` | string | — | 本地导出目录（**必填**） |
| `--comment` | string | `""` | 可选描述 |
| `--read-only` | bool | `false` | 是否只读导出 |
| `--enabled` | bool | `true` | 是否立即启用 |
| `--hosts` | string | `""` | 空格分隔的主机/CIDR（空表示 `*` 全部允许） |

**人类可读输出**：`NFS export created: nfsdata`

**JSON 输出示例**

```json
{ "name": "nfsdata", "result": "created" }
```

**退出码**：`0` 成功；`1` 校验失败、名称已存在（`CONFLICT`）或服务重载失败

---

### mingyue nfs update

更新已有 NFS 导出。

```sh
mingyue nfs update <name> --path <dir> [标志同 nfs create]
```

所有字段均以提供值替换。`--path` 为必填。

**人类可读输出**：`NFS export updated: nfsdata`

**JSON 输出示例**

```json
{ "name": "nfsdata", "result": "updated" }
```

**退出码**：`0` 成功；`1` 导出不存在、校验失败或重载失败

---

### mingyue nfs delete

删除指定 NFS 导出。

```sh
mingyue nfs delete <name> [--json]
```

**人类可读输出**：`NFS export deleted: nfsdata`

**JSON 输出示例**

```json
{ "name": "nfsdata", "result": "deleted" }
```

**退出码**：`0` 成功；`1` 导出不存在或重载失败

---

## Phase 5：网络管理 + 权限/ACL

### mingyue network

网络接口管理命令组。

#### mingyue network list

列出所有网络接口。

```sh
mingyue network list [--json]
```

**人类可读输出示例**

```
NAME                 INDEX    FLAGS                 ADDRESSES
lo                   1        LOOPBACK,UP           127.0.0.1/8
eth0                 2        UP,BROADCAST,MULTICAST 192.168.1.10/24, fe80::1/64
```

**JSON 输出示例**

```json
{
  "interfaces": [
    {
      "name": "lo",
      "index": 1,
      "flags": ["UP", "LOOPBACK"],
      "mtu": 65536,
      "addresses": [{ "ip": "127.0.0.1", "prefix": 8, "family": "ipv4" }]
    }
  ]
}
```

**退出码**：`0` 成功；`1` 系统调用失败

---

#### mingyue network get \<name\>

查询单个网络接口详情。

```sh
mingyue network get <name> [--json]
```

**人类可读输出示例**

```
Name    : eth0
Index   : 2
MTU     : 1500
HWAddr  : 52:54:00:ab:cd:ef
Flags   : UP, BROADCAST, MULTICAST
Addresses:
  192.168.1.10/24 (ipv4)
  fe80::1/64 (ipv6)
```

**JSON 输出**：单个接口对象（同 `list` 中 `interfaces[]` 条目结构）

**退出码**：`0` 成功；`1` 接口不存在或系统调用失败

---

#### mingyue network up \<name\>

启用网络接口（需要 root 或 `CAP_NET_ADMIN`）。

```sh
mingyue network up <name> [--json]
```

**人类可读输出**：`Interface eth0 is now up`

**JSON 输出**：`{"interface":"eth0","result":"up"}`

**退出码**：`0` 成功；`1` 权限不足或命令执行失败

---

#### mingyue network down \<name\>

禁用网络接口（需要 root 或 `CAP_NET_ADMIN`）。

```sh
mingyue network down <name> [--json]
```

**人类可读输出**：`Interface eth0 is now down`

**JSON 输出**：`{"interface":"eth0","result":"down"}`

**退出码**：`0` 成功；`1` 权限不足或命令执行失败

---

#### mingyue network dhcp \<name\>

刷新接口的 DHCP 租约（优先尝试 `dhclient`，备选 `dhcpcd`）。
需要 root 或 `CAP_NET_ADMIN`。

```sh
mingyue network dhcp <name> [--json]
```

**人类可读输出**：`DHCP lease renewed on eth0`

**JSON 输出**：`{"interface":"eth0","result":"dhcp-renewed"}`

**退出码**：`0` 成功；`1` 两种 DHCP 客户端均失败或权限不足

---

### mingyue acl

文件权限与 POSIX ACL 管理命令组。

所有操作均受 `--root` 根目录约束，防止路径穿越。

| 标志 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--root` | string | `/` | 约束所有 ACL 操作的根目录 |

#### mingyue acl get \<path\>

查询文件或目录的权限与 POSIX ACL 条目。

```sh
mingyue acl get <path> [--root DIR] [--json]
```

**人类可读输出示例**

```
Path  : /srv/data
Owner : 1000
Group : 1000
Mode  : drwxr-xr-x
ACL Entries:
  user::rwx
  group::r-x
  user:alice:rwx
  mask::rwx
  other::r--
```

**JSON 输出**：`ACLInfo` 对象（含 `path`、`owner`、`group`、`mode`、`acl_entries`）

**退出码**：`0` 成功；`1` 路径不存在、路径越权或系统调用失败

---

#### mingyue acl set \<path\>

设置文件或目录的 POSIX ACL 条目（需要安装 `setfacl`，以及写权限）。

```sh
mingyue acl set <path> --entry <spec> [--entry <spec> ...] [--root DIR] [--json]
```

| 标志 | 类型 | 说明 |
|------|------|------|
| `--entry` | string（可重复） | ACL 规格字符串，格式 `type:qualifier:perms`（如 `u:alice:rwx`） |

**人类可读输出**：`ACL updated: /srv/data`

**JSON 输出示例**

```json
{ "path": "/srv/data", "entries": ["u:alice:rwx", "g:devs:r-x"], "result": "set" }
```

**退出码**：`0` 成功；`1` 路径越权、`setfacl` 未安装、权限不足或命令执行失败

---

## Phase 6：OpenAPI 规范 + 安装脚本

Phase 6 不新增 CLI 子命令，其产出为 API 文档规范与系统服务安装能力。

### OpenAPI 规范

项目提供完整的 OpenAPI v3 规范文件，覆盖 v1 全量端点（含认证方式、请求/响应结构与错误结构）：

```
docs/api/openapi.yaml    # OpenAPI v3 规范（YAML 格式）
```

可使用任意 OpenAPI 工具加载该文件，例如：

```sh
# 使用 Swagger UI（Docker）本地预览
docker run -p 8080:8080 -e SWAGGER_JSON=/api/openapi.yaml \
  -v $(pwd)/docs/api:/api swaggerapi/swagger-ui

# 使用 redoc-cli 本地预览
npx @redocly/cli preview-docs docs/api/openapi.yaml

# 使用 openapi-generator 生成客户端代码
openapi-generator-cli generate -i docs/api/openapi.yaml \
  -g python -o ./client-python
```

---

### 安装为系统服务（systemd）

使用提供的脚本将 `mingyue` 守护进程注册为 systemd 服务，实现开机自启。

#### 安装

```sh
sudo bash scripts/install.sh
```

安装脚本执行步骤：
1. 检测 systemd 环境（不满足则退出并提示）
2. 将编译好的 `mingyue` 二进制复制到 `/usr/local/bin/mingyue`
3. 创建运行时目录（`/etc/mingyue`、`/var/lib/mingyue`、`/var/log/mingyue`）
4. 写入 `/etc/systemd/system/mingyue.service` unit 文件
5. 执行 `systemctl daemon-reload && systemctl enable --now mingyue`

**验证安装**

```sh
systemctl status mingyue
curl http://localhost:7070/api/v1/health
```

#### 卸载

```sh
sudo bash scripts/uninstall.sh          # 卸载服务，保留数据目录
sudo bash scripts/uninstall.sh --purge  # 卸载服务并删除所有数据目录
```

`--purge` 选项额外删除：`/etc/mingyue`、`/var/lib/mingyue`、`/var/log/mingyue`。

**退出码**：`0` 成功；`1` 非 root 运行、systemd 不可用或命令执行失败
