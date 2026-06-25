# SuperFlare fnOS 原生应用打包说明

这个目录存放 SuperFlare 的 fnOS 原生应用打包文件。

## 打包模型

- 包根目录：`fnapp/superflare`
- 包类型：fnOS 原生应用
- 运行模型：原生 Go 二进制，不依赖 Docker
- 二进制工作目录：`var/runtime`

SuperFlare 当前会从工作目录读取 `config.yml`、`apps.yml`、`bookmarks.yml`、`ports.yaml`、`.env` 和 `var/`。其中 `.env` 只用于少量启动项，例如登录和 Cookie 配置。由于 fnOS 会将不可变程序文件与可写的 `etc/`、`var/` 分离存放，所以生命周期脚本会补齐以下运行结构：

- `etc/` 存放 `.env`、`config.yml`、`apps.yml`、`bookmarks.yml`、`ports.yaml`
- `var/` 存放上传、缓存、pid、日志和其他可变运行数据
- `var/runtime/` 作为实际工作目录
- `var/runtime/var -> var/`
- `var/runtime/*.yml` 与 `var/runtime/.env` 软链接到 `etc/`

这样就不需要大范围改造现有代码路径，也能让当前代码在 fnOS 上正常运行。

## 构建

在仓库根目录执行：

```powershell
powershell -ExecutionPolicy Bypass -File .\fnapp\build-superflare-fpk.ps1
```

构建脚本会依次执行：

1. 运行 `go run build/build.go`
2. 运行 `go test ./... -count=1`
3. 交叉编译 Linux `amd64` 版本的 `superflare`
4. 将当前根目录下的 `config.yml`、`apps.yml`、`bookmarks.yml`、`ports.yaml` 复制为应用默认配置
5. 执行 `fnpack build`

构建结果：

- `fnapp/superflare/superflare.fpk`

## 运行默认值

- 端口：`3636`
- 登录：默认启用
- 默认用户名和密码来自打包时的 `config.yml` 默认值，目前为 `admin / admin`
- 编辑页：默认启用
- fnOS 权限：`run-as: root`，用于让端口页在 NAS 上读取完整的系统端口归属名称
- fnOS 应用设置：`系统设置 -> 应用设置` 现已支持修改 `FLARE_USER` / `FLARE_PASS`，会持久化到 `etc/.env` 并通过自动重启生效

## 说明

- 这个包明确以 fnOS 原生运行方式为目标，不依赖 Docker。
- 升级时会优先保留现有 `etc/` 和 `var/` 数据，默认文件只会在缺失时补入。
