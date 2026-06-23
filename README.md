# SuperFlare

SuperFlare 是一个以原生部署为优先的个人导航页项目。它以你当前已经多次定制过的 `flare` 代码为核心，合并了历史 `docker-flare` 仓库中的示例、截图和部分文档，作为后续持续演进的统一主仓库。

## 当前定位

- 主运行方式：原生二进制运行
- 后续封装方向：可继续对接 fnOS / 飞牛 NAS 应用形态
- Docker 相关内容：仅保留为兼容示例和历史资产，不作为默认部署方案

## 已合并内容

- `flare` 当前代码与本地已验证过的功能改动
- `docker-flare` 的历史截图、示例编排文件、部分说明文档
- 统一后的仓库名、模块路径、界面品牌标识

## 目录说明

```text
build/                  构建脚本与资源生成
cmd/                    启动参数、环境变量、版本信息
config/                 配置、数据模型、默认设置
docker/                 历史 Docker 构建脚本
docs/                   当前文档
docs/legacy-docker/     历史 Docker 使用文档
embed/                  前端模板与静态资源源文件
examples/               历史示例编排文件
fnApp/                  预留给后续 fnOS/fnApp 打包资产
internal/               主要业务代码
legacy/docker-flare/    原 docker-flare 合并留档
screenshots/            历史界面截图
```

## 本地开发

1. 进入仓库目录
2. 准备配置文件
3. 生成嵌入资源
4. 启动服务

```bash
go run build/build.go
go run .
```

默认运行目录下会读取：

- `config.yml`
- `apps.yml`
- `bookmarks.yml`
- `ports.yaml`

这些文件属于本地运行数据，默认不纳入 Git 版本控制。

## 常用命令

```bash
go test ./... -count=1
go build ./...
go run build/build.go
```

## 历史兼容说明

- `examples/` 和 `legacy/docker-flare/` 中仍保留了 Docker 时代的内容
- `docs/legacy-docker/` 仅用于回溯旧部署方式
- 后续若继续做飞牛 NAS 应用封装，建议以 `fnApp/` 作为新打包入口，而不是继续围绕 Docker 目录组织

## 仓库

- GitHub: [junfuchang/superflare](https://github.com/junfuchang/superflare)
