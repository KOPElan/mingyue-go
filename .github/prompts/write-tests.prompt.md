---
agent: 'agent'
model: GPT-4.1
tools: ['codebase']
description: '生成 Go 单元测试用例'
---
你的目标是为指定的 Go 函数或模块生成标准单元测试用例。

请先询问需要测试的函数/模块名称和预期行为。

要求：
- 使用 testing 包
- 覆盖常见边界和异常情况
- 推荐表驱动测试
