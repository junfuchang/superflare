# Docker 构建说明

本目录存放 SuperFlare 的 Docker 构建、运行和 Compose 示例文件。

项目根目录下的 `config.yml`、`apps.yml`、`bookmarks.yml`、`ports.yaml` 是运行时数据文件，默认被 `.gitignore` 排除，不会随 Git 仓库提交。Docker 镜像构建不依赖这些本地文件；容器首次启动后，SuperFlare 会在 `/data` 中自动补齐缺失的运行配置。

## 文件说明

- `Dockerfile`：Linux 容器多阶段构建文件
- `docker-entrypoint.sh`：容器入口脚本，会从 `/data` 工作目录启动 SuperFlare
- `build-image.sh`：Linux / macOS 构建脚本
- `build-image.ps1`：Windows PowerShell 构建脚本
- `docker-compose.yml`：基础 Compose 运行示例
- `docker-compose.host-proc.yml`：读取宿主机 `/proc` 的可选叠加配置

## 运行时数据

容器内可写运行时数据统一放在 `/data`：

- `/data/config.yml`
- `/data/apps.yml`
- `/data/bookmarks.yml`
- `/data/ports.yaml`
- `/data/.env`（可选）
- `/data/var/`

建议用 Docker volume 或宿主机目录持久化 `/data`。镜像升级或容器重建时，只要复用同一个 `/data`，已有配置不会被覆盖。

## 构建镜像

Linux / macOS：

```bash
chmod +x ./tools/docker/build-image.sh
./tools/docker/build-image.sh superflare latest
```

Windows PowerShell：

```powershell
.\tools\docker\build-image.ps1 -ImageName superflare -Tag latest
```

强制无缓存构建：

```bash
NO_CACHE=1 ./tools/docker/build-image.sh superflare latest
```

指定平台构建：

```bash
PLATFORM=linux/amd64 ./tools/docker/build-image.sh superflare latest
PLATFORM=linux/arm64 ./tools/docker/build-image.sh superflare latest
```

## 推送到 Docker Hub

先登录 Docker Hub：

```bash
docker login
```

构建并打上 Docker Hub 仓库标签，例如：

```bash
NO_CACHE=1 ./tools/docker/build-image.sh junfucn/superflare latest
```

推送：

```bash
docker push junfucn/superflare:latest
```

建议同时推送一个固定版本标签，方便回滚：

```bash
docker tag junfucn/superflare:latest junfucn/superflare:20260630
docker push junfucn/superflare:20260630
```

## 运行容器

基础 `docker run`：

```bash
docker run -d \
  --name superflare \
  -p 3636:3636 \
  -v superflare-data:/data \
  --restart unless-stopped \
  superflare:latest
```

基础 Docker Compose：

```bash
docker compose -f ./tools/docker/docker-compose.yml up -d
```

首次部署前建议编辑 `tools/docker/docker-compose.yml`：

- `FLARE_DISABLE_LOGIN=false`：启用登录
- `FLARE_DISABLE_LOGIN=true`：关闭登录
- `FLARE_COOKIE_SECRET`：请替换为稳定且足够随机的值，避免容器重建后登录会话失效

登录用户名和密码来自 `/data/config.yml`、`/data/.env` 或设置页保存结果。

## 读取宿主机端口信息

如果希望 Docker 部署时读取宿主机端口信息并用于 `/settings/ports`，需要显式挂载宿主机 `/proc`，并设置 `FLARE_PORT_PROC_ROOT`：

```bash
docker run -d \
  --name superflare \
  -p 3636:3636 \
  -v superflare-data:/data \
  -v /proc:/host/proc:ro \
  -e FLARE_PORT_PROC_ROOT=/host/proc \
  --restart unless-stopped \
  superflare:latest
```

Compose 叠加方式：

```bash
docker compose \
  -f ./tools/docker/docker-compose.yml \
  -f ./tools/docker/docker-compose.host-proc.yml \
  up -d
```

停止：

```bash
docker compose -f ./tools/docker/docker-compose.yml down
```

带宿主机 `/proc` 叠加配置时：

```bash
docker compose \
  -f ./tools/docker/docker-compose.yml \
  -f ./tools/docker/docker-compose.host-proc.yml \
  down
```
