# CLI 命令契约（Phase 1–3）

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
| `FORBIDDEN` | 权限不足（缺少所需系统能力或角色） |
| `UNAUTHORIZED` | 未提供有效认证令牌（API 模式）|
| `INVALID_INPUT` | 参数格式或值无效 |
| `CONFLICT` | 目标已存在或状态冲突（如重复挂载）|
| `INTERNAL` | 内部错误（系统调用失败） |
