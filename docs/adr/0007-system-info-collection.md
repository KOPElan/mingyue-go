# ADR-007: 系统信息采集方案选型（/proc + gopsutil）

**状态**: 已接受（Accepted）
**日期**: 2026-02-27
**决策者**: 开发团队

## 背景

mingyue-go 需要从 Linux 主机采集以下系统信息：

- **CPU**：使用率、核心数、频率。
- **内存**：总量、已用、可用、缓存。
- **磁盘**：各挂载点的总量、已用、可用、IO 统计。
- **进程**：PID、名称、状态、CPU 占用、内存占用、所属用户、启动时间等。
- **网络接口**（P1）：接口列表、地址、发送/接收字节数。

这些信息主要来源于 Linux 的 `/proc` 虚拟文件系统和 `/sys` 设备文件系统，但直接读取需要处理大量格式解析细节，且不同 Linux 内核版本间可能存在字段差异。

需要在以下三种主流方案中选择最适合本项目的信息采集策略：

1. **直读 `/proc` 与 `/sys`**：直接解析 `/proc/stat`、`/proc/meminfo`、`/proc/[pid]/stat` 等文件。
2. **使用 gopsutil 库**：[github.com/shirou/gopsutil](https://github.com/shirou/gopsutil) 是 Go 生态中最主流的跨平台系统信息采集库，封装了 `/proc` 读取细节。
3. **调用系统命令**：通过 `os/exec` 执行 `top`、`ps`、`df`、`free` 等命令并解析输出。

## 备选方案对比

| 维度 | 直读 /proc | gopsutil | 系统命令（exec） |
|------|-----------|----------|-----------------|
| 开发成本 | 高（需自行解析每个文件格式） | 低（直接调用 API） | 中（需解析命令输出） |
| 性能 | 最优（无额外进程开销） | 优（库内封装 /proc 读取，无进程开销） | 差（每次采集需 fork 子进程） |
| 可维护性 | 低（内核版本差异需自行处理） | 高（库维护者跟进内核变更） | 低（命令输出格式跨发行版不稳定） |
| 依赖 | 无外部依赖 | 需引入第三方库 | 依赖系统命令存在 |
| 覆盖范围 | 完全可控 | 覆盖大多数常见指标；特殊字段需补充 /proc 直读 | 受限于命令支持 |
| 测试友好性 | 可 mock 文件系统 | 可 mock 接口 | mock 较复杂（需模拟子进程） |
| 跨发行版兼容 | 需自行适配 | 已处理主流发行版差异 | 受命令参数兼容性影响 |

## 决策

采用 **gopsutil 为主、必要时直读 `/proc` 为辅** 的混合策略：

### 主要采集方案：gopsutil

使用 `github.com/shirou/gopsutil/v3` 采集以下指标：

```go
import (
    "github.com/shirou/gopsutil/v3/cpu"
    "github.com/shirou/gopsutil/v3/mem"
    "github.com/shirou/gopsutil/v3/disk"
    "github.com/shirou/gopsutil/v3/process"
    "github.com/shirou/gopsutil/v3/net"
)
```

- **CPU**：`cpu.Percent()`、`cpu.Info()`
- **内存**：`mem.VirtualMemory()`
- **磁盘**：`disk.Partitions()`、`disk.Usage()`
- **进程**：`process.Processes()`、单进程 `p.CPUPercent()` / `p.MemoryInfo()`
- **网络（P1）**：`net.Interfaces()`

### 补充方案：直读 `/proc`

对于 gopsutil 未覆盖或需要更精细控制的场景，通过 `pkg/linux/proc.go` 直读 `/proc`：

- 挂载信息：读取 `/proc/mounts`（更可靠，避免 gopsutil 挂载解析的边缘情况）。
- 特殊进程属性：如 `/proc/[pid]/cmdline`（完整命令行）、`/proc/[pid]/environ`（谨慎使用，安全敏感）。
- 内核参数或特定 `/sys` 路径（如 block 设备信息）。

### 明确排除：系统命令调用（用于信息采集）

对于**监控类只读采集**，不使用 `exec.CommandContext` 调用 `top`/`ps`/`df`/`free` 等命令。  
`exec.CommandContext` 仅用于必须通过系统命令完成的变更类操作（`mount`/`umount`/`smartctl`/`smbd`/`exportfs` 等）。

### 性能约定

- 高频采集接口（如 `/api/v1/system/overview`）在 service 层实现轻量采样，避免每次请求都读取全量数据。
- gopsutil 的 CPU 使用率采集需要间隔时间采样，service 层需正确处理此语义（例如缓存上次采集结果）。
- 所有采集调用通过 `context` 传递超时，避免在慢速系统或 `/proc` 读取阻塞时挂死请求。

## 后果

### 正面影响
- gopsutil 显著降低开发成本，无需为每个 `/proc` 文件格式编写解析器。
- gopsutil 已在业界广泛使用（Prometheus node_exporter、Telegraf 等均基于此），可靠性有保障。
- 混合策略保留了对特殊 `/proc` 路径的直读能力，不受 gopsutil API 覆盖范围限制。
- 统一避免系统命令采集，消除了 fork 开销与命令输出格式不稳定的风险。

### 负面影响 / 权衡
- 引入 `gopsutil/v3` 外部依赖，增加了 go.mod 的依赖项；需关注其安全公告与版本更新。
- gopsutil 对某些 Linux 特定特性（如 cgroup v2 内存统计）的支持可能滞后于内核特性，届时需要补充直读 `/proc`。
- gopsutil 默认适配多平台（含 macOS/Windows），但项目仅需 Linux 支持；多余的平台适配代码会引入但不影响功能。
- `/proc/[pid]/environ` 等敏感路径需要在 `pkg/linux/proc.go` 中明确注释访问策略，防止误用。

### 关联
- ADR-005：监控采集不需要特殊权限（只读 /proc 普通用户可访问）
- PRD §8.1 集成点（Linux 系统信息来源选型）、§8.3 可扩展性与性能
- spec.md FR-003、FR-004
- plan.md Technical Context（Primary Dependencies：gopsutil）、Phase 2
