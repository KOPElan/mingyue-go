<!-- Based on: https://github.com/github/awesome-copilot/blob/main/instructions/testing.instructions.md -->
---
applyTo: "**/*.go"
description: "Go 测试规范"
---
# Go 测试规范

- 所有导出函数/方法应有对应单元测试。
- 测试文件以 _test.go 结尾，使用标准 testing 包。
- 测试用例应覆盖常见边界和异常情况。
- 推荐使用表驱动测试模式。
- 测试代码应简洁明了，便于维护。
