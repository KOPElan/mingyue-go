<!-- Based on: https://github.com/github/awesome-copilot/blob/main/instructions/go.instructions.md -->
---
applyTo: "**/*.go"
description: "Go 语言开发规范"
---
# Go 语言开发规范

- 遵循 gofmt 格式化标准。
- 包结构应职责单一，避免循环依赖。
- 变量、函数、包命名采用小写+下划线或驼峰，保持简洁。
- 错误处理必须显式，优先返回 error。
- 优先使用标准库，外部依赖需在 go.mod 管理。
- 注释应简明扼要，导出类型/函数需有英文注释。
