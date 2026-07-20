# SuperFlare

SuperFlare 是一个轻量级、自托管的导航首页，基于 [soulteary/docker-flare](https://github.com/soulteary/docker-flare) 演进而来。它使用本地 YAML 保存导航数据，适合个人主页、家庭实验室和内网服务入口。

## 功能亮点

- 管理应用和普通书签；应用、书签都可标记为私有，普通书签可单独设为收藏。
- 书签描述会以提示信息展示；带 `subdir` 的应用可在子目录弹窗中按分类访问。
- 提供搜索、主题与背景等外观设置、多语言界面、在线编辑，以及配置备份和恢复。
- 可检查公开链接、发现本机端口并在端口页查看结果，便于整理服务入口。
- 站点图标支持显式图标、站点 favicon 和内置图标回退；外部图标请求可能因网络或目标站点限制失败，失败时会回退到可用的内置图标。

## 部署方式

| 方式 | 适用场景 | 入口 |
| --- | --- | --- |
| 源码运行 | 本地体验与开发 | 本 README 的“快速开始” |
| Docker | 容器化部署 | [`tools/docker/README.md`](tools/docker/README.md) |
| Linux 原生 | 二进制与 systemd | [`tools/linux/README.md`](tools/linux/README.md) |
| Windows | PowerShell 构建与后台运行 | `tools/windows/restart-superflare.ps1` |
| fnOS | NAS 原生 FPK | [`fnapp/README.md`](fnapp/README.md) |

## 快速开始

需要 Go `1.26.x`。在仓库根目录依次执行：

```bash
go run ./build/build.go
go run .
```

默认访问地址：

```text
http://127.0.0.1:3636/
```

首次启动会补齐缺失的运行配置；随后可登录管理或直接编辑本地 YAML 文件。

## 登录与管理入口

默认用户名和密码均为 `admin` / `admin`，首次登录后应立即修改。生产环境请配置稳定且足够随机的 `FLARE_COOKIE_SECRET`，避免重启、容器重建或密钥变更导致会话失效。

- `/settings`：登录、外观、搜索和其他设置。
- `/editor`：在线编辑应用、书签与配置。
- `/settings/ports`：查看端口发现结果。

修改用户名或密码会使当前会话失效，需要使用新凭据重新登录。禁用登录模式被视为受信任访问，私有项目仍会显示；请只在可信网络中关闭登录。

## 运行数据与升级

以下文件和目录是持久化运行数据：

- `config.yml`
- `apps.yml`
- `bookmarks.yml`
- `ports.yaml`
- `.env`（可选）
- `var/`

原生部署会从进程工作目录解析这些路径，因此二进制、配置和 `var/` 应放在同一运行目录。Docker 必须持久化 `/data`，其中包含上述所有数据；复用同一个 `/data` 后，镜像升级或容器重建不会覆盖已有配置。

fnOS 将配置保存在可写的 `etc/`，运行数据保存在 `var/`。升级时会保留已有配置，只补齐缺失文件或必要键，不会用包内默认值覆盖现有值。

## 配置示例

应用写入 `apps.yml`：

```yaml
# apps.yml
links:
  - name: NAS
    link: https://nas.example.com
    local_link: http://192.168.1.10:5000
    subdir: 存储服务
    private: true
```

普通书签和分类写入 `bookmarks.yml`：

```yaml
# bookmarks.yml
categories:
  - id: "1"
    title: 开发
links:
  - name: SuperFlare
    link: https://github.com/junfuchang/superflare
    category: "1"
    desc: 项目仓库
    favorite: true
    private: false
```

- `local_link`：优先用于内网或本地网络访问的地址。
- `subdir`：为项目指定子目录分类，并在首页弹窗中分组展示。
- `desc`：项目描述，会作为提示信息展示。
- `favorite`：将普通书签加入收藏区；该字段不用于应用。
- `private`：仅在已登录时显示的应用或书签；禁用登录时视为受信任访问，因此仍可见。

## 图标与网络访问

可为项目配置图标 URL 或内置 MDI 图标名；未配置时，SuperFlare 会尝试使用站点 favicon。图标请求、公开链接检查和页面访问都依赖当前网络条件及目标站点响应，无法保证外部资源始终可用。favicon 获取失败、缓存无效或浏览器加载错误时，界面会使用内置图标回退，避免导航项失去可识别的图标。

## 开发与校验

修改嵌入的静态资源或模板后，先生成资源，再运行测试和构建：

```bash
go run ./build/build.go
go test ./... -count=1
go build ./...
```

Windows 可额外执行仓库脚本校验：

```powershell
powershell -ExecutionPolicy Bypass -File .\tools\test-all-scripts.ps1
```

## 仓库结构

```text
build/      资源生成工具
config/     配置模型与默认配置
embed/      模板和静态资源
internal/   应用主体代码
fnapp/      fnOS 原生应用打包文件
tools/      Docker、Linux、Windows 等部署与校验脚本
var/        运行时可变数据
```

## 致谢

感谢 [soulteary/docker-flare](https://github.com/soulteary/docker-flare) 提供的开源基础，以及所有参与测试、反馈和维护的贡献者。
