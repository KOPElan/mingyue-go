# Feature Specification: Linux 系统操作代理（CLI + 标准 API）

**Feature Branch**: `001-linux-ops-agent`  
**Created**: 2026-02-27  
**Status**: Draft  
**Input**: User description: "本项目旨在用go实现一个linux系统操作代理，可以通过cli以及api的形式提供：系统监控及进程管理（cpu、内存、磁盘、进程）、文件管理、共享管理（samba、nfs）、网络管理、磁盘管理（挂载/卸载磁盘、cifs/nfs挂载/卸载、SMART信息获取等）、用户权限/ACL等功能。使用户可以在终端以简单的命令完成对应的操作，同时也可以作为agent，通过api供web端（web端非本项目范围，本项目旨在提供标准API）调用实现可视化管理。"

## User Scenarios & Testing *(mandatory)*

<!--
  IMPORTANT: User stories should be PRIORITIZED as user journeys ordered by importance.
  Each user story/journey must be INDEPENDENTLY TESTABLE - meaning if you implement just ONE of them,
  you should still have a viable MVP (Minimum Viable Product) that delivers value.
  
  Assign priorities (P1, P2, P3, etc.) to each story, where P1 is the most critical.
  Think of each story as a standalone slice of functionality that can be:
  - Developed independently
  - Tested independently
  - Deployed independently
  - Demonstrated to users independently
-->

### User Story 1 - 查看主机状态与进程概览（CLI/API 一致） (Priority: P1)

运维人员或平台通过 CLI 或 HTTP+JSON API 获取主机的 CPU、内存、磁盘使用概览，以及进程列表/关键指标，用于快速定位资源异常。

**Why this priority**: 这是所有管理动作的前置能力，也是最小可用系统管理（MVP）中价值最高、风险最低的切片。

**Independent Test**: 在一台 Linux 测试主机上，通过 CLI 命令与 API 请求分别获取状态数据；验证输出字段完整、含义一致、错误场景可解释。

**Acceptance Scenarios**:

1. **Given** 目标主机可访问且服务可用，**When** 用户请求“系统概览”，**Then** 返回 CPU/内存/磁盘概览与时间戳，且响应结构稳定。
2. **Given** 目标主机可访问且服务可用，**When** 用户请求“进程列表”，**Then** 返回包含 PID、名称、资源占用等字段的列表，并支持分页/限制返回规模。
3. **Given** 服务不可用或守护进程未运行，**When** 用户通过 CLI 请求“系统概览”，**Then** CLI 给出明确提示并按既定策略降级（若不支持降级则清晰提示如何恢复）。
4. **Given** 用户仅具备只读权限，**When** 用户调用只读接口，**Then** 请求成功且不暴露敏感信息。

---

### User Story 2 - 磁盘与挂载管理（本地 + CIFS/NFS + SMART） (Priority: P1)

管理员通过 CLI 或 API 完成挂载/卸载操作，并可查询当前挂载点与磁盘健康信息（SMART），以支持存储接入与故障排查。

**Why this priority**: 磁盘与挂载是主机管理的高频场景，且直接影响业务可用性；v1 必做。

**Independent Test**: 在测试主机上，准备一个测试挂载点与可控的测试目标（本地设备或远端共享），分别验证“列出挂载”“挂载”“卸载”“SMART 查询”的成功与失败路径。

**Acceptance Scenarios**:

1. **Given** 用户具备管理员权限且提供有效参数，**When** 用户发起挂载操作，**Then** 挂载成功并可在“挂载列表”中看到目标挂载点。
2. **Given** 挂载点已存在且指向相同目标，**When** 用户重复发起挂载操作，**Then** 行为幂等（返回已挂载或成功且不产生额外副作用）。
3. **Given** 目标设备忙或权限不足，**When** 用户发起卸载操作，**Then** 返回可定位错误原因与可操作建议（例如需要更高权限或先释放占用）。
4. **Given** 缺少获取 SMART 所需的权限或条件，**When** 用户请求 SMART 信息，**Then** 返回明确的失败原因与建议，不返回误导性健康结论。

---

### User Story 3 - 文件管理（安全边界清晰） (Priority: P2)

用户通过 CLI 或 API 进行文件/目录的查询与基础操作（列出、查看元信息、创建/删除/移动/复制、读写），并确保不会因为路径输入导致越权或破坏系统关键文件。

**Why this priority**: 文件管理是常见运维需求，也是共享管理与权限治理的基础能力；v1 必做。

**Independent Test**: 在测试目录下，通过 CLI/API 执行文件操作并验证结果；对危险路径、非法路径与权限不足场景验证拒绝策略与错误响应。

**Acceptance Scenarios**:

1. **Given** 用户具备只读权限且目标路径存在，**When** 用户列出目录或查询元信息，**Then** 返回结果完整且不包含超出权限范围的信息。
2. **Given** 用户具备写权限且目标路径位于允许范围内，**When** 用户创建/删除/移动文件或目录，**Then** 操作成功且结果可被再次查询验证。
3. **Given** 用户请求访问危险路径或疑似目录穿越输入，**When** 用户发起文件操作，**Then** 系统拒绝并返回稳定错误码与解释信息。

---

---

### User Story 4 - 共享管理（Samba/NFS） (Priority: P2)

管理员通过 CLI 或 API 查询共享状态与配置摘要，并对共享进行创建/修改/删除，以满足团队协作与服务共享需求。

**Why this priority**: 共享能力属于 v1 必做范围，且直接影响多用户协作与数据交付。

**Independent Test**: 在测试环境中，针对一条共享配置执行“查询→创建/修改→验证→删除”的完整闭环，覆盖典型失败（配置无效、权限不足、服务不可用）。

**Acceptance Scenarios**:

1. **Given** 系统具备共享服务的基础条件且用户具备管理员权限，**When** 用户创建共享，**Then** 共享状态可被查询到且配置摘要与输入一致。
2. **Given** 用户修改共享配置，**When** 修改提交成功，**Then** 后续查询返回的配置摘要反映最新状态，且变更操作被审计记录。
3. **Given** 配置内容无效或服务不可用，**When** 用户提交变更，**Then** 返回可定位错误并说明恢复步骤（回滚或修复建议）。

---

### User Story 5 - 标准化 API 文档（OpenAPI/Swagger） (Priority: P3)

平台集成工程师获取 OpenAPI/Swagger 规范文件，并据此生成客户端或完成接口对接；规范与实际 API 行为保持一致。

**Why this priority**: 没有标准化接口规范，上层平台对接成本高且容易漂移；这是“标准 API”交付物的核心。

**Independent Test**: 使用 OpenAPI 文件在本地或 CI 中做静态校验，并通过选择若干关键端点执行冒烟测试，确认请求/响应结构与文档一致。

**Acceptance Scenarios**:

1. **Given** 项目发布了 v1 API，**When** 集成方获取 OpenAPI 规范，**Then** 规范包含认证方式、端点列表、请求/响应结构与错误结构。
2. **Given** API 返回错误，**When** 集成方按规范解析错误响应，**Then** 能稳定获取错误码与可读信息用于 UI 展示。

### Edge Cases

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right edge cases.
-->

- 权限不足：只读用户调用变更接口、或管理员能力缺失时如何返回？
- 守护进程不可用：CLI 调用 API 能否降级？若不能，提示是否明确？
- 输出规模：进程列表/目录列表很大时如何限制输出并保持响应可用？
- 并发与竞态：并发挂载/卸载、并发共享变更时如何避免状态不一致？
- 幂等：重复挂载、重复删除共享、重复删除文件时返回应可预期。
- 目标不存在：PID/路径/挂载点/共享不存在时的错误码与提示。
- 机密信息：远端挂载凭据、共享密钥等如何避免被回显或写入日志。

## Requirements *(mandatory)*

<!--
  ACTION REQUIRED: The content in this section represents placeholders.
  Fill them out with the right functional requirements.
-->

### Functional Requirements

- **FR-001**: 系统 MUST 同时提供 CLI 与 RESTful HTTP+JSON API，并保证两者对同一能力的行为一致。
- **FR-002**: 系统 MUST 支持两种运行方式：直接执行（无需常驻服务）与常驻守护进程（对外提供 API）。
- **FR-003**: 系统 MUST 提供系统监控能力：CPU、内存、磁盘使用概览，并提供可用于可视化展示的结构化输出。
- **FR-004**: 系统 MUST 提供进程查询能力：返回包含 PID、名称、资源占用等字段的进程列表与单进程详情。
- **FR-005**: 系统 MUST 提供受控的进程管理能力（至少包含终止进程），并在权限不足时拒绝并返回稳定错误结构。
- **FR-006**: 系统 MUST 提供挂载信息查询（当前挂载点/目标/类型/关键参数摘要）。
- **FR-007**: 系统 MUST 支持本地磁盘挂载与卸载，并对重复调用提供幂等或明确的“已存在/不存在”语义。
- **FR-008**: 系统 MUST 支持 CIFS 与 NFS 的挂载/卸载，并确保敏感字段（如凭据）不被回显或记录到日志中。
- **FR-009**: 系统 MUST 提供 SMART 信息获取能力；当条件不满足时 MUST 返回可解释错误（不得给出误导性健康结论）。
- **FR-010**: 系统 MUST 提供文件/目录查询与基础操作（列出、元信息、创建、删除、移动、复制、读写）。
- **FR-011**: 系统 MUST 对文件相关输入进行路径安全校验，并拒绝明显的目录穿越与危险路径访问。
- **FR-012**: 系统 MUST 提供共享管理能力，至少包含：查询共享状态与配置摘要、创建共享、修改共享、删除共享。
- **FR-013**: 系统 MUST 对“破坏性操作”（卸载磁盘、终止进程、共享变更、文件写操作等）记录审计事件（含时间、来源、对象、结果、失败原因）。
- **FR-014**: 系统 MUST 遵循最小权限原则：默认以最低权限运行，仅在必要场景使用更高权限或最小化的系统能力授权。
- **FR-015**: 系统 MUST 提供 API 访问控制：区分只读与变更能力，未授权请求 MUST 返回稳定的错误码与错误结构。
- **FR-016**: 系统 MUST 为所有 API 提供 OpenAPI/Swagger 规范文件作为交付物，并确保字段与行为长期可维护地保持一致。
- **FR-017**: 系统 MUST 对高开销只读接口提供输出规模控制（例如限制数量/分页），避免在大规模数据下不可用。
- **FR-018**: 系统 MUST 为可能长耗时的系统操作提供超时/取消语义，并在超时后保持系统状态可恢复（可重试或明确失败状态）。

*Assumptions (documented as defaults; can be refined later):*

- 该 agent 部署在被管理的 Linux 主机本机（非跨主机集中调度）。
- API 主要服务于内网/受控环境；认证方式将以“易集成、可轮换、可审计”为优先。
- v1 必做模块范围：监控+进程、磁盘管理、文件管理、共享管理；网络管理与权限/ACL 为后续优先级。

### Key Entities *(include if feature involves data)*

- **Host Snapshot**: 某一时刻的主机概览数据（CPU、内存、磁盘汇总、时间戳）。
- **Process**: 进程信息（PID、名称、状态、资源占用、所属用户等）。
- **Mount**: 挂载条目（挂载点、目标、类型、只读/读写、关键参数摘要）。
- **Disk Health**: 磁盘健康信息（健康状态、关键属性摘要、采集时间）。
- **File Entry**: 文件/目录条目（路径、类型、大小、权限、时间戳）。
- **Share**: 共享条目（类型 samba/nfs、共享名、路径、访问控制摘要、状态）。
- **Audit Event**: 审计事件（时间、来源、操作类型、目标对象、结果、错误码）。

## Success Criteria *(mandatory)*

<!--
  ACTION REQUIRED: Define measurable success criteria.
  These must be technology-agnostic and measurable.
-->

### Measurable Outcomes

- **SC-001**: 用户在首次接触时，能在 5 分钟内完成“查看主机概览 + 查看进程列表”的任务（CLI 或 API 任一方式）。
- **SC-002**: 在典型主机规模下，95% 的只读查询（概览/挂载列表/共享列表）可在 1 秒内返回。
- **SC-003**: 变更类操作的失败返回中，90% 以上可被用户据此采取下一步行动（例如权限不足、目标不存在、参数无效、资源忙）。
- **SC-004**: OpenAPI 规范覆盖 v1 全部对外 API 端点，并能支持集成方生成客户端并完成至少 3 个关键流程对接（概览、挂载、共享变更）。
- **SC-005**: 未授权用户无法成功执行任何变更类操作（通过权限测试验证），且所有变更尝试均产生可检索的审计记录。
