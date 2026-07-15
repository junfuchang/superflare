# SuperFlare fnOS 原生应用打包说明

这个目录存放 SuperFlare 的 fnOS 原生应用打包文件。

## 打包模型

- 包根目录：`fnapp/superflare`
- 包类型：fnOS 原生应用
- 运行模型：原生 Go 二进制，不依赖 Docker
- 不可变程序：`app/server/superflare`
- 打包默认配置：`app/server/defaults`
- 当前配置存储：fnOS 可写的 `etc/`
- 可变运行数据：fnOS 可写的 `var/`
- 二进制工作目录：`etc/`

SuperFlare 会从工作目录读取 `config.yml`、`apps.yml`、`bookmarks.yml`、`ports.yaml`、`.env` 和 `var/`。其中 `.env` 用于端口、登录和 Cookie 等启动项。fnOS 会将不可变程序文件与可写的 `etc/`、`var/` 分离存放，生命周期脚本因此维护以下运行结构：

- `etc/` 存放 `.env`、`config.yml`、`apps.yml`、`bookmarks.yml`、`ports.yaml`
- `var/` 存放上传、缓存、pid、日志和其他可变运行数据
- `etc/` 作为实际工作目录
- `etc/var -> var/` 是生命周期脚本管理的运行数据软链接
- `var/runtime/` 仅作为旧版布局兼容入口；真实目录中的旧配置会按安全规则迁移或保留

这样就不需要大范围改造现有代码路径，也能让当前代码在 fnOS 上正常运行。

## 配置保留保证

安装、恢复时重放的安装回调以及升级回调都把现有配置视为用户数据。当前 `etc/` 中的以下五个文件会被保留：

- `.env`
- `config.yml`
- `apps.yml`
- `bookmarks.yml`
- `ports.yaml`

生命周期初始化只补齐缺失的文件和必需键，不会用包内默认值覆盖已经存在的文件或值。安装向导的用户名和密码只会应用于真正的首次安装：当前 `etc/` 与真实的旧版 `var/runtime/` 中都没有上述任一配置文件。只要任一位置已有配置，即使只是部分配置，安装回调也会忽略向导中的替换凭据，并只修复缺失项。

旧版真实目录中的配置仅在当前目标文件缺失时迁移。如果当前 `etc/` 与旧版 `var/runtime/` 中存在同名冲突文件，两份内容都会保留并记录冲突；软链接形式的旧版目录不会被当作配置目录遍历。若 `etc/var` 是真实文件或目录，生命周期脚本会先将它移动到首个可用的 `var.pre-superflare-link`、`var.pre-superflare-link.1` 等保留路径，再创建受管的 `etc/var -> var/` 链接，已有的同名保留路径也不会被覆盖。

卸载回调只会移除目标精确指向当前 fnOS `var/` 的受管 `etc/var` 软链接。五个配置文件、真实的 `etc/var`、保留路径、旧版配置以及操作员创建的其他软链接都不会被递归删除。必需的配置读取、写入、迁移或链接操作一旦失败，相关生命周期或配置回调会返回失败状态，不会在配置更新未完成时报告成功。

## 构建

在仓库根目录执行：

```powershell
powershell -ExecutionPolicy Bypass -File .\fnapp\build-superflare-fpk.ps1
```

构建脚本会依次执行：

1. 运行 `go run build/build.go`
2. 运行 `go test ./... -count=1`
3. 运行 `fnapp/test-lifecycle-config-preservation.ps1`，验证安装、恢复、升级和卸载不会破坏持久化配置
4. 将 `config/defaults/` 下的四个 YAML 模板同步到 `app/server/defaults/`，并重建打包源目录中的空 `etc/`、`var/`
5. 交叉编译 Linux `amd64` 版本的 `superflare`
6. 执行 `fnpack build`

配置保留测试在同步默认文件和重置打包源运行目录之前执行；测试失败会立即中止构建，不会继续生成 FPK。仓库级脚本检查 `tools/test-all-scripts.ps1` 也运行同一测试，因此破坏性更新回归会同时阻断常规脚本检查和打包流程。

构建结果：

- `fnapp/superflare/superflare.fpk`

## 运行默认值

- 端口：`3636`
- 登录：默认启用
- 首次安装向导的用户名和密码初始值为 `admin` / `admin`，且只会写入真正的首次安装；安装后建议尽快修改
- 编辑页：默认启用
- fnOS 权限：`run-as: root`，用于让端口页在 NAS 上读取完整的系统端口归属名称
- fnOS 应用设置：`系统设置 -> 应用设置` 现已支持修改 `FLARE_USER` / `FLARE_PASS`，会持久化到 `etc/.env` 并通过自动重启生效

## 说明

- 这个包明确以 fnOS 原生运行方式为目标，不依赖 Docker。
- 打包源目录的 `etc/`、`var/` 保持为空；用户配置只在安装后的 fnOS 持久化目录中初始化，不会嵌入 FPK。
