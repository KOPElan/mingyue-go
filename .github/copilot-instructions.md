# Project Guidelines

## 语言与文档约束
- 所有 AI 代理回复以中文为首选语言。
- 所有自动生成的文档均以中文为首选语言。

## Code Style
- 主要使用 Go 语言，遵循标准 Go 代码风格（gofmt）。
- 模块名为 `kopelan/mingyue-go`，Go 版本为 1.25.7。
- 代码应保持简洁、可读，优先使用标准库。

## Architecture
- 当前未检测到具体代码文件，建议后续补充架构说明。
- 推荐采用包结构组织代码，每个包职责单一。

## Build and Test
- 构建命令：
  ```sh
  go build ./...
  ```
- 测试命令：
  ```sh
  go test ./...
  ```
- 依赖管理：
  ```sh
  go mod tidy
  ```

## Project Conventions
- 遵循 Go Modules 规范进行依赖管理。
- 代码和文档应保持同步，建议补充 README.md。

## Integration Points
- 当前未检测到外部依赖，建议后续补充。

## Security
- 暂无特殊安全约定，后续如涉及敏感信息请使用环境变量管理。
