# Pika 探针监控系统

<div align="center">

一个基于 Go + PostgreSQL/SQLite + VictoriaMetrics 的实时探针监控系统

[快速开始](#快速开始) • [截图](#截图) • [功能特性](#功能特性) • [文档](#文档) • [加入群聊](#加入群聊) 

</div>

## 原生日志监控与二进制下载

本分支新增 Pika Agent 本地日志监控，在 **探针详情 → 日志监控** 网页中配置路径、正则、告警级别、检查间隔和冷却时间。命中日志进入现有告警记录，复用 Pika 告警规则、通知渠道和模板；无需安装 logtail。通知需在对应告警规则中开启“日志告警通知”。服务端和 Agent 都需要升级。

[下载二进制与完整服务端运行包](https://github.com/chiehw/pika/releases/latest) · [日志监控配置与通用示例](docs/log-monitor.md)

服务端运行包包含管理网页、默认主题、配置示例和 Agent 下载文件。解压后复制 `config.sqlite.yaml` 为 `config.yaml`，配置数据库、认证和 VictoriaMetrics 地址，在解压目录运行 `./pika serve --config config.yaml`。VictoriaMetrics 需单独运行。独立 Agent 二进制可按现有安装流程使用。

下面的 Docker 示例使用本仓库的 GHCR 镜像。可通过 `PIKA_VERSION` 固定版本，例如 `PIKA_VERSION=0.3.3 docker compose -f docker-compose.sqlite.yml up -d`。

## 简介

Pika 是一个轻量级的探针监控系统，支持实时数据采集、存储和查询。系统采用 WebSocket 进行探针与服务端的通信，使用 VictoriaMetrics 存储时序指标数据，支持 PostgreSQL 和 SQLite 两种数据库方案。除了基础监控功能外，还提供 Linux 应急响应和安全基线检查能力，帮助快速发现和分析系统安全风险。

## 功能特性

- **📊 实时性能监控**：CPU、内存、磁盘、网络、GPU、温度等系统资源监控
- **📄 日志监控**：网页配置本地文件与正则，原生 Agent 采集，复用告警和通知模板
- **🔍 服务监控**：HTTP/HTTPS、TCP 端口、ICMP/Ping 监控，支持证书到期检测
- **🛡️ 防篡改保护**：文件实时监控、属性巡检、事件告警
- **🔒 安全审计**：资产清单收集、安全风险分析、历史审计记录
- **🔐 多种认证**：Basic Auth、OIDC、GitHub OAuth
- **📦 轻量部署**：Docker Compose 一键部署，资源占用低

详细功能说明请参考 [功能特性文档](docs/features.md)。

## 截图

![public1.png](screenshots/public1.png)
![public2.png](screenshots/public2.png)
![public3.png](screenshots/public3.png)
![public4.png](screenshots/public4.png)
![sec1.png](screenshots/sec1.png)
![sec2.png](screenshots/sec2.png)
![tamper.png](screenshots/tamper.png)
![setting.png](screenshots/setting.png)

## 快速开始

### SQLite 版本

```bash
# 下载配置文件
curl -O https://raw.githubusercontent.com/chiehw/pika/main/docker-compose.sqlite.yml
curl -o config.yaml https://raw.githubusercontent.com/chiehw/pika/main/config.sqlite.yaml

# 修改配置（重要：修改 JWT Secret 和管理员密码）
# 编辑 config.yaml

# 启动服务
docker-compose -f docker-compose.sqlite.yml up -d

# 访问 http://localhost:8080
# 默认账户 admin / admin123
```

详细文档：[SQLite 版本部署指南](docs/deployment-sqlite.md)

### PostgreSQL 版本

```bash
# 下载配置文件
curl -O https://raw.githubusercontent.com/chiehw/pika/main/docker-compose.postgresql.yml
curl -o config.yaml https://raw.githubusercontent.com/chiehw/pika/main/config.postgresql.yaml

# 修改配置（重要：修改数据库密码、JWT Secret 和管理员密码）
# 编辑 config.yaml

# 启动服务
docker-compose -f docker-compose.postgresql.yml up -d

# 访问 http://localhost:8080
# 默认账户 admin / admin123
```

详细文档：[PostgreSQL 版本部署指南](docs/deployment-postgresql.md)

## 文档

- [日志监控](docs/log-monitor.md)
- [功能特性](docs/features.md)
- [SQLite 版本部署指南](docs/deployment-sqlite.md)
- [PostgreSQL 版本部署指南](docs/deployment-postgresql.md)
- [通用配置说明](docs/common-config.md)

## Docker 镜像发布

只有推送 `v*` 版本标签才会自动构建和发布镜像，普通分支提交不会触发。工作流向 `ghcr.io/chiehw/pika` 发布 Linux amd64、arm64 镜像，并提供原始标签（如 `v0.3.3`）、版本号（`0.3.3`）、次版本（`0.3`）以及正式版本的 `latest`。镜像包含管理网页、默认主题和全部受支持的 Agent 二进制。

修改本地镜像标签或 Compose 的 `PIKA_VERSION` 只会选择已有镜像，不会触发构建。需要重跑时，可在 Actions 中手动选择该版本标签运行。GHCR 首次创建的包默认为私有；如需免登录拉取，可在 GitHub Packages 设置中将其改为公开。

## 环境要求

- Docker 20.10+
- Docker Compose 1.29+

## 加入群聊 

请见官网 https://pika.termark.app 获取社群入口

https://pika.termark.app
