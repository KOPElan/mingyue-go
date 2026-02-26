# ADR-004: 统一错误结构与错误码

**状态**: 已接受（Accepted）
**日期**: 2026-02-27
**决策者**: 开发团队

## 背景

mingyue-go 以 CLI 和 HTTP API 两种形态对外提供能力。两种形态的消费者对错误信息有不同但相关的需求：

- **运维工程师（CLI）**：需要清晰、可定位的错误描述，快速理解失败原因并采取行动；同时在脚本化场景中需要通过 `--json` 获取结构化错误，以便自动化处理。
- **平台集成工程师（API）**：需要机器可解析的错误码（用于 UI 展示逻辑分支）和人类可读信息（用于提示用户）；HTTP 状态码需与错误语义一致。
- **审计/安全人员**：需要错误信息包含足够的上下文（操作类型、目标、原因）用于审计，但不能泄露敏感信息（密码、密钥、完整配置内容）。

如果 CLI 和 API 使用不同的错误表示方式，将导致：
- 上层平台对接 CLI `--json` 与 API 响应时需要维护两套解析逻辑。
- 错误码含义不一致，同一操作失败在两个接口中语义模糊。
- 难以复用统一的错误处理测试策略。

## 决策

在 **`internal/errors/`** 包中定义统一的错误结构 `AppError`，CLI 与 API 均基于此结构生成面向用户的错误输出：

### 错误结构定义

```go
// AppError 是 mingyue-go 统一错误类型
type AppError struct {
    Code    ErrorCode `json:"code"`              // 机器可读错误码（字符串枚举）
    Message string    `json:"message"`           // 人类可读错误信息
    Details any       `json:"details,omitempty"` // 可选：结构化附加信息
    Cause   error     `json:"-"`                 // 原始错误（不序列化，不暴露给外部）
}
```

### 错误码约定

错误码采用 **`大写_下划线_域名前缀`** 格式的字符串枚举，例如：

| 错误码 | 含义 | HTTP 状态码 |
|--------|------|-------------|
| `NOT_FOUND` | 目标资源不存在（PID/路径/挂载点/共享） | 404 |
| `PERMISSION_DENIED` | 权限不足（需要 root/capabilities/更高角色） | 403 |
| `INVALID_ARGUMENT` | 请求参数非法 | 400 |
| `ALREADY_EXISTS` | 资源已存在（幂等语义下的冲突） | 409 |
| `RESOURCE_BUSY` | 目标资源忙（设备占用、文件锁等） | 409 |
| `INTERNAL_ERROR` | 内部错误（系统调用失败等非预期） | 500 |
| `TIMEOUT` | 操作超时 | 504 |
| `UNAUTHENTICATED` | 未提供认证凭据 | 401 |
| `PATH_TRAVERSAL` | 路径安全校验失败（目录穿越等） | 400 |
| `DEPENDENCY_MISSING` | 所需系统工具/服务不可用（如 smartctl） | 422 |

### CLI 输出映射

- **默认输出**（stderr）：格式化的人类可读错误信息，含错误码前缀：`Error [PERMISSION_DENIED]: 需要 CAP_SYS_ADMIN 权限才能执行挂载操作`
- **`--json` 输出**（stderr）：与 API 响应一致的 JSON 结构：
  ```json
  { "error": { "code": "PERMISSION_DENIED", "message": "需要 CAP_SYS_ADMIN 权限才能执行挂载操作" } }
  ```
- 错误退出码：任何错误均以非 0 退出码（`exit 1`）退出。

### API 响应映射

```json
HTTP 403
{
  "error": {
    "code": "PERMISSION_DENIED",
    "message": "需要 CAP_SYS_ADMIN 权限才能执行挂载操作",
    "details": { "required_capability": "CAP_SYS_ADMIN" }
  }
}
```

## 备选方案

| 方案 | 描述 | 拒绝原因 |
|------|------|----------|
| 仅使用 HTTP 状态码（无错误码） | API 错误仅靠 HTTP 状态码区分 | HTTP 状态码粒度不足（如 400 无法区分"参数非法"与"路径穿越"）；CLI 无法复用 |
| CLI 使用 Go `error` 字符串，API 使用自定义结构 | 两套错误表示 | 违背 CLI/API 语义一致性原则；上层平台需维护两套解析逻辑 |
| 使用数字错误码（如 1001、2003） | 数字编码的错误码 | 可读性差；不经查表无法理解含义；字符串枚举对调试和日志更友好 |
| 直接透传系统错误信息 | 将 `errno`/系统调用错误直接暴露 | 可能泄露系统内部信息；跨平台不一致；用户不友好 |

## 后果

### 正面影响
- CLI `--json` 输出与 API 响应在错误结构上完全对齐，上层平台可用同一套逻辑处理两种来源的错误。
- 字符串错误码语义自明，无需查表即可理解；易于在日志、监控告警中进行过滤与统计。
- `Cause` 字段不序列化，避免将底层系统错误细节泄露给外部调用者，同时保留内部调试信息。
- HTTP 状态码与错误码的映射表提供了明确的 API 语义契约，便于 OpenAPI 规范描述。

### 负面影响 / 权衡
- 统一错误结构需要所有 service/handler 层代码统一使用 `AppError`，而非直接返回标准 Go `error`，需要一定的适配工作。
- 错误码字符串枚举需要集中维护；随着功能扩展，需要有纪律地扩充而非随意添加。
- `Details` 字段的内容需要在文档（OpenAPI）中明确，否则仍可能造成接口不稳定。

### 关联
- ADR-002：service 层使用 `AppError` 统一处理错误
- ADR-003：API 错误响应结构（OpenAPI 中描述错误 schema）
- Constitution §I（错误语义一致）、§II（错误显式处理，不泄露敏感信息）
- spec.md FR-015、GH-015
- plan.md Phase 1（统一错误结构交付物）
