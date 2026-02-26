<!-- Based on: https://github.com/github/awesome-copilot/blob/main/instructions/security.instructions.md -->
---
applyTo: "**/*.go"
description: "Go 安全规范"
---
# Go 安全规范

- 避免硬编码敏感信息，优先使用环境变量。
- 外部输入需严格校验，防止注入攻击。
- 依赖管理需定期 go mod tidy，关注安全公告。
- 日志中避免输出敏感数据。
