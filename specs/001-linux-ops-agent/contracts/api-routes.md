# HTTP API 路由契约（Phase 1–4）

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

API Key 通过 `auth.RegisterAPIKey()` 在进程启动时注册（当前为内存存储，重启后需重新配置）。

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

## 审计日志

以下端点在执行时向 `/var/log/mingyue/audit.log` 追加一条 JSON Lines 审计记录：

| 端点 | 触发条件 |
|------|----------|
| `DELETE /api/v1/processes/{pid}` | 成功或失败均记录 |
| `POST /api/v1/files` | 成功或失败均记录（`file.mkdir` 或 `file.write`） |
| `DELETE /api/v1/files` | 成功或失败均记录（`file.remove`） |
| `PUT /api/v1/files/move` | 成功或失败均记录（`file.move`） |
| `PUT /api/v1/files/copy` | 成功或失败均记录（`file.copy`） |
| `POST /api/v1/shares` | 成功或失败均记录（`share.create`） |
| `PUT /api/v1/shares/{name}` | 成功或失败均记录（`share.update`） |
| `DELETE /api/v1/shares/{name}` | 成功或失败均记录（`share.delete`） |

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

> **说明**：当前共享后端为内存存储（placeholder），变更不落盘，重启后丢失。真实 Samba/NFS 配置文件支持为后续迭代任务。

### GET /api/v1/shares

列出所有已配置的共享。**需要 viewer 或以上角色**。

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

| 字段 | 类型 | 说明 |
|------|------|------|
| `shares` | array | 共享列表（可为空数组） |
| `shares[].name` | string | 共享名称（不含 `/`） |
| `shares[].type` | string | `"smb"` 或 `"nfs"` |
| `shares[].path` | string | 本地共享目录 |
| `shares[].comment` | string | 可选描述 |
| `shares[].read_only` | bool | 是否只读 |
| `shares[].enabled` | bool | 是否启用 |

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 500 | `INTERNAL` | 列表获取失败 |

---

### GET /api/v1/shares/{name}

查询指定共享详情。**需要 viewer 或以上角色**。

**路径参数**：`name` — 共享名称

**响应 200**：单个 Share 对象（字段同 `shares[]` 条目）

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | 名称为空 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 404 | `NOT_FOUND` | 共享不存在 |

---

### POST /api/v1/shares

创建新共享并重载服务。**需要 operator 或 admin 角色**。审计记录 `share.create`。

**请求体**

```json
{
  "name": "myshare",
  "type": "smb",
  "path": "/srv/myshare",
  "comment": "optional description",
  "read_only": false,
  "enabled": true
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string（必填） | 共享名称，不可含 `/` |
| `type` | string（必填） | `"smb"` 或 `"nfs"` |
| `path` | string（必填） | 本地共享目录 |
| `comment` | string | 可选描述 |
| `read_only` | bool | 是否只读，默认 `false` |
| `enabled` | bool | 是否启用，默认 `true` |

**响应 201**：无响应体

**错误响应**

| HTTP 状态码 | 错误码 | 触发条件 |
|-------------|--------|----------|
| 400 | `INVALID_INPUT` | JSON 错误、必填字段缺失、类型不支持、名称含 `/`、名称已存在 |
| 401 | `UNAUTHORIZED` | 未提供有效 Bearer Token |
| 403 | `FORBIDDEN` | 角色不足（viewer） |
| 500 | `INTERNAL` | 创建失败或服务重载失败 |

---

### PUT /api/v1/shares/{name}

更新指定共享并重载服务。**需要 operator 或 admin 角色**。审计记录 `share.update`。重载失败时自动回滚至原始配置。

**路径参数**：`name` — 共享名称（与请求体中的 name 一致；路径参数优先）

**请求体**：同 `POST /api/v1/shares`（`name` 字段可省略，以路径参数为准）

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

### DELETE /api/v1/shares/{name}

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
