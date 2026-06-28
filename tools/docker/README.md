# Docker 构建说明

这个目录存放 SuperFlare 的 Docker 兼容打包文件。

项目当前仍以原生二进制运行作为主要开发和部署方式。Docker 支持被集中放在这里，避免扩散到项目根目录。

## 文件说明

- `Dockerfile`：Linux 容器多阶段构建文件
- `docker-entrypoint.sh`：容器入口脚本，从 `/data` 目录运行 SuperFlare
- `build-image.ps1`：Windows PowerShell 构建脚本
- `build-image.sh`：POSIX Shell 构建脚本
- `docker-compose.yml`：基础 Compose 运行文件
- `docker-compose.host-proc.yml`：用于读取宿主机 `/proc` 的可选叠加文件

## 容器运行结构

容器内可写的运行时文件统一放在 `/data` 下：

- `/data/config.yml`
- `/data/apps.yml`
- `/data/bookmarks.yml`
- `/data/ports.yaml`
- `/data/.env`（可选）
- `/data/var/`

镜像本身不会打包你本地仓库根目录下的运行时数据文件。
镜像内只包含一份默认模板；容器启动时仅在 `/data` 中缺少同名文件时补齐，不会覆盖已经存在的配置。

## 构建

Windows PowerShell：

```powershell
.\tools\docker\build-image.ps1 -ImageName superflare -Tag latest
```

Linux / macOS：

```bash
chmod +x ./tools/docker/build-image.sh
./tools/docker/build-image.sh superflare latest
```

交叉构建示例：

```powershell
.\tools\docker\build-image.ps1 -ImageName superflare -Tag arm64 -Platform linux/arm64
```

```bash
PLATFORM=linux/arm64 ./tools/docker/build-image.sh superflare arm64
```

## 运行

基础 `docker run`：

```bash
docker run -d \
  --name superflare \
  -p 3636:3636 \
  -v superflare-data:/data \
  superflare:latest
```

基础 Docker Compose：

```bash
docker compose -f ./tools/docker/docker-compose.yml up -d
```

首次部署前建议先编辑 `tools/docker/docker-compose.yml`：

- `FLARE_DISABLE_LOGIN`：`false` 表示启用登录，`true` 表示关闭登录
- `FLARE_COOKIE_SECRET`：登录 Cookie 签名密钥，请替换为稳定且足够随机的值，避免容器重建后登录会话失效
- 登录用户名和密码来自容器数据目录 `/data/config.yml`、`/data/.env` 或设置页保存结果

如果你希望 Docker 部署时读取宿主机端口信息并用于 `/settings/ports`，需要显式挂载宿主机 `/proc`，并设置 `FLARE_PORT_PROC_ROOT`：

```bash
docker run -d \
  --name superflare \
  -p 3636:3636 \
  -v superflare-data:/data \
  -v /proc:/host/proc:ro \
  -e FLARE_PORT_PROC_ROOT=/host/proc \
  superflare:latest
```

带宿主机 `/proc` 的 Docker Compose：

```bash
docker compose \
  -f ./tools/docker/docker-compose.yml \
  -f ./tools/docker/docker-compose.host-proc.yml \
  up -d
```

这组叠加配置会增加：

- 将宿主机 `/proc` 以只读方式挂载到容器内 `/host/proc`
- 设置 `FLARE_PORT_PROC_ROOT=/host/proc`

即便这样，SuperFlare 最终仍只能读取 Docker 允许暴露给容器的宿主机进程和端口信息。如果宿主机限制了访问，或者使用了非标准 `proc` 结构，那么 `/settings/ports` 仍然可能只能拿到部分结果。

停止命令：

```bash
docker compose -f ./tools/docker/docker-compose.yml down
```

```bash
docker compose \
  -f ./tools/docker/docker-compose.yml \
  -f ./tools/docker/docker-compose.host-proc.yml \
  down
```

运行时镜像内已经包含 `ss` 和 `netstat` 相关依赖，用于增强 Linux 容器环境下的端口探测兼容性。
