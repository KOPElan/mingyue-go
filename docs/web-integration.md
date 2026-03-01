# mingyue Web 前端接入指南

本文档是 Web 前端开发者接入 **mingyue agent** 的通用指南，涵盖：认证流程、局域网自动发现、完整 API 参考，以及 TypeScript/JavaScript 示例代码。

---

## 目录

1. [概述](#1-概述)
2. [快速开始](#2-快速开始)
3. [认证机制](#3-认证机制)
4. [局域网自动发现](#4-局域网自动发现)
5. [API 参考](#5-api-参考)
   - 5.1 [基础信息](#51-基础信息)
   - 5.2 [系统监控](#52-系统监控)
   - 5.3 [进程管理](#53-进程管理)
   - 5.4 [磁盘与挂载](#54-磁盘与挂载)
   - 5.5 [文件管理](#55-文件管理)
   - 5.6 [Samba 共享管理](#56-samba-共享管理)
   - 5.7 [NFS 导出管理](#57-nfs-导出管理)
   - 5.8 [网络管理](#58-网络管理)
   - 5.9 [权限与 ACL 管理](#59-权限与-acl-管理)
6. [错误处理](#6-错误处理)
7. [TypeScript 客户端示例](#7-typescript-客户端示例)
8. [安全建议](#8-安全建议)
9. [常见问题](#9-常见问题)

---

## 1. 概述

mingyue agent 是一个运行在 Linux 宿主机上的 HTTP 守护进程，默认监听 `:7070`，提供 RESTful JSON API，供 Web 管理界面操控宿主机。

**通信模型：**

```
Web 浏览器 / Web 前端
       │
       │  HTTP + Bearer Token
       ▼
 mingyue agent (:7070)
       │
       │  Linux syscalls / CLI tools
       ▼
   宿主机 OS
```

**支持功能：**

| 功能模块 | 说明 |
|---------|------|
| 系统监控 | CPU、内存、磁盘使用率、运行时间 |
| 进程管理 | 进程列表查询、按 PID 终止进程 |
| 磁盘管理 | 块设备列表、挂载/卸载、SMART 健康、电源控制 |
| 文件管理 | 目录浏览、文件读写、创建/删除/移动/复制 |
| Samba 共享 | SMB 共享 CRUD、用户管理 |
| NFS 导出 | NFS exports CRUD |
| 网络管理 | 网络接口查询、up/down/dhcp |
| 权限/ACL | POSIX ACL 查询与设置 |

---

## 2. 快速开始

### 步骤一：在宿主机上启动 agent

> **⚠️ systemd 安全警告**：若通过 systemd 服务启动 agent（`StandardOutput=journal`），初始密钥会被写入系统日志，任何有日志读取权限的用户均可获取完整 admin token。**请勿在生产环境通过 systemd 首次启动来获取密钥**。
>
> **推荐方案**：
> 1. **交互式首次初始化**：以终端交互方式手动运行一次（如下），保存密钥后停止，再通过服务方式正式运行。
> 2. **预先生成密钥**：在启动服务前通过 `sudo mingyue auth keygen --role admin --subject admin` 手动创建管理员密钥，避免服务启动时打印密钥。

```bash
# 首次启动（交互式终端，非 systemd 环境） — 会自动生成初始 admin 密钥
sudo ./mingyue agent start

# 输出示例：
# Starting mingyue daemon on :7070 (pid file: /var/run/mingyue/mingyue.pid)
#
# *** Initial admin API key (save this) ***
# a3f1c2d4e5f6789012345678901234567890abcdef01234567890abcdef012345
```

**请立即保存打印出的密钥**，它仅在首次启动时显示。

### 步骤二：验证连通性

```bash
# 无需认证的健康检查
curl http://<agent-ip>:7070/api/v1/health
# 返回: {"status":"ok","version":"dev",...}
```

### 步骤三：使用 API Key 发起认证请求

```bash
curl -H "Authorization: Bearer <your-api-key>" \
     http://<agent-ip>:7070/api/v1/system/overview
```

### 步骤四（可选）：创建 operator 角色密钥供前端使用

```bash
# 在宿主机上执行
sudo mingyue auth keygen --role operator --subject "web-frontend"
# 输出的 key 即为 web 前端专用密钥
```

---

## 3. 认证机制

### 3.1 Bearer Token 认证

所有 API（除 `/health` 和 `/version` 外）均需要在 HTTP 请求头中携带 API Key：

```http
Authorization: Bearer <api-key>
```

- 缺少 Token → `401 Unauthorized`
- Token 无效 → `401 Unauthorized`
- Token 有效但权限不足 → `403 Forbidden`

### 3.2 角色权限模型

| 角色 | 说明 | 适用场景 |
|------|------|---------|
| `viewer` | 只读操作 | 监控看板、状态展示 |
| `operator` | 读写（不含网络变更） | 文件管理、共享管理 |
| `admin` | 全部权限（含网络变更） | 系统管理员 |

**权限层级**：`viewer < operator < admin`，高级角色包含低级角色的全部权限。

### 3.3 密钥管理（宿主机 CLI）

```bash
# 生成新密钥
sudo mingyue auth keygen --role viewer --subject "read-only-dashboard"
sudo mingyue auth keygen --role operator --subject "web-frontend"
sudo mingyue auth keygen --role admin --subject "admin-panel"

# 列出所有密钥（密钥值部分掩码）
sudo mingyue auth list

# 撤销密钥
sudo mingyue auth revoke <full-key-value>
```

密钥持久化到 `/var/lib/mingyue/apikeys.json`（权限 `0600`），agent 重启后自动恢复。

### 3.4 前端安全存储建议

- ✅ 将 API Key 存储在 **环境变量** 或 **服务端配置文件** 中
- ✅ 通过后端代理转发请求，避免前端直接持有 Key
- ✅ 使用 `operator` 而非 `admin` 角色，遵循最小权限原则
- ❌ 不要将 API Key 硬编码在前端 JavaScript 源码中
- ❌ 不要将 API Key 存储在 `localStorage` 或 `sessionStorage`（XSS 风险）

---

## 4. 局域网自动发现

agent 启动后会每 3 秒向局域网多播地址 `224.0.0.251:7071` 发送包含自身信息的 UDP 数据包，Web 前端可通过以下方式发现宿主机 agent。

### 4.1 CLI 发现（管理工具）

```bash
# 扫描局域网内的所有 agent（默认等待 3 秒）
mingyue agent discover

# 输出示例：
# Scanning for mingyue agents (3s)...
# HOSTNAME                        ADDRESS               VERSION
# ------------------------------  --------------------  -------
# myserver                        :7070                 1.0.0
# nas-box                         :7070                 1.0.0
```

### 4.2 发现协议说明

| 属性 | 值 |
|------|---|
| 协议 | UDP 多播（IPv4） |
| 多播组 | `224.0.0.251` |
| 端口 | `7071` |
| 公告间隔 | 每 3 秒 |
| 数据格式 | JSON |

**公告数据包格式：**

```json
{
  "hostname": "myserver",
  "addr": ":7070",
  "version": "1.0.0"
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `hostname` | string | 宿主机 hostname |
| `addr` | string | agent HTTP 监听地址（`host:port` 或 `:port`） |
| `version` | string | agent 软件版本 |

### 4.3 前端发现实现思路

由于浏览器无法直接发送/接收 UDP 多播，推荐以下方案：

**方案一（推荐）：后端代理发现**
- 在 Web 应用的 Node.js/Go 后端中监听 UDP 多播
- 收集到 agent 列表后通过 WebSocket 或 REST API 推送给前端

**方案二：手动配置**
- 由用户在 Web 界面输入 agent 的 IP 和端口
- 前端通过 `/api/v1/health` 验证连通性

**方案三：已知网段扫描**
- 前端请求后端对指定网段的 `:7070` 端口进行 TCP 探测
- 命中后请求 `/api/v1/version` 确认是 mingyue agent

---

## 5. API 参考

### 基础信息

| 属性 | 值 |
|------|---|
| Base URL | `http://<agent-host>:7070` |
| 路径前缀 | `/api/v1` |
| Content-Type | `application/json` |
| 认证方式 | `Authorization: Bearer <api-key>` |

---

### 5.1 基础信息

#### GET /api/v1/health

健康检查。**无需认证**。

**响应 200**

```json
{
  "status": "ok",
  "version": "1.0.0",
  "go_os": "linux",
  "go_arch": "amd64"
}
```

#### GET /api/v1/version

查询版本信息。**无需认证**。

**响应 200**

```json
{
  "version": "1.0.0"
}
```

---

### 5.2 系统监控

#### GET /api/v1/system/overview

获取 CPU、内存、运行时间概览。**需要 `viewer` 及以上角色**。

**响应 200**

```json
{
  "timestamp": "2026-01-01T00:00:00.000Z",
  "cpu_percent": 42.0,
  "mem_total": 8589934592,
  "mem_used": 4294967296,
  "mem_percent": 50.0,
  "uptime": 3600
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `timestamp` | RFC3339 | 采集时刻（UTC） |
| `cpu_percent` | float64 | 全系统 CPU 使用率（0–100） |
| `mem_total` | uint64 | 总物理内存（字节） |
| `mem_used` | uint64 | 已用物理内存（字节） |
| `mem_percent` | float64 | 内存使用率（0–100） |
| `uptime` | uint64 | 系统运行时间（秒） |

---

### 5.3 进程管理

#### GET /api/v1/processes

获取进程列表（支持分页）。**需要 `viewer` 及以上角色**。

**查询参数**

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `limit` | int | 0（不限制） | 每页最大条数 |
| `page` | int | 1 | 页码（1 起始） |

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

#### GET /api/v1/processes/{pid}

查询指定 PID 进程详情。**需要 `viewer` 及以上角色**。

**响应 200**：单个 Process 对象（同上 `processes[]` 条目结构）

**错误**：`404 NOT_FOUND` — 进程不存在

#### DELETE /api/v1/processes/{pid}

向指定 PID 发送 SIGTERM 信号。**需要 `operator` 或 `admin` 角色**。

**响应 204**：无响应体

**错误**：`403 FORBIDDEN`、`404 NOT_FOUND`

---

### 5.4 磁盘与挂载

#### GET /api/v1/disks/devices

列出所有块设备（含未挂载设备）。**需要 `viewer` 及以上角色**。

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
    }
  ]
}
```

#### GET /api/v1/disks/mounts

列出当前所有挂载点。**需要 `viewer` 及以上角色**。

**响应 200**

```json
{
  "mounts": [
    {
      "device": "/dev/sda1",
      "mount_point": "/",
      "fs_type": "ext4",
      "options": "rw,relatime"
    }
  ]
}
```

#### POST /api/v1/disks/mounts

挂载文件系统。**需要 `operator` 或 `admin` 角色**。

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

> CIFS 凭据（username/password）不会记录到审计日志或响应体。

**响应 201**：无响应体

#### DELETE /api/v1/disks/mounts/{mountpoint}

卸载指定挂载点。`{mountpoint}` 需 URL 编码（`/mnt/data` → `%2Fmnt%2Fdata`）。**需要 `operator` 或 `admin` 角色**。

**响应 204**：无响应体

#### GET /api/v1/disks/{device}/smart

查询设备 SMART 健康信息（需安装 `smartmontools`）。**需要 `viewer` 及以上角色**。

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

#### GET /api/v1/disks/{device}/power

查询设备电源状态（需安装 `hdparm`）。**需要 `viewer` 及以上角色**。

**响应 200**

```json
{
  "device": "/dev/sda",
  "power_mode": "active"
}
```

`power_mode` 可选值：`active`、`standby`、`sleeping`、`unknown`

#### POST /api/v1/disks/{device}/power

设置设备电源状态。**需要 `operator` 或 `admin` 角色**。

**请求体**

```json
{
  "action": "standby"
}
```

`action` 可选值：`standby`（磁盘停转待机）、`sleep`（强制睡眠）

**响应 204**：无响应体

---

### 5.5 文件管理

> **路径安全**：所有文件操作均进行路径穿越防护（含符号链接逃逸检测），违规路径返回 `403 FORBIDDEN`。

#### GET /api/v1/files

列出目录内容。**需要 `viewer` 及以上角色**。

**查询参数**：`path`（必填）— 目标目录路径

**响应 200**

```json
{
  "path": "/var/log",
  "entries": [
    {
      "name": "syslog",
      "type": "file",
      "size": 102400,
      "mode": "0644",
      "mod_time": "2026-01-01T00:00:00Z",
      "is_dir": false,
      "is_symlink": false
    }
  ]
}
```

#### GET /api/v1/files/stat

查询文件或目录元信息。**需要 `viewer` 及以上角色**。

**查询参数**：`path`（必填）

**响应 200**：单个文件条目对象（同上 `entries[]` 结构）

#### GET /api/v1/files/read

读取文件内容。**需要 `viewer` 及以上角色**。

**查询参数**：`path`（必填）

**响应 200**

```json
{
  "path": "/etc/hostname",
  "content": "bXlzZXJ2ZXI="
}
```

> `content` 为 Base64 编码的文件内容，前端需解码后使用：
> ```javascript
> const text = atob(response.content);
> ```

#### POST /api/v1/files

创建文件或目录。**需要 `operator` 或 `admin` 角色**。

**请求体（创建文件）**

```json
{
  "path": "/tmp/hello.txt",
  "type": "file",
  "content": "aGVsbG8="
}
```

`content` 为 Base64 编码内容：
```javascript
const content = btoa("hello world");
```

**请求体（创建目录）**

```json
{
  "path": "/tmp/mydir",
  "type": "dir"
}
```

**响应 201**：无响应体

#### DELETE /api/v1/files

删除文件或目录。**需要 `operator` 或 `admin` 角色**。

**查询参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `path` | string（必填） | 目标路径 |
| `recursive` | bool | `true` 时递归删除目录 |

**响应 204**：无响应体

#### PUT /api/v1/files/move

移动（重命名）文件或目录。**需要 `operator` 或 `admin` 角色**。

**请求体**

```json
{
  "src": "/tmp/old.txt",
  "dst": "/tmp/new.txt"
}
```

**响应 204**：无响应体

#### PUT /api/v1/files/copy

复制文件或目录。**需要 `operator` 或 `admin` 角色**。

**请求体**

```json
{
  "src": "/tmp/a.txt",
  "dst": "/tmp/b.txt"
}
```

**响应 204**：无响应体

---

### 5.6 Samba 共享管理

#### GET /api/v1/smb/shares

列出所有 Samba 共享。**需要 `viewer` 及以上角色**。

**响应 200**

```json
{
  "shares": [
    {
      "name": "myshare",
      "type": "smb",
      "path": "/srv/myshare",
      "read_only": false,
      "enabled": true,
      "valid_users": "",
      "write_list": "",
      "create_mask": "0664",
      "directory_mask": "0775"
    }
  ]
}
```

#### GET /api/v1/smb/shares/{name}

查询指定 Samba 共享详情。**需要 `viewer` 及以上角色**。

**响应 200**：单个 Share 对象

#### POST /api/v1/smb/shares

创建 Samba 共享。**需要 `operator` 或 `admin` 角色**。

**请求体**

```json
{
  "name": "myshare",
  "path": "/srv/myshare",
  "read_only": false,
  "enabled": true,
  "valid_users": "alice bob",
  "write_list": "alice",
  "create_mask": "0664",
  "directory_mask": "0775"
}
```

**响应 201**：无响应体

#### PUT /api/v1/smb/shares/{name}

更新 Samba 共享配置。**需要 `operator` 或 `admin` 角色**。

**请求体**：同创建（`name` 字段忽略，使用路径参数）

**响应 204**：无响应体

#### DELETE /api/v1/smb/shares/{name}

删除 Samba 共享。**需要 `operator` 或 `admin` 角色**。

**响应 204**：无响应体

#### Samba 用户管理

| 方法 | 路径 | 角色 | 说明 |
|------|------|------|------|
| GET | `/api/v1/smb/users` | viewer+ | 列出所有 Samba 用户 |
| POST | `/api/v1/smb/users` | operator+ | 创建 Samba 用户 |
| PUT | `/api/v1/smb/users/{username}/password` | operator+ | 修改用户密码 |
| DELETE | `/api/v1/smb/users/{username}` | operator+ | 删除 Samba 用户 |

---

### 5.7 NFS 导出管理

#### GET /api/v1/nfs/exports

列出所有 NFS 导出。**需要 `viewer` 及以上角色**。

**响应 200**

```json
{
  "shares": [
    {
      "name": "myexport",
      "type": "nfs",
      "path": "/srv/myexport",
      "read_only": false,
      "enabled": true,
      "hosts": "192.168.1.0/24"
    }
  ]
}
```

#### POST /api/v1/nfs/exports

创建 NFS 导出。**需要 `operator` 或 `admin` 角色**。

**请求体**

```json
{
  "name": "myexport",
  "path": "/srv/myexport",
  "read_only": false,
  "enabled": true,
  "hosts": "192.168.1.0/24"
}
```

**响应 201**：无响应体

---

### 5.8 网络管理

#### GET /api/v1/network/interfaces

列出所有网络接口。**需要 `viewer` 及以上角色**。

**响应 200**

```json
{
  "interfaces": [
    {
      "name": "eth0",
      "addresses": ["192.168.1.100/24", "fe80::1/64"],
      "mac": "aa:bb:cc:dd:ee:ff",
      "mtu": 1500,
      "up": true,
      "flags": ["UP", "BROADCAST", "RUNNING", "MULTICAST"]
    }
  ]
}
```

#### GET /api/v1/network/interfaces/{name}

查询指定接口详情。**需要 `viewer` 及以上角色**。

**响应 200**：单个接口对象

#### PUT /api/v1/network/interfaces/{name}

变更网络接口状态。**需要 `admin` 角色**。

**请求体**

```json
{
  "action": "up"
}
```

`action` 可选值：

| 值 | 说明 |
|----|------|
| `up` | 启用接口（`ip link set up`） |
| `down` | 禁用接口（`ip link set down`） |
| `dhcp` | 刷新 DHCP 租约（`dhclient`） |

**响应 204**：无响应体

---

### 5.9 权限与 ACL 管理

#### GET /api/v1/acl

查询文件或目录的权限与 POSIX ACL。**需要 `viewer` 及以上角色**。

**查询参数**：`path`（必填）

**响应 200**

```json
{
  "path": "/srv/data",
  "mode": "0755",
  "owner": "alice",
  "group": "devs",
  "acl_entries": [
    {
      "tag": "user",
      "qualifier": "bob",
      "permissions": "r-x"
    }
  ]
}
```

> 若系统未安装 `getfacl`，`acl_entries` 返回空数组（优雅降级）。

#### PUT /api/v1/acl

设置 POSIX ACL 条目（需安装 `setfacl`）。**需要 `operator` 或 `admin` 角色**。

**请求体**

```json
{
  "path": "/srv/data",
  "entries": [
    "u:alice:rwx",
    "g:devs:r-x"
  ]
}
```

`entries` 格式遵循 `setfacl` 约定：`[u|g|o|m]:[qualifier]:[rwx-]`

**响应 204**：无响应体

---

## 6. 错误处理

所有错误响应均返回统一的 JSON 结构：

```json
{
  "code": "NOT_FOUND",
  "message": "process 9999 not found"
}
```

| 错误码 | HTTP 状态码 | 说明 |
|--------|-------------|------|
| `UNAUTHORIZED` | 401 | 缺少或无效的 API Key |
| `FORBIDDEN` | 403 | 角色权限不足 |
| `NOT_FOUND` | 404 | 资源不存在 |
| `INVALID_INPUT` | 400 | 请求参数格式或值无效 |
| `CONFLICT` | 409 | 资源冲突（如挂载点已挂载） |
| `INTERNAL` | 500 | 内部服务器错误 |

**前端错误处理建议：**

```typescript
async function apiRequest(url: string, options?: RequestInit) {
  const res = await fetch(url, options);
  if (!res.ok) {
    const err = await res.json() as { code: string; message: string };
    if (err.code === 'UNAUTHORIZED') {
      // 跳转到登录页 / 提示重新输入 API Key
    }
    throw new Error(`[${err.code}] ${err.message}`);
  }
  return res.status === 204 ? null : res.json();
}
```

---

## 7. TypeScript 客户端示例

### 7.1 基础客户端封装

```typescript
// mingyue-client.ts

const DEFAULT_BASE_URL = 'http://localhost:7070';

interface AgentClientOptions {
  baseUrl?: string;
  apiKey: string;
}

interface AppError {
  code: string;
  message: string;
}

class MingyueClient {
  private baseUrl: string;
  private headers: HeadersInit;

  constructor(options: AgentClientOptions) {
    this.baseUrl = (options.baseUrl ?? DEFAULT_BASE_URL).replace(/\/$/, '');
    this.headers = {
      'Content-Type': 'application/json',
      'Authorization': `Bearer ${options.apiKey}`,
    };
  }

  private async request<T>(
    path: string,
    options: RequestInit = {}
  ): Promise<T | null> {
    const res = await fetch(`${this.baseUrl}/api/v1${path}`, {
      ...options,
      headers: { ...this.headers, ...options.headers },
    });

    if (!res.ok) {
      const err = (await res.json()) as AppError;
      throw new Error(`[${err.code}] ${err.message}`);
    }

    if (res.status === 204) return null;
    return res.json() as Promise<T>;
  }

  // ── 健康检查 ──────────────────────────────────────────────────────────────

  async health() {
    return this.request<{ status: string; version: string }>('/health', {
      headers: {},  // 健康检查不需要认证
    });
  }

  // ── 系统监控 ──────────────────────────────────────────────────────────────

  async getSystemOverview() {
    return this.request<{
      timestamp: string;
      cpu_percent: number;
      mem_total: number;
      mem_used: number;
      mem_percent: number;
      uptime: number;
    }>('/system/overview');
  }

  // ── 进程管理 ──────────────────────────────────────────────────────────────

  async listProcesses(limit = 0, page = 1) {
    return this.request<{
      total: number;
      processes: Array<{
        pid: number;
        name: string;
        status: string;
        cpu_percent: number;
        mem_rss: number;
        user: string;
        cmdline: string;
      }>;
    }>(`/processes?limit=${limit}&page=${page}`);
  }

  async killProcess(pid: number) {
    return this.request(`/processes/${pid}`, { method: 'DELETE' });
  }

  // ── 磁盘与挂载 ────────────────────────────────────────────────────────────

  async listBlockDevices() {
    return this.request<{
      devices: Array<{
        name: string;
        size_bytes: number;
        type: string;
        mount_point: string;
        model: string;
        removable: boolean;
      }>;
    }>('/disks/devices');
  }

  async listMounts() {
    return this.request<{
      mounts: Array<{
        device: string;
        mount_point: string;
        fs_type: string;
        options: string;
      }>;
    }>('/disks/mounts');
  }

  async mount(params: {
    source: string;
    mount_point: string;
    fs_type?: string;
    read_only?: boolean;
    options?: string;
    username?: string;
    password?: string;
    domain?: string;
  }) {
    return this.request('/disks/mounts', {
      method: 'POST',
      body: JSON.stringify(params),
    });
  }

  async umount(mountpoint: string) {
    return this.request(
      `/disks/mounts/${encodeURIComponent(mountpoint)}`,
      { method: 'DELETE' }
    );
  }

  // ── 文件管理 ──────────────────────────────────────────────────────────────

  async listFiles(path: string) {
    return this.request<{
      path: string;
      entries: Array<{
        name: string;
        type: string;
        size: number;
        mode: string;
        mod_time: string;
        is_dir: boolean;
        is_symlink: boolean;
      }>;
    }>(`/files?path=${encodeURIComponent(path)}`);
  }

  async readFile(path: string): Promise<string> {
    const res = await this.request<{ path: string; content: string }>(
      `/files/read?path=${encodeURIComponent(path)}`
    );
    return atob(res!.content);
  }

  async writeFile(path: string, content: string) {
    return this.request('/files', {
      method: 'POST',
      body: JSON.stringify({ path, type: 'file', content: btoa(content) }),
    });
  }

  async deleteFile(path: string, recursive = false) {
    return this.request(
      `/files?path=${encodeURIComponent(path)}&recursive=${recursive}`,
      { method: 'DELETE' }
    );
  }

  // ── 网络管理 ──────────────────────────────────────────────────────────────

  async listInterfaces() {
    return this.request<{
      interfaces: Array<{
        name: string;
        addresses: string[];
        mac: string;
        mtu: number;
        up: boolean;
        flags: string[];
      }>;
    }>('/network/interfaces');
  }

  async setInterfaceAction(name: string, action: 'up' | 'down' | 'dhcp') {
    return this.request(`/network/interfaces/${encodeURIComponent(name)}`, {
      method: 'PUT',
      body: JSON.stringify({ action }),
    });
  }
}

export { MingyueClient };
export type { AgentClientOptions };
```

### 7.2 使用示例

```typescript
import { MingyueClient } from './mingyue-client';

const client = new MingyueClient({
  baseUrl: 'http://192.168.1.100:7070',
  apiKey: process.env.MINGYUE_API_KEY!,
});

// 获取系统概览
const overview = await client.getSystemOverview();
console.log(`CPU: ${overview!.cpu_percent.toFixed(1)}%`);
console.log(`内存: ${(overview!.mem_used / 1024 ** 3).toFixed(1)} GB / ${(overview!.mem_total / 1024 ** 3).toFixed(1)} GB`);

// 列出磁盘设备
const { devices } = await client.listBlockDevices() ?? { devices: [] };
devices.forEach(d => {
  console.log(`${d.name}: ${(d.size_bytes / 1024 ** 3).toFixed(0)} GB [${d.type}]`);
});

// 读取文件
const content = await client.readFile('/etc/hostname');
console.log('主机名:', content.trim());
```

### 7.3 多 agent 场景（动态切换）

```typescript
class AgentManager {
  private clients = new Map<string, MingyueClient>();

  addAgent(hostname: string, addr: string, apiKey: string) {
    const host = addr.startsWith(':') ? `http://localhost${addr}` : `http://${addr}`;
    this.clients.set(hostname, new MingyueClient({ baseUrl: host, apiKey }));
  }

  getClient(hostname: string) {
    const client = this.clients.get(hostname);
    if (!client) throw new Error(`Agent ${hostname} not registered`);
    return client;
  }

  async healthCheckAll(): Promise<Map<string, boolean>> {
    const results = new Map<string, boolean>();
    await Promise.allSettled(
      Array.from(this.clients.entries()).map(async ([hostname, client]) => {
        try {
          await client.health();
          results.set(hostname, true);
        } catch {
          results.set(hostname, false);
        }
      })
    );
    return results;
  }
}
```

---

## 8. 安全建议

### 8.1 网络层安全

由于 mingyue agent 默认使用 HTTP（明文），在跨网络场景下建议：

- **局域网内部**：可直接使用 HTTP，但建议限制 `:7070` 端口仅局域网可访问（防火墙规则）
- **跨公网访问**：在 agent 前面部署 nginx/caddy 反向代理，启用 TLS（HTTPS）
- **建议的 nginx 配置**：

```nginx
server {
    listen 443 ssl;
    server_name agent.example.com;

    ssl_certificate /etc/ssl/certs/agent.crt;
    ssl_certificate_key /etc/ssl/private/agent.key;

    location /api/ {
        proxy_pass http://127.0.0.1:7070;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

### 8.2 API Key 安全

| 建议 | 说明 |
|------|------|
| 最小权限 | 前端只读功能使用 `viewer` key，写操作使用 `operator` key |
| 定期轮换 | 周期性使用 `mingyue auth keygen` 生成新密钥并用 `revoke` 撤销旧密钥 |
| 后端代理 | Web 前端不直接持有 key，由 Node.js/Go 后端服务代理转发 |
| 密钥分离 | 不同客户端（Web 前端、脚本、监控系统）使用不同 key，便于独立撤销 |

### 8.3 CIFS 凭据安全

在通过 API 挂载 CIFS 共享时，密码通过 HTTPS/JSON 传输，agent 会：
- 通过临时 credentials 文件将密码传递给 mount 命令
- **不会**将密码写入审计日志
- **不会**在 API 响应中回显密码

### 8.4 CORS 配置

当前 agent 未内置 CORS 支持。如需从浏览器直接调用，在 nginx 反代层添加：

```nginx
add_header 'Access-Control-Allow-Origin' 'https://your-frontend.example.com';
add_header 'Access-Control-Allow-Headers' 'Authorization, Content-Type';
add_header 'Access-Control-Allow-Methods' 'GET, POST, PUT, DELETE, OPTIONS';
```

---

## 9. 常见问题

**Q: 忘记了初始 admin API Key，如何恢复？**

A: 初始密钥只显示一次。若丢失，可在宿主机上通过 CLI 生成新密钥：
```bash
sudo mingyue auth keygen --role admin --subject "recovery-admin"
```

**Q: API 返回 401，但 Key 是对的？**

A: 最可能的原因是 agent 重启后未加载密钥（旧版本行为）。请确认 agent 版本 ≥ Phase 7，或手动检查：
```bash
sudo mingyue auth list  # 确认 key 在列表中
```

**Q: agent discover 找不到 agent？**

A: 检查以下几点：
1. agent 是否在运行：`mingyue agent status`
2. 防火墙是否放通 UDP `7071` 端口（多播）
3. 网络设备是否支持 IPv4 多播路由（部分 VPN 或 Docker 网络会阻断）

**Q: 如何监控 agent 是否在线？**

A: 定期轮询 `GET /api/v1/health`（无需认证）。HTTP 200 且 `status: "ok"` 表示在线。

**Q: 多个 mingyue agent 如何管理？**

A: 每个宿主机运行独立的 agent 实例，Web 前端维护一个「agent 地址 → API Key」的映射表。可用 `mingyue agent discover` 自动发现局域网内的所有 agent。

**Q: 文件内容为什么是 Base64？**

A: API 传输的是通用二进制内容（涵盖文本/图片/可执行文件），Base64 编码确保 JSON 传输无损。读取文本文件时用 `atob(content)` 解码；写入时用 `btoa(text)` 编码。

---

*本文档随 mingyue 版本迭代更新。完整 HTTP API 规范请参阅 [`specs/001-linux-ops-agent/contracts/api-routes.md`](../specs/001-linux-ops-agent/contracts/api-routes.md)，完整 OpenAPI 规范请参阅 [`docs/api/openapi.yaml`](api/openapi.yaml)。*
