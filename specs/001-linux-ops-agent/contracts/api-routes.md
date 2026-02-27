# HTTP API 路由契约（Phase 1–2）

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
- `operator`：只读 + 进程终止等非破坏性写操作
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
