<!-- Based on: https://github.com/github/awesome-copilot/blob/main/instructions/performance.instructions.md -->
---
applyTo: "**/*.go"
description: "Go 性能优化规范"
---
# Go 性能优化规范

- 优先使用高效的数据结构和算法。
- 关注内存分配，避免不必要的拷贝。
- 并发场景下注意锁粒度和死锁风险。
- 使用 go test -bench 进行性能基准测试。
