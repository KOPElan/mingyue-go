# ADR-003: 采用 RESTful HTTP+JSON API 并提供 OpenAPI 规范

**状态**: 已接受（Accepted）
**日期**: 2026-02-27
**决策者**: 开发团队

## 背景

mingyue-go 作为 Linux 主机管理 agent，需要为上层 Web 可视化平台或自动化工具提供标准化的程序可调用接口。平台集成工程师的核心诉求包括：

- 通过稳定的接口获取监控数据并发起管理动作。
- 能够自动生成客户端代码，减少手工对接成本。
- 错误响应结构稳定，便于 UI 层统一渲染与处理。
- 有版本化保障，小版本改动不破坏已有集成。

需要选择一种 API 风格与描述规范，使其既易于实现（Go 生态成熟）、易于消费（HTTP 客户端通用），又能提供可生成客户端的机器可读文档。

## 决策

采用 **RESTful HTTP+JSON API**，并以 **OpenAPI v3 规范**作为标准交付物：

### API 设计约定

- **路径前缀**：所有端点使用 `/api/v1/` 前缀，为未来版本演进（`/api/v2/`）预留空间。
- **HTTP 动词语义**：`GET`（只读查询）、`POST`（创建/执行动作）、`PUT`（全量更新）、`PATCH`（部分更新）、`DELETE`（删除/取消）。
- **统一响应结构**：
  ```json
  // 成功响应
  { "data": { ... }, "meta": { "timestamp": "..." } }

  // 错误响应（见 ADR-004）
  { "error": { "code": "MOUNT_NOT_FOUND", "message": "...", "details": { ... } } }
  ```
- **版本化承诺**：`/api/v1/` 内不做破坏性变更；新字段以可选方式添加；破坏性变更升级到 `/api/v2/`。
- **HTTP 框架**：使用 `github.com/gin-gonic/gin` 或 Go 标准库 `net/http` + `mux`（具体在实现阶段确认），通过 `context` 传递超时与取消。

### OpenAPI 规范交付物

- 仓库中提供 `docs/api/openapi.yaml`，覆盖 v1 全量 API 端点。
- 规范包含：认证方式、请求/响应结构、错误结构、端点描述。
- CI 流水线（`.github/workflows/openapi-sync.yaml`）在每次 PR 合并时校验 API 路由变更是否同步更新了 OpenAPI 文件，防止文档与实现长期漂移。

### 典型端点示例

```
GET  /api/v1/system/overview
GET  /api/v1/processes?limit=N&page=N
DELETE /api/v1/processes/:pid
GET  /api/v1/disks/mounts
POST /api/v1/disks/mounts
GET  /api/v1/shares
POST /api/v1/shares
```

## 备选方案

| 方案 | 描述 | 拒绝原因 |
|------|------|----------|
| gRPC + Protobuf | 使用 gRPC 提供强类型 API | 客户端需 gRPC 支持，Web 前端对接复杂（需 gRPC-Web）；对运维人员 curl 调试不友好 |
| GraphQL | 按需查询图结构 API | 实现复杂度显著高于 REST；对于系统管理类"命令式操作"语义不自然；Go 生态 GraphQL 库成熟度不及 REST |
| 私有二进制协议 | 自定义 TCP 协议 | 无标准工具支持；文档与客户端生成成本极高；违背"标准化"目标 |
| REST 但不提供 OpenAPI | 仅实现 REST，无规范文档 | 平台集成工程师无法自动生成客户端；接口漂移无法及时发现（违背 GH-011、PRD §4） |

## 后果

### 正面影响
- HTTP+JSON 具有最广泛的客户端生态支持，任意语言均可轻松对接。
- `/api/v1/` 路径前缀提供清晰的版本边界，向后兼容有制度保障。
- OpenAPI 规范支持自动生成客户端代码（openapi-generator 等工具），大幅降低平台集成成本。
- RESTful 语义与 HTTP 状态码的标准化使错误处理模式统一，降低 UI 层处理复杂度。
- CI 同步机制（openapi-sync.yaml）从流程上保障文档与实现一致，减少人工维护疏漏。

### 负面影响 / 权衡
- REST 对于复杂查询场景（如多维度过滤、嵌套资源）表达能力有限，需通过约定的查询参数弥补。
- OpenAPI 规范需要随代码同步维护，前期需要投入额外的文档编写成本。
- HTTP 的无状态特性对于长耗时操作（如大规模文件复制）需要额外的异步任务机制，当前 v1 范围内通过超时/取消机制缓解。

### 关联
- ADR-001：守护进程形态提供 HTTP API
- ADR-004：统一错误结构（API 错误响应规范）
- PRD §4 功能需求：标准 API 与 OpenAPI 交付物（P0）
- spec.md FR-016、GH-011
- plan.md Phase 6（OpenAPI 规范 + CI 文档同步）
