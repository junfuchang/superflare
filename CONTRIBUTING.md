# Contributing to SuperFlare

本文档只描述当前 `superflare` 仓库的协作约定。

## 基本原则

- 以当前代码行为为准，不引用已删除的旧截图、旧示例或旧部署文档作为判断依据
- 默认围绕原生运行方式开发和验证
- Docker 相关内容只保留兼容性说明，不作为主线能力设计前提
- 变更应尽量保持小而清晰，避免顺手重构无关模块

## 提交前检查

在仓库根目录执行：

```bash
go run build/build.go
go test ./... -count=1
go build ./...
```

## 代码风格

1. 所有 Go 代码使用 `gofmt -s`。
2. 遵循 [Effective Go](https://go.dev/doc/effective_go) 与 [Go Code Review Comments](https://github.com/golang/go/wiki/CodeReviewComments)。
3. 注释只写必要信息，重点说明意图、边界和兼容性约束。
4. 新增能力优先复用现有结构，不额外引入“万能 helper”包。
5. 测试应能直接通过 `go test` 运行，不依赖额外测试框架。

## 运行时文件

下列文件或目录属于本地运行数据，不应提交到仓库：

- `config.yml`
- `apps.yml`
- `bookmarks.yml`
- `ports.yaml`
- `var/`

如果页面、配置模型或编辑器行为发生变化，应同步更新：

- `README.md`
- `docs/overview.md`
- `docs/config-reference.md`
- 相关截图或基线报告
