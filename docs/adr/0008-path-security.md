# ADR-008: 路径安全与目录穿越防护

**状态**: 已接受（Accepted）
**日期**: 2026-02-27
**决策者**: 开发团队

## 背景

mingyue-go 的文件管理功能接受来自 CLI 参数或 API 请求中的路径输入，对文件系统执行读取、写入、删除、移动、复制等操作。这类功能面临以下安全风险：

- **目录穿越（Path Traversal）**：恶意输入如 `../../../etc/passwd`、`/safe/path/../../etc/shadow` 等，通过相对路径跳出预期操作目录，访问或修改系统关键文件。
- **绝对路径越权**：用户直接指定 `/etc/`、`/proc/`、`/sys/` 等危险系统路径，绕过目录限制。
- **符号链接攻击（Symlink）**：操作路径本身安全，但目标是指向危险位置的符号链接。
- **空字节注入**：路径中包含 `%00`（null byte）等特殊字符，在某些底层函数中截断路径。

如果不对路径输入进行严格校验，文件管理 API 可能成为对整个 Linux 文件系统的未受控访问通道，造成严重的系统安全事故。

## 决策

在 **`internal/service/file/`** 包中集中实现路径安全校验逻辑，所有文件操作在执行前**必须**通过路径安全校验：

### 校验策略

#### 1. 路径规范化

所有输入路径在使用前必须经过规范化处理：

```go
import "path/filepath"

func sanitizePath(input string) (string, error) {
    // 1. 清理路径（解析 . 和 ..，去除多余斜杠）
    cleaned := filepath.Clean(input)
    // 2. 转换为绝对路径
    abs, err := filepath.Abs(cleaned)
    if err != nil {
        return "", AppError{Code: INVALID_ARGUMENT, ...}
    }
    return abs, nil
}
```

#### 2. 白名单根目录策略

文件操作只允许在预设的**根目录白名单**内进行，默认白名单示例：

```
/home/        # 用户主目录
/data/        # 数据目录
/mnt/         # 挂载目录
/srv/         # 服务数据目录
/tmp/         # 临时文件目录（可限制）
```

校验规则：规范化后的绝对路径必须以白名单中的某个前缀开头：

```go
func isWithinAllowedRoot(path string, allowedRoots []string) bool {
    for _, root := range allowedRoots {
        if strings.HasPrefix(path, filepath.Clean(root)+"/") || path == filepath.Clean(root) {
            return true
        }
    }
    return false
}
```

#### 3. 危险路径拒绝列表

以下路径前缀**始终拒绝**，不受白名单配置影响：

```
/proc/        # 内核虚拟文件系统
/sys/         # 设备/内核参数
/dev/         # 设备文件
/boot/        # 引导文件
/etc/         # 系统配置（文件管理功能不允许操作；共享配置变更由专用 service 层负责）
/root/        # root 用户主目录（默认拒绝）
```

#### 4. 符号链接处理

在执行文件操作前，通过 `os.Lstat()` 而非 `os.Stat()` 检测路径，并对符号链接的解析目标再次进行路径安全校验：

```go
info, err := os.Lstat(path)
if err == nil && info.Mode()&os.ModeSymlink != 0 {
    resolved, err := filepath.EvalSymlinks(path)
    if err != nil || !isWithinAllowedRoot(resolved, allowedRoots) {
        return AppError{Code: PATH_TRAVERSAL, Message: "符号链接目标超出允许范围"}
    }
}
```

#### 5. 错误响应

路径安全校验失败统一返回 `AppError{Code: PATH_TRAVERSAL}`，HTTP 状态码 400，**不**在错误信息中透露被拒绝的具体路径（防止路径探测）：

```json
{ "error": { "code": "PATH_TRAVERSAL", "message": "请求路径不在允许的操作范围内" } }
```

### 配置管理

允许根目录白名单可在 `/etc/mingyue/config.yaml` 中配置：

```yaml
file_manager:
  allowed_roots:
    - /home
    - /data
    - /mnt
```

管理员通过配置扩展允许范围；危险路径拒绝列表为硬编码，不可通过配置覆盖。

## 备选方案

| 方案 | 描述 | 拒绝原因 |
|------|------|----------|
| 不进行路径校验 | 直接使用用户输入路径 | 存在严重的目录穿越安全漏洞，不可接受 |
| 仅过滤 `../` 字符串 | 简单字符串过滤 | 绕过方式多样（URL 编码 `%2E%2E%2F`、多重编码等），不可靠 |
| 全量 chroot 隔离 | 以 `chroot` 限制进程文件系统视图 | 需要 root 权限创建 chroot 环境；增加部署复杂度；共享管理等功能需要访问系统路径，与 chroot 冲突 |
| 仅依赖 OS 权限 | 以操作系统文件权限控制访问 | 守护进程以特定权限运行时，OS 权限可能允许访问不该访问的路径；不提供应用层防御纵深 |

## 后果

### 正面影响
- `filepath.Clean` + 白名单校验在 Go 标准库基础上实现，无外部依赖，可靠性高。
- 路径安全校验集中在 `internal/service/file/` 包，避免在多处散落校验逻辑导致遗漏。
- 统一的 `PATH_TRAVERSAL` 错误码使测试可以精确验证拒绝策略（表驱动测试覆盖各类危险输入）。
- 危险路径硬编码拒绝列表提供了不依赖配置的安全基线，即使配置错误也不会暴露关键系统路径。

### 负面影响 / 权衡
- 白名单策略要求管理员在安装时正确配置 `allowed_roots`，否则可能导致合法操作被拒绝。
- 符号链接双重校验增加了少量 `stat` 系统调用开销，对高频目录列表操作有轻微性能影响。
- 共享管理（`/etc/samba/`、`/etc/exports`）等需要访问系统配置路径的操作，必须通过专用 service 层（`internal/service/share/`）而非通过文件管理 API，需要在文档中明确说明此边界。
- 路径校验错误信息不透露具体被拒绝的路径，提升了安全性，但可能给开发者调试带来一定不便。

### 关联
- ADR-005：最小权限原则（路径安全是文件操作权限控制的应用层防御）
- ADR-006：路径安全校验失败记录审计日志（`PATH_TRAVERSAL` 类事件）
- spec.md FR-011、GH-009
- plan.md Phase 4（文件管理路径安全校验实现）、DoD（路径穿越测试用例）
