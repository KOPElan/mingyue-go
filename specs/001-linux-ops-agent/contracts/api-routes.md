# HTTP API 路由契约（Phase 1–7）

本文件定义已实现的 HTTP API 端点的请求/响应结构与错误约定，作为 API 行为的稳定契约。

**基础 URL**：`http://<host>:7070`  
**路径前缀**：`/api/v1`  
**内容类型**：`application/json`

---

## 认证

除健康检查与版本查询外，所有端点均需在请求头中携带 Bearer Token：

```
Authorization: Bearer <api-key>
```

**角色层级**（由低到高）：`viewer` → `operator` → `admin`

- `viewer`：只读操作
- `operator`：只读、进程终止、文件写/删/移/复制、共享创建/更新/删除
- `admin`：全部操作

API Key 通过 `auth.RegisterAPIKey()` 在进程启动时注册。自 Phase 7 起，密钥使用文件持久化存储：

- **密钥文件**：`/var/lib/mingyue/apikeys.json`（权限 0600，仅 owner 可读）
- **首次启动**：若密钥文件为空，agent 自动生成一个 admin 密钥并打印到 stdout，请妥善保存
- **密钥管理**：使用 `mingyue auth keygen/list/revoke` CLI 命令管理密钥（详见 CLI 契约）
- **重启持久性**：密钥写入文件后重启 agent 会自动加载，无需重新配置

---

## 统一错误响应结构

所有错误响应均使用以下 JSON 格式：

```json
{
  "code": "NOT_FOUND",
  "message": "process 9999 not found"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `code` | string | 机器可解析的错误码 |
| `message` | string | 人类可读错误说明 |

**错误码与 HTTP 状态码映射**

| 错误码 | HTTP 状态码 | 含义 |
|--------|-------------|------|
| `NOT_FOUND` | 404 | 目标资源不存在 |
| `UNAUTHORIZED` | 401 | 缺少或无效的认证令牌 |
| `FORBIDDEN` | 403 | 权限不足 |
| `INVALID_INPUT` | 400 | 请求参数格式或值无效 |
| `INTERNAL` | 500 | 内部服务器错误 |

---

## 端点列表

### GET /api/v1/health

健康检查。**无需认证**，供负载均衡器与监控系统使用。

**请求**：无参数

**响应 200**

```json
{
  "status": "ok",
  "version": "dev",
  "go_os": "linux",
  "go_arch": "amd64"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `status` | string | 固定为 `"ok"` |
| `version` | string | 应用版本 |
| `go_os` | string | 运行时操作系统 |
| `go_arch` | string | 运行时架构 |

---

### GET /api/v1/version

查询应用版本。**无需认证**。

**响应 200**

```json
{
  "version": "dev"
}
```

---

### GET /api/v1/system/overview

获取当前主机资源概览（CPU、内存、运行时间）。**需要 viewer 或以上角色**。

**请求**：无参数

**响应 200**

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

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 500 | `INTERNAL` | gopsutil 系统调用失败 |

---

### GET /api/v1/processes

获取运行中的进程列表，支持分页。**需要 viewer 或以上角色**。

**查询参数**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `limit` | int | 0（不限制） | 每页最大条数（0 = 返回全部） |
| `page` | int | 1 | 页码（1 起始，需配合 limit 使用） |

**响应 200**

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
| `total` | int | 系统中进程总数（分页前） |
| `processes` | array | 当前页进程列表 |
| `processes[].pid` | int32 | 进程 ID |
| `processes[].name` | string | 进程名称 |
| `processes[].status` | string | 进程状态（sleep/running/zombie 等） |
| `processes[].cpu_percent` | float64 | CPU 使用率（0–100） |
| `processes[].mem_rss` | uint64 | RSS 内存（字节） |
| `processes[].user` | string | 进程所属用户名 |
| `processes[].cmdline` | string | 完整命令行 |

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | limit 或 page 参数格式无效 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 500 | `INTERNAL` | 进程枚举失败 |

---

### GET /api/v1/processes/{pid}

查询指定 PID 的进程详情。**需要 viewer 或以上角色**。

**路径参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `pid` | int32 | 目标进程 ID |

**响应 200**：单个 Process 对象（字段同上文 `processes[]` 条目）

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | pid 路径段不是合法整数 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 404 | `NOT_FOUND` | 指定 PID 的进程不存在 |

---

### DELETE /api/v1/processes/{pid}

向指定 PID 发送 SIGTERM 信号。**需要 operator 或 admin 角色**。

> 此操作无论成功或失败，均产生一条审计日志（`action: "process.kill"`）。

**路径参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `pid` | int32 | 目标进程 ID |

**响应 204**：无响应体（信号已发送）

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | pid 路径段不是合法整数 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 当前角色为 viewer，或对目标进程无发送信号的权限（EPERM）|
| 404 | `NOT_FOUND` | 指定 PID 的进程不存在 |
| 500 | `INTERNAL` | 系统调用失败 |

---

## Phase 3 端点 — 磁盘与挂载管理

---

### GET /api/v1/disks/mounts

列出当前所有挂载点（读取 `/proc/mounts`）。**需要 viewer 或以上角色**。

**请求**：无参数

**响应 200**

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

| 字段 | 类型 | 说明 |
|------|------|------|
| `mounts` | array | 挂载点列表 |
| `mounts[].device` | string | 设备路径或网络路径 |
| `mounts[].mount_point` | string | 挂载目录 |
| `mounts[].fs_type` | string | 文件系统类型 |
| `mounts[].options` | string | 挂载选项 |

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 500 | `INTERNAL` | 读取 /proc/mounts 失败 |

---

### POST /api/v1/disks/mounts

挂载文件系统。**需要 operator 或 admin 角色**。

> CIFS 凭据（username/password/domain）由服务端通过临时凭据文件传递给 mount 命令，**不会回显到响应或写入日志**。

**请求体**

```json
{
  "source": "/dev/sdb1",
  "mount_point": "/mnt/data",
  "fs_type": "ext4",
  "read_only": false,
  "options": "",
  "username": "",
  "password": "",
  "domain": ""
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `source` | string | ✓ | 设备路径或网络路径 |
| `mount_point` | string | ✓ | 挂载目标目录 |
| `fs_type` | string | — | 文件系统类型（空字符串表示自动检测） |
| `read_only` | bool | — | 是否只读挂载 |
| `options` | string | — | 附加挂载选项（逗号分隔，不含凭据） |
| `username` | string | — | CIFS 用户名（不记录到日志） |
| `password` | string | — | CIFS 密码（不记录到日志） |
| `domain` | string | — | CIFS 域（可选，不记录到日志） |

**响应 201**：无响应体（挂载成功）

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | source 或 mount_point 缺失；请求体格式错误 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 当前角色为 viewer |
| 409 | `CONFLICT` | 目标挂载点已被挂载（幂等语义） |
| 500 | `INTERNAL` | mount 命令执行失败 |

> 挂载成功与失败均产生一条审计日志（`action: "disk.mount"`）。

---

### DELETE /api/v1/disks/mounts/{mountpoint}

卸载指定挂载点。**需要 operator 或 admin 角色**。

`{mountpoint}` 须 URL 编码（例如 `/mnt/data` → `%2Fmnt%2Fdata`）。

**路径参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `mountpoint` | string（URL 编码） | 要卸载的挂载点路径 |

**响应 204**：无响应体（卸载成功）

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | mountpoint 路径缺失或编码错误 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 当前角色为 viewer |
| 404 | `NOT_FOUND` | 指定挂载点当前未挂载（幂等语义） |
| 500 | `INTERNAL` | umount 命令执行失败（设备忙等） |

> 卸载成功与失败均产生一条审计日志（`action: "disk.umount"`）。

---

### GET /api/v1/disks/{device}/smart

查询指定设备的 SMART 健康信息（调用 `smartctl -j -a`）。**需要 viewer 或以上角色**。

`{device}` 可以是设备短名称（如 `sda`，自动补全为 `/dev/sda`）或 URL 编码的完整路径（如 `%2Fdev%2Fsda`）。

**路径参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `device` | string | 设备名（如 `sda`）或 URL 编码的设备路径 |

**响应 200**

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
| `health_ok` | bool | SMART 自检是否通过（`smart_status.passed`）|
| `temperature_c` | int | 当前温度（摄氏度） |
| `power_on_hours` | uint64 | 累计通电时间（小时） |

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 权限不足（需要 root 或 CAP_SYS_RAWIO）|
| 404 | `NOT_FOUND` | smartctl 未安装（需安装 smartmontools）|
| 500 | `INTERNAL` | 命令失败或输出无法解析 |

---

### GET /api/v1/disks/devices

列出系统上所有块设备（含未挂载设备，调用 `lsblk`）。**需要 viewer 或以上角色**。

**请求**：无参数

**响应 200**

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
| `devices` | array | 块设备列表 |
| `devices[].name` | string | 设备短名称（如 `sda`、`sda1`） |
| `devices[].size_bytes` | uint64 | 设备大小（字节，0 表示未知） |
| `devices[].type` | string | 设备类型：`disk`、`part`、`rom`、`loop` 等 |
| `devices[].mount_point` | string | 当前挂载点（未挂载时省略或为空） |
| `devices[].model` | string | 设备型号（分区或虚拟设备可能省略） |
| `devices[].removable` | bool | 是否为可移动设备（如 USB） |

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 500 | `INTERNAL` | lsblk 未安装或命令执行失败 |

---

### GET /api/v1/disks/{device}/power

查询指定设备的当前电源/睡眠状态（调用 `hdparm -C`）。**需要 viewer 或以上角色**。

`{device}` 可以是设备短名称（如 `sda`）或 URL 编码的完整路径（如 `%2Fdev%2Fsda`）。

**路径参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `device` | string | 设备名（如 `sda`）或 URL 编码的设备路径 |

**响应 200**

```json
{
  "device": "/dev/sda",
  "power_mode": "active"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `device` | string | 设备路径 |
| `power_mode` | string | 当前电源模式：`active`、`standby`、`sleeping`、`unknown` |

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 权限不足（需要 root 或 CAP_SYS_RAWIO）|
| 404 | `NOT_FOUND` | hdparm 未安装（需安装 hdparm 工具包）|
| 500 | `INTERNAL` | 命令执行失败 |

---

### POST /api/v1/disks/{device}/power

设置指定设备的电源/睡眠状态。**需要 operator 或 admin 角色**。

> 此操作产生一条审计日志（`action: "disk.power"`）。

**路径参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `device` | string | 设备名（如 `sda`）或 URL 编码的设备路径 |

**请求体**

```json
{
  "action": "standby"
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `action` | string | ✓ | 目标电源模式：`"standby"`（磁盘停转待机）或 `"sleep"`（强制睡眠） |

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | `action` 值无效或请求体格式错误 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer）或权限不足（需要 root 或 CAP_SYS_RAWIO）|
| 404 | `NOT_FOUND` | hdparm 未安装 |
| 500 | `INTERNAL` | 命令执行失败 |

---

## 审计日志

以下端点在执行时向 `/var/log/mingyue/audit.log` 追加一条 JSON Lines 审计记录：

| 端点 | 触发条件 |
|------|----------|
| `DELETE /api/v1/processes/{pid}` | 成功或失败均记录 |
| `POST /api/v1/disks/mounts` | 成功或失败均记录 |
| `DELETE /api/v1/disks/mounts/{mountpoint}` | 成功或失败均记录 |
| `POST /api/v1/disks/{device}/power` | 成功或失败均记录（`disk.power`）|
| `POST /api/v1/files` | 成功或失败均记录（`file.mkdir` 或 `file.write`） |
| `DELETE /api/v1/files` | 成功或失败均记录（`file.remove`） |
| `PUT /api/v1/files/move` | 成功或失败均记录（`file.move`） |
| `PUT /api/v1/files/copy` | 成功或失败均记录（`file.copy`） |
| `POST /api/v1/shares` | 成功或失败均记录（`share.create`） |
| `PUT /api/v1/shares/{name}` | 成功或失败均记录（`share.update`） |
| `DELETE /api/v1/shares/{name}` | 成功或失败均记录（`share.delete`） |
| `POST /api/v1/smb/shares` | 成功或失败均记录（`share.create`） |
| `PUT /api/v1/smb/shares/{name}` | 成功或失败均记录（`share.update`） |
| `DELETE /api/v1/smb/shares/{name}` | 成功或失败均记录（`share.delete`） |
| `POST /api/v1/nfs/exports` | 成功或失败均记录（`share.create`） |
| `PUT /api/v1/nfs/exports/{name}` | 成功或失败均记录（`share.update`） |
| `DELETE /api/v1/nfs/exports/{name}` | 成功或失败均记录（`share.delete`） |

**审计事件示例**

```json
{
  "time": "2026-02-27T06:00:00.000Z",
  "source": "192.168.1.10:52341",
  "action": "process.kill",
  "target": "pid:1234",
  "result": "success",
  "error_code": ""
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `time` | RFC3339 string | 操作发生时刻（UTC） |
| `source` | string | 调用来源（API 远端地址或 `"cli"`） |
| `action` | string | 操作名称 |
| `target` | string | 操作目标 |
| `result` | string | `"success"` 或 `"failure"` |
| `error_code` | string | 失败时的错误码，成功时为空 |

---

## Phase 4 — 文件管理端点

> **路径安全说明**：所有文件端点在服务层通过 `safePath()` 强制路径校验——字符串级目录穿越防护 + `EvalSymlinks` 符号链接逃逸防护。违规路径返回 `FORBIDDEN`（HTTP 403）而非错误详情，以防止路径探测。

### GET /api/v1/files

列出指定目录的内容。**需要 viewer 或以上角色**。

**查询参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `path` | string（必填） | 目标目录路径 |

**响应 200**

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
      "owner": "0",
      "unreadable": false
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `path` | string | 请求的目录路径 |
| `entries` | array | 目录条目列表 |
| `entries[].name` | string | 文件/目录基名 |
| `entries[].path` | string | 绝对路径 |
| `entries[].is_dir` | bool | 是否为目录 |
| `entries[].size` | int64 | 文件大小（字节，目录为 0） |
| `entries[].mode` | string | 权限字符串，如 `"-rw-r--r--"` |
| `entries[].mod_time` | RFC3339 string | 最后修改时间（UTC） |
| `entries[].owner` | string | 属主 UID（字符串格式） |
| `entries[].unreadable` | bool | 若 `true`，该条目元数据不可读（权限/竞态等），其余字段可能为空 |

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | `path` 参数缺失 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 路径越出配置的根目录边界 |
| 404 | `NOT_FOUND` | 目录不存在 |
| 500 | `INTERNAL` | 系统读取失败 |

---

### GET /api/v1/files/stat

获取文件或目录的元信息。**需要 viewer 或以上角色**。

**查询参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `path` | string（必填） | 目标路径 |

**响应 200**：单个 FileEntry 对象（字段同 `/api/v1/files` entries 条目）

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | `path` 参数缺失 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 路径越界 |
| 404 | `NOT_FOUND` | 路径不存在 |
| 500 | `INTERNAL` | 系统调用失败 |

---

### GET /api/v1/files/read

读取文件内容（base64 编码）。**需要 viewer 或以上角色**。

**查询参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `path` | string（必填） | 目标文件路径 |

**响应 200**

```json
{
  "path": "/var/log/syslog",
  "content": "PGh0bWw+Cg==",
  "encoding": "base64"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `path` | string | 文件路径 |
| `content` | string | 文件内容的 base64 编码（支持二进制文件） |
| `encoding` | string | 固定为 `"base64"` |

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | `path` 参数缺失 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 路径越界 |
| 404 | `NOT_FOUND` | 文件不存在 |
| 500 | `INTERNAL` | 读取失败 |

---

### POST /api/v1/files

创建文件或目录。**需要 operator 或 admin 角色**。审计记录 `file.write` 或 `file.mkdir`。

**请求体**

```json
{
  "path": "/tmp/hello.txt",
  "type": "file",
  "content": "aGVsbG8="
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `path` | string（必填） | 目标路径 |
| `type` | string | `"file"`（默认）或 `"dir"` |
| `content` | string | 文件内容（base64 编码）；`type=dir` 时忽略 |

**响应 201**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 格式错误、`path` 缺失、`type` 非法、`content` 非 base64 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer）或路径越界 |
| 500 | `INTERNAL` | 文件系统操作失败 |

---

### DELETE /api/v1/files

删除文件或目录。**需要 operator 或 admin 角色**。审计记录 `file.remove`。

**查询参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `path` | string（必填） | 目标路径 |
| `recursive` | bool（`true`/`false`，可选） | 是否递归删除目录，默认 `false` |

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | `path` 参数缺失 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer）或路径越界 |
| 404 | `NOT_FOUND` | 路径不存在 |
| 500 | `INTERNAL` | 文件系统操作失败 |

---

### PUT /api/v1/files/move

移动（重命名）文件或目录。**需要 operator 或 admin 角色**。审计记录 `file.move`。

**请求体**

```json
{
  "src": "/tmp/old.txt",
  "dst": "/tmp/new.txt"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `src` | string（必填） | 源路径 |
| `dst` | string（必填） | 目标路径 |

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 格式错误、`src` 或 `dst` 缺失 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足或路径越界 |
| 404 | `NOT_FOUND` | 源路径不存在 |
| 500 | `INTERNAL` | 移动失败 |

---

### PUT /api/v1/files/copy

复制文件。**需要 operator 或 admin 角色**。审计记录 `file.copy`。

**请求体**：同 `PUT /api/v1/files/move`（`src`、`dst` 字段）

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 格式错误、`src` 或 `dst` 缺失 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足或路径越界 |
| 404 | `NOT_FOUND` | 源文件不存在 |
| 500 | `INTERNAL` | 复制失败 |

---

## Phase 4 — 共享管理端点

> 共享状态持久化至 `/var/lib/mingyue/shares.json`；进程重启后自动恢复。
> Samba 配置片段写入 `/etc/samba/smb.conf.d/mingyue.conf`（需在 `/etc/samba/smb.conf` 中添加 `include` 指令）；
> NFS exports 片段写入 `/etc/exports.d/mingyue.exports`。
> 服务重载：Samba 类型共享触发 `smbcontrol all reload-config`；NFS 类型共享触发 `exportfs -ra`。

### Share 对象字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 共享名称（不含 `/`） |
| `type` | string | `"smb"` 或 `"nfs"` |
| `path` | string | 本地共享目录 |
| `comment` | string | 可选描述 |
| `read_only` | bool | 是否只读 |
| `enabled` | bool | 是否启用 |
| `valid_users` | string | **Samba 专属** 空格或逗号分隔的用户或 `@组`（空表示所有认证用户） |
| `write_list` | string | **Samba 专属** 拥有写权限的用户或 `@组`（空格或逗号分隔） |
| `create_mask` | string | **Samba 专属** 新建文件的八进制权限掩码，如 `"0644"` |
| `directory_mask` | string | **Samba 专属** 新建目录的八进制权限掩码，如 `"0755"` |
| `hosts` | string | **NFS 专属** 空格分隔的主机/CIDR（空表示 `*` 全部允许） |

---

### GET /api/v1/shares *(已弃用，请使用 `/api/v1/smb/shares` 或 `/api/v1/nfs/exports`)*

列出所有已配置的共享（Samba + NFS 混合）。**需要 viewer 或以上角色**。

**响应 200**

```json
{
  "shares": [
    {
      "name": "myshare",
      "type": "smb",
      "path": "/srv/myshare",
      "comment": "My share",
      "read_only": false,
      "enabled": true
    }
  ]
}
```

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 500 | `INTERNAL` | 列表获取失败 |

---

### GET /api/v1/shares/{name} *(已弃用)*

查询指定共享详情（类型不限）。**需要 viewer 或以上角色**。

**路径参数**：`name` — 共享名称

**响应 200**：单个 Share 对象（字段参见上方 Share 对象字段表）

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | 名称为空 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 404 | `NOT_FOUND` | 共享不存在 |

---

### POST /api/v1/shares *(已弃用，请使用 `/api/v1/smb/shares` 或 `/api/v1/nfs/exports`)*

创建新共享并重载服务。**需要 operator 或 admin 角色**。审计记录 `share.create`。

**请求体**：Share 对象（`type` 字段决定后端，支持 Samba 和 NFS 专属字段）

**响应 201**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 错误、必填字段缺失、类型不支持、名称含 `/` |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 409 | `CONFLICT` | 同名共享已存在 |
| 500 | `INTERNAL` | 创建失败或服务重载失败 |

---

### PUT /api/v1/shares/{name} *(已弃用)*

更新指定共享并重载服务。**需要 operator 或 admin 角色**。审计记录 `share.update`。重载失败时自动回滚至原始配置。

**路径参数**：`name` — 共享名称（路径参数优先）

**请求体**：同 `POST /api/v1/shares`（`name` 字段可省略）

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 错误、必填字段缺失、类型不支持 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 404 | `NOT_FOUND` | 共享不存在 |
| 500 | `INTERNAL` | 更新或重载失败 |

---

### DELETE /api/v1/shares/{name} *(已弃用)*

删除指定共享并重载服务。**需要 operator 或 admin 角色**。审计记录 `share.delete`。重载失败时自动回滚（重新创建该共享）。

**路径参数**：`name` — 共享名称

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | 名称为空 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 404 | `NOT_FOUND` | 共享不存在 |
| 500 | `INTERNAL` | 删除或重载失败 |

---

## Phase 4 — Samba 专属端点

> 以下端点仅操作 `type = "smb"` 的共享。创建/更新时 `type` 字段由服务端强制写入，无需在请求体中指定。

### GET /api/v1/smb/shares

列出所有 Samba 共享。**需要 viewer 或以上角色**。

**响应 200**

```json
{
  "shares": [
    {
      "name": "myshare",
      "type": "smb",
      "path": "/srv/myshare",
      "comment": "My share",
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

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 500 | `INTERNAL` | 列表获取失败 |

---

### GET /api/v1/smb/shares/{name}

查询指定 Samba 共享。**需要 viewer 或以上角色**。

**路径参数**：`name` — 共享名称

**响应 200**：单个 Share 对象（含 Samba 专属字段）

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | 名称为空 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 404 | `NOT_FOUND` | 共享不存在或类型不为 smb |

---

### POST /api/v1/smb/shares

创建 Samba 共享并重载 smbd。**需要 operator 或 admin 角色**。审计记录 `share.create`。

**请求体**

```json
{
  "name": "myshare",
  "path": "/srv/myshare",
  "comment": "optional description",
  "read_only": false,
  "enabled": true,
  "valid_users": "alice @staff",
  "write_list": "alice",
  "create_mask": "0644",
  "directory_mask": "0755"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string（必填） | 共享名称，不可含 `/` |
| `path` | string（必填） | 本地共享目录 |
| `comment` | string | 可选描述 |
| `read_only` | bool | 是否只读，默认 `false` |
| `enabled` | bool | 是否启用，默认 `true` |
| `valid_users` | string | 允许连接的用户或 `@组`（空格或逗号分隔；空表示所有认证用户） |
| `write_list` | string | 拥有写权限的用户或 `@组`（空格或逗号分隔） |
| `create_mask` | string | 新建文件权限掩码（八进制字符串，如 `"0644"`） |
| `directory_mask` | string | 新建目录权限掩码（八进制字符串，如 `"0755"`） |

**响应 201**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 错误、必填字段缺失、名称含 `/` |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 409 | `CONFLICT` | 同名共享已存在 |
| 500 | `INTERNAL` | 创建失败或 smbd 重载失败 |

---

### PUT /api/v1/smb/shares/{name}

更新 Samba 共享并重载 smbd。**需要 operator 或 admin 角色**。审计记录 `share.update`。重载失败时自动回滚。

**路径参数**：`name` — 共享名称（路径参数优先）

**请求体**：同 `POST /api/v1/smb/shares`（`name` 字段可省略）

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 错误、必填字段缺失 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 404 | `NOT_FOUND` | 共享不存在 |
| 500 | `INTERNAL` | 更新或重载失败 |

---

### DELETE /api/v1/smb/shares/{name}

删除 Samba 共享并重载 smbd。**需要 operator 或 admin 角色**。审计记录 `share.delete`。重载失败时自动回滚。

**路径参数**：`name` — 共享名称

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | 名称为空 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 404 | `NOT_FOUND` | 共享不存在 |
| 500 | `INTERNAL` | 删除或重载失败 |

---

## Phase 4 — Samba 用户管理端点

> 管理 Samba 密码数据库（tdbsam）中的用户账号，与系统 `/etc/shadow` 独立。
> Linux 系统账号必须存在，才能注册为 Samba 用户。
> 密码字段在传输中敏感，**不会**记录到审计日志。

### GET /api/v1/smb/users

列出所有 Samba 用户。**需要 viewer 或以上角色**。

**响应 200**

```json
{
  "users": [
    { "username": "alice" },
    { "username": "bob" }
  ]
}
```

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 500 | `INTERNAL` | pdbedit 命令失败 |

---

### POST /api/v1/smb/users

向 Samba 数据库添加用户并设置初始密码。**需要 operator 或 admin 角色**。

**请求体**

```json
{
  "username": "alice",
  "password": "s3cr3t"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `username` | string（必填） | 已存在的 Linux 用户名 |
| `password` | string（必填） | 初始 Samba 密码（不记录到日志） |

**响应 201**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 错误或用户名为空 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 500 | `INTERNAL` | smbpasswd 命令失败（用户不存在等） |

---

### DELETE /api/v1/smb/users/{username}

从 Samba 数据库删除用户。**需要 operator 或 admin 角色**。

**路径参数**：`username` — Samba 用户名

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | 用户名为空 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 500 | `INTERNAL` | smbpasswd 命令失败 |

---

### PUT /api/v1/smb/users/{username}/password

修改 Samba 用户密码。**需要 operator 或 admin 角色**。

**路径参数**：`username` — Samba 用户名

**请求体**

```json
{
  "password": "newpassword"
}
```

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 错误或用户名为空 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 500 | `INTERNAL` | smbpasswd 命令失败 |

---

## Phase 4 — NFS 专属端点

> 以下端点仅操作 `type = "nfs"` 的导出。创建/更新时 `type` 字段由服务端强制写入，无需在请求体中指定。

### GET /api/v1/nfs/exports

列出所有 NFS 导出。**需要 viewer 或以上角色**。

**响应 200**

```json
{
  "exports": [
    {
      "name": "nfsdata",
      "type": "nfs",
      "path": "/data/nfs",
      "comment": "Data export",
      "read_only": false,
      "enabled": true,
      "hosts": "192.168.1.0/24 10.0.0.5"
    }
  ]
}
```

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 500 | `INTERNAL` | 列表获取失败 |

---

### GET /api/v1/nfs/exports/{name}

查询指定 NFS 导出。**需要 viewer 或以上角色**。

**路径参数**：`name` — 导出名称

**响应 200**：单个 Share 对象（含 NFS 专属 `hosts` 字段）

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | 名称为空 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 404 | `NOT_FOUND` | 导出不存在或类型不为 nfs |

---

### POST /api/v1/nfs/exports

创建 NFS 导出并重载 exportfs。**需要 operator 或 admin 角色**。审计记录 `share.create`。

**请求体**

```json
{
  "name": "nfsdata",
  "path": "/data/nfs",
  "comment": "optional description",
  "read_only": false,
  "enabled": true,
  "hosts": "192.168.1.0/24 10.0.0.5"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string（必填） | 导出名称，不可含 `/` |
| `path` | string（必填） | 本地导出目录 |
| `comment` | string | 可选描述 |
| `read_only` | bool | 是否只读，默认 `false` |
| `enabled` | bool | 是否启用，默认 `true` |
| `hosts` | string | 空格分隔的主机/CIDR（空表示 `*` 全部允许） |

**响应 201**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 错误、必填字段缺失、名称含 `/` |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 409 | `CONFLICT` | 同名导出已存在 |
| 500 | `INTERNAL` | 创建失败或 exportfs 重载失败 |

---

### PUT /api/v1/nfs/exports/{name}

更新 NFS 导出并重载 exportfs。**需要 operator 或 admin 角色**。审计记录 `share.update`。重载失败时自动回滚。

**路径参数**：`name` — 导出名称（路径参数优先）

**请求体**：同 `POST /api/v1/nfs/exports`（`name` 字段可省略）

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 错误、必填字段缺失 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 404 | `NOT_FOUND` | 导出不存在 |
| 500 | `INTERNAL` | 更新或重载失败 |

---

### DELETE /api/v1/nfs/exports/{name}

删除 NFS 导出并重载 exportfs。**需要 operator 或 admin 角色**。审计记录 `share.delete`。重载失败时自动回滚。

**路径参数**：`name` — 导出名称

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | 名称为空 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 404 | `NOT_FOUND` | 导出不存在 |
| 500 | `INTERNAL` | 删除或重载失败 |

---

## Phase 5：网络管理 + 权限/ACL

### GET /api/v1/network/interfaces

列出所有网络接口。**任何已认证角色**均可访问。

**请求**：无参数

**响应 200**

```json
{
  "interfaces": [
    {
      "name": "eth0",
      "index": 2,
      "flags": ["UP", "BROADCAST", "MULTICAST"],
      "mtu": 1500,
      "hardware_addr": "52:54:00:ab:cd:ef",
      "addresses": [
        { "ip": "192.168.1.10", "prefix": 24, "family": "ipv4" },
        { "ip": "fe80::1", "prefix": 64, "family": "ipv6" }
      ]
    }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `interfaces` | array | 网络接口列表 |
| `interfaces[].name` | string | 接口名称 |
| `interfaces[].index` | int | OS 分配的接口编号 |
| `interfaces[].flags` | array\<string\> | 接口标志，如 `UP`、`BROADCAST`、`LOOPBACK` |
| `interfaces[].mtu` | int | 最大传输单元（字节） |
| `interfaces[].hardware_addr` | string | MAC 地址，虚拟/回环接口为空 |
| `interfaces[].addresses` | array | 单播地址列表 |
| `interfaces[].addresses[].ip` | string | IP 地址 |
| `interfaces[].addresses[].prefix` | int | CIDR 前缀长度 |
| `interfaces[].addresses[].family` | string | `"ipv4"` 或 `"ipv6"` |

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 500 | `INTERNAL` | 系统调用失败 |

---

### GET /api/v1/network/interfaces/{name}

查询单个网络接口详情。**任何已认证角色**均可访问。

**路径参数**：`name` — 接口名称（如 `eth0`、`lo`）

**响应 200**：同上述 `interfaces[]` 单条记录结构

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 404 | `NOT_FOUND` | 接口不存在 |
| 500 | `INTERNAL` | 系统调用失败 |

---

### PUT /api/v1/network/interfaces/{name}

对指定接口执行变更操作（启用/禁用/刷新 DHCP）。**需要 `admin` 角色**。

**路径参数**：`name` — 接口名称

**请求体**

```json
{ "action": "up" }
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `action` | string | 是 | `"up"`（启用）、`"down"`（禁用）、`"dhcp"`（刷新 DHCP 租约） |

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | `action` 值无效或请求体格式错误 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（非 admin） |
| 500 | `INTERNAL` | 系统命令执行失败 |

---

### GET /api/v1/acl

查询文件或目录的权限与 POSIX ACL 条目。**任何已认证角色**均可访问。

**查询参数**

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `path` | string | 是 | 目标文件或目录的绝对路径 |

**响应 200**

```json
{
  "path": "/srv/data",
  "owner": "1000",
  "group": "1000",
  "mode": "drwxr-xr-x",
  "acl_entries": [
    { "type": "user",  "qualifier": "",      "permissions": "rwx" },
    { "type": "group", "qualifier": "",      "permissions": "r-x" },
    { "type": "user",  "qualifier": "alice", "permissions": "rwx" },
    { "type": "mask",  "qualifier": "",      "permissions": "rwx" },
    { "type": "other", "qualifier": "",      "permissions": "r--" }
  ]
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `path` | string | 目标路径（绝对路径） |
| `owner` | string | 所有者 UID（或名称） |
| `group` | string | 所有者组 GID（或名称） |
| `mode` | string | Unix 权限字符串 |
| `acl_entries` | array | POSIX ACL 条目；未安装 getfacl 时为空数组 |
| `acl_entries[].type` | string | `"user"`、`"group"`、`"mask"`、`"other"` |
| `acl_entries[].qualifier` | string | 用户名或组名；空字符串表示所有者用户/组 |
| `acl_entries[].permissions` | string | 三字符权限串，如 `"rwx"`、`"r--"` |

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | `path` 参数缺失 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 路径越权（目录穿越或超出根目录） |
| 404 | `NOT_FOUND` | 路径不存在 |
| 500 | `INTERNAL` | 系统调用失败 |

---

### PUT /api/v1/acl

设置文件或目录的 POSIX ACL 条目（调用 `setfacl`）。**需要 `operator` 或 `admin` 角色**。

**请求体**

```json
{
  "path": "/srv/data",
  "entries": ["u:alice:rwx", "g:devs:r-x"]
}
```

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `path` | string | 是 | 目标文件或目录的绝对路径 |
| `entries` | array\<string\> | 是 | ACL 规格列表，格式为 `"type:qualifier:perms"`（如 `"u:alice:rwx"`） |

**响应 204**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | `path` 为空、`entries` 为空或请求体格式错误 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer）或路径越权 |
| 404 | `NOT_FOUND` | 路径不存在或 `setfacl` 未安装（含安装提示） |
| 500 | `INTERNAL` | `setfacl` 执行失败 |

---

## Phase 6：OpenAPI 规范 + CI 同步

Phase 6 不新增 API 端点，其产出为机器可读的 API 文档规范与 CI 同步机制。

### OpenAPI v3 规范文件

项目提供完整的 OpenAPI v3 规范文件，覆盖 v1 全量端点：

```
docs/api/openapi.yaml    # OpenAPI v3 规范（YAML 格式）
```

规范文件包含：
- 认证方式（Bearer Token）与角色说明
- 所有端点的路径、HTTP 方法、请求参数与请求体
- 响应结构（含成功响应与错误响应）
- 可复用的 schema 组件（HostSnapshot、Process、Mount、FileEntry、Share、ACLInfo、BlockDevice、DiskPower 等）

**快速预览**

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

### CI 同步机制

`.github/workflows/openapi-sync.yaml` 在 PR 合并时对 OpenAPI 规范执行自动校验：
- 使用 `@stoplight/spectral-cli` 对 `docs/api/openapi.yaml` 执行 lint 校验
- 检查规范文件是否包含关键路径覆盖（health、system/overview、disks/mounts、shares 等）
- 任何 lint 失败或路径缺失均导致 CI 检查不通过，防止规范与实现发生漂移
