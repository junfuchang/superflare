# Linux 部署说明

这个目录提供 SuperFlare 在 Linux 原生环境中的部署脚本和说明。

项目当前推荐的 Linux 部署方式是原生二进制运行，不推荐把 Docker 作为默认部署模型。Docker 仅保留兼容用途，相关文件在 `tools/docker/`。

## 文件说明

- `build-superflare.sh`：构建 Linux 可执行文件，并执行资源生成与测试
- `run-superflare.sh`：前台运行 SuperFlare，适合手动调试
- `restart-superflare.sh`：重新构建并后台启动 SuperFlare
- `clean-superflare.sh`：停止进程并清理构建产物、缓存和运行日志
- `superflare.service`：`systemd` 服务模板
- `install-systemd.sh`：将当前构建结果安装为 `systemd` 服务

## 目录要求

SuperFlare 默认从当前工作目录读取以下文件：

- `config.yml`
- `apps.yml`
- `bookmarks.yml`
- `ports.yaml`
- `.env`（可选）
- `var/`

因此 Linux 下最直接的部署方式就是：将仓库作为运行目录，或者将这些文件和 `superflare` 二进制一并放到同一个目标目录。

## 环境要求

- Linux
- Go `1.26.x`
- 建议系统自带 `curl`
- 如需更稳定的端口采集兼容性，建议系统安装 `ss`

## 快速部署

首次构建：

```bash
chmod +x ./tools/linux/*.sh
./tools/linux/build-superflare.sh
```

前台调试运行：

```bash
./tools/linux/run-superflare.sh
```

后台重建并启动：

```bash
./tools/linux/restart-superflare.sh
```

停止并清理：

```bash
./tools/linux/clean-superflare.sh
```

## 常用环境变量

- `SUPERFLARE_GO_BIN`：指定 Go 可执行文件路径，或其所在目录
- `SUPERFLARE_PORT`：运行端口，默认 `3636`
- `SUPERFLARE_ENABLE_LOGIN`：是否启用登录，可取 `true/false`
- `SUPERFLARE_COOKIE_SECRET`：登录 Cookie 密钥
- `GOARCH`：构建目标架构，默认 `amd64`

说明：

- 如果 `SUPERFLARE_ENABLE_LOGIN` 未设置，`run-superflare.sh` 与 `restart-superflare.sh` 会在启动前询问是否启用登录
- 如果启用登录，用户名和密码来自当前 `config.yml` / `.env` / 设置页保存结果

## 生产环境建议

`run-superflare.sh` 与 `restart-superflare.sh` 在未设置 `SUPERFLARE_COOKIE_SECRET` 时，会为本次运行生成临时随机 Cookie 密钥；重启后已登录会话会失效。

`install-systemd.sh` 在未设置 `SUPERFLARE_COOKIE_SECRET` 且目标 `.env` 中也没有 `FLARE_COOKIE_SECRET` 时，会生成随机 Cookie 密钥并写入目标 `.env`。如果目标 `.env` 已经存在密钥，脚本会继续沿用，避免破坏已有登录会话。

生产环境建议显式设置稳定且足够随机的 Cookie 密钥：

```bash
export SUPERFLARE_COOKIE_SECRET='请替换为足够随机的生产密钥'
export SUPERFLARE_ENABLE_LOGIN=true
export SUPERFLARE_PORT=3636
```

然后执行：

```bash
./tools/linux/restart-superflare.sh
```

## systemd 部署

先构建：

```bash
./tools/linux/build-superflare.sh
```

然后以 root 安装：

```bash
sudo SUPERFLARE_ENABLE_LOGIN=true \
  SUPERFLARE_PORT=3636 \
  ./tools/linux/install-systemd.sh
```

默认安装位置：

- 程序目录：`/opt/superflare`
- 服务名：`superflare`
- 配置文件：`/opt/superflare/.env`

可选变量：

- `SUPERFLARE_INSTALL_DIR`：自定义安装目录
- `SUPERFLARE_SERVICE_NAME`：自定义服务名

常用命令：

```bash
sudo systemctl status superflare
sudo systemctl restart superflare
sudo journalctl -u superflare -f
```

安装脚本会自动把以下项目写入目标目录的 `.env`：

- `FLARE_PORT`
- `FLARE_DISABLE_LOGIN`
- `FLARE_COOKIE_SECRET`
- `FLARE_EDITOR=true`

后续如果你要修改端口、登录开关或 Cookie 密钥，直接编辑目标目录下的 `.env`，然后执行：

```bash
sudo systemctl restart superflare
```

## 端口采集说明

Linux 原生部署时，`/settings/ports` 默认读取本机进程和端口信息，不需要像 Docker 那样额外挂载宿主机 `/proc`。

如果你是在容器中运行，再去读取宿主机端口信息，才需要参考 `tools/docker/docker-compose.host-proc.yml` 或手动挂载 `/proc` 并设置 `FLARE_PORT_PROC_ROOT`。
