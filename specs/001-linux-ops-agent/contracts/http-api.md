# Contracts: HTTP API v1（草案）

> 目的：明确 v1 API 的端点、语义、错误结构与鉴权方式；实现阶段需同步到 OpenAPI 文件。

## Base

- Base path: `/api/v1`
- Content-Type: `application/json`

## Auth（最小可用，后续可增强）

- v1 决策：使用 API Key
  - Header: `X-API-Key: <key>`
  - 默认策略：除 `/health` 外，其它端点均需要鉴权
- 授权（实现阶段落地）：角色 viewer/operator/admin（只读/变更类分层），并确保错误码语义一致（`AUTH_REQUIRED` / `FORBIDDEN`）

## Error model

- 统一返回 `ErrorResponse`（见 data-model.md）
- 常用错误码：`AUTH_REQUIRED`、`FORBIDDEN`、`NOT_FOUND`、`INVALID_ARGUMENT`、`TIMEOUT`、`INTERNAL`

## Endpoints（v1 必做）

### Health

- `GET /health`
  - 200: 服务存活

### Monitoring

- `GET /host/snapshot`
  - 200: `HostSnapshot`

- `GET /processes?limit=&offset=`
  - 200: `Process[]` + pagination

- `GET /processes/{pid}`
  - 200: `Process`
  - 404: NOT_FOUND

- `POST /processes/{pid}/kill`
  - 200: 操作结果（建议包含审计事件 id 或 request_id）
  - 403: FORBIDDEN

### Disk & Mount

- `GET /mounts?limit=&offset=`
  - 200: `Mount[]` + pagination

- `POST /mounts`
  - 200/201: 挂载结果
  - 409: 已挂载（或返回幂等成功）

- `DELETE /mounts`
  - body: 指定 mount_point/source
  - 200: 卸载结果

- `GET /disks/{device}/smart`
  - 200: `DiskHealth`
  - 501: 不支持或条件不足（以错误码表达）

### File

- `GET /fs/list?path=&limit=&offset=`
  - 200: `FileEntry[]` + pagination

- `GET /fs/stat?path=`
  - 200: `FileEntry`

- `POST /fs/write`
  - body: path + content（或上传机制，后续确定）
  - 403/400: 权限/参数错误

- `POST /fs/mkdir`
- `POST /fs/remove`
- `POST /fs/move`
- `POST /fs/copy`

### Shares（Samba/NFS）

- `GET /shares?type=&limit=&offset=`
  - 200: `Share[]` + pagination

- `POST /shares`
- `PUT /shares/{name}`
- `DELETE /shares/{name}`

## Notes

- 所有列表接口必须支持 limit/offset 或 cursor；并对最大 limit 设置上限。
- 所有变更类接口必须：鉴权/授权 + 审计事件。
- CLI `--json` 输出需与 API schema 的核心字段对齐。
