# 用户账号相关

superflare 默认会启动免登陆模式，方便在 HomeLab 或本地使用的小伙伴。

然而，有一些小伙伴需要在公网使用，本篇文档就来展示如何设置和获取 superflare 的用户和密码。

## 设置 SuperFlare 账号和密码

我们可以通过在环境变量中设置 `FLARE_USER` 和 `FLARE_PASS` 来指定 superflare 的账号和密码，下面是一个容器编排文件示例：

```yaml
version: '3.6'

services:
  superflare:
    image: junfuchang/superflare
    restart: always
    # 默认无需添加任何参数，如有特殊需求
    # 可阅读文档 https://github.com/junfuchang/superflare/blob/main/docs/legacy-docker/advanced-startup.md
    # 启用账号登陆模式
    command: superflare --nologin=0
    environment:
      # 如需开启用户登陆模式，需要先设置 `nologin` 启动参数为 `0`
      # 如开启 `nologin`，未设置 FLARE_USER，则默认用户为 `superflare`
      - FLARE_USER=superflare
      # 指定你自己的账号密码，如未设置 `FLARE_USER`，则会默认生成密码并展示在应用启动日志中
      - FLARE_PASS=your_password
    ports:
      - 5005:5005
    volumes:
      - ./app:/app
```

更多示例，可以参考仓库根目录下的 `examples/`。

在执行命令前，不妨执行 `docker pull junfuchang/superflare` 确认使用的应用镜像是新版本。

当你使用 `docker-compose up -d` 启动应用之后，接着使用 `docker-compose ps`，就可以看到包含密码的日志输出啦：

```bash
INFO[2023-05-07T11:13:13+08:00] SuperFlare v0.4.1-332C2E0E24789AA4D7F578AA14E7BA6F62970ADA linux/amd64 BuildDate=2023-05-07T03:06:36Z
INFO[2023-05-07T11:13:13+08:00]
INFO[2023-05-07T11:13:13+08:00] 程序服务端口 5005
INFO[2023-05-07T11:13:13+08:00] 页面请求合并 false
INFO[2023-05-07T11:13:13+08:00] 启用离线模式 false
INFO[2023-05-07T11:13:13+08:00] 已禁用登陆模式，用户可直接调整应用设置。
INFO[2023-05-07T11:13:13+08:00] 在线编辑模块启用，可以访问 /editor 来进行数据编辑。
INFO[2023-05-07T11:13:13+08:00] 向导模块启用，可以访问 /guide 来获取程序使用帮助。
INFO[2023-05-07T11:13:13+08:00] 程序已启动完毕 🚀
```
