# Phase 1 Data Model: Linux 系统操作代理

> 本文定义对外契约与内部领域层应对齐的数据模型（不包含具体实现代码）。

## 1. 通用结构

### 1.1 ErrorResponse（统一错误结构）

- `error.code`：稳定错误码（例如 `AUTH_REQUIRED`、`FORBIDDEN`、`NOT_FOUND`、`INVALID_ARGUMENT`、`TIMEOUT`、`INTERNAL`）
- `error.message`：面向用户的可读信息
- `error.details`：可选的结构化详情（不含敏感信息）
- `request_id`：可选，用于链路定位

### 1.2 Pagination

- `page.limit` / `page.offset` 或 `page.cursor`
- 对列表型接口（进程、文件列表、共享列表、挂载列表）必须支持“限量/分页/最大上限”。

## 2. 监控与进程

### 2.1 HostSnapshot

- `timestamp`：采集时间
- `cpu`：总体利用率、负载（如有）
- `memory`：总量/已用/可用
- `disk`：总量/已用/可用（可按挂载点汇总或总体）

### 2.2 Process

- `pid`：进程 ID
- `name`：进程名
- `user`：所属用户（如可获取）
- `cpu_percent` / `mem_percent`：占用
- `state`：运行状态（运行/睡眠/僵尸等，枚举化）
- `started_at`：启动时间（如可获取）

## 3. 磁盘与挂载

### 3.1 Mount

- `mount_point`：挂载点
- `source`：源（设备/远端路径）
- `fs_type`：文件系统类型（ext4/xfs/cifs/nfs 等）
- `options`：关键参数摘要（不包含敏感信息）
- `read_only`：是否只读

### 3.2 MountRequest（创建挂载）

- `mount_point`
- `source`
- `fs_type`
- `options`：可选
- `credentials_ref`：可选，引用凭据（避免明文回显；具体策略在契约中定义）

### 3.3 DiskHealth（SMART）

- `device`：设备标识
- `health`：健康状态（OK/WARN/FAIL/UNKNOWN）
- `collected_at`
- `attributes`：关键属性列表（id/name/value/raw 等）

## 4. 文件管理

### 4.1 FileEntry

- `path`
- `type`：file/dir/link
- `size`
- `mode`：权限（符号或数值表示，契约统一）
- `owner`/`group`
- `mtime`/`ctime`（如可获取）

### 4.2 PathPolicy（路径安全策略）

- 明确允许/禁止的根路径（可配置）
- 拒绝目录穿越与不合法路径输入
- 对符号链接策略需明确：是否跟随、如何限制越界

## 5. 共享管理

### 5.1 Share

- `type`：samba/nfs
- `name`
- `path`
- `access`：访问控制摘要（只读/读写、允许来源/用户摘要）
- `status`：enabled/disabled/unknown

### 5.2 ShareChange

- `desired_state`：创建/更新/删除
- `payload`：共享配置变更内容（最小可用集，后续扩展字段需向后兼容）

## 6. 审计

### 6.1 AuditEvent

- `timestamp`
- `source`：cli/api + 调用方摘要
- `action`：枚举（KILL_PROCESS、MOUNT、UMOUNT、WRITE_FILE、SHARE_UPDATE 等）
- `target`：对象摘要（pid、path、mount_point、share_name 等）
- `result`：success/failure
- `error_code`：失败时填充

## 7. 状态机与验证规则（摘要）

- 挂载：`absent -> present -> absent`（必须定义幂等行为与“已存在/不存在”的错误语义）
- 共享：`absent -> present -> updated -> absent`（变更失败的恢复策略必须明确）
- 高危操作：必须校验权限与审计写入路径；敏感字段不得进入日志或响应。
