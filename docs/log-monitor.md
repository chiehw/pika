# 日志监控

Pika Agent 原生监控本机日志，匹配后的事件通过现有 WebSocket 上报，在 Pika 的“告警记录”中显示。通知沿用已有告警规则、渠道和自定义请求体模板，无需安装 logtail 或运行转换程序。

## 网页配置

1. 同时升级 Pika 服务端与目标机器的 Agent 到支持日志监控的版本（v0.3.0 起）。
2. 在“探针 → 探针详情 → 日志监控”中启用监控，添加规则。
3. 填写规则名称、日志文件绝对路径、正则表达式、告警级别。路径属于 Agent 所在机器，每行一个，支持 `*`、`?` 和字符集合；不支持递归 `**`。
4. 保存配置。在线 Agent 会立即应用，离线 Agent 在下次连接时应用。页面会显示配置应用结果；路径格式、文件权限等问题会反馈到页面。
5. 在“设置 → 告警规则”中，找到该探针实际匹配的规则，开启“日志告警通知”，选择已有通知渠道。

日志事件始终记录；未匹配通知规则、通知开关关闭或处于该规则维护时段时，不发送通知。日志事件类型为 `log`，状态为 `notice`，没有阈值、数值或自动恢复通知。

## Clash Party / macOS 示例

规则名称：`Clash Party`。日志路径：

```text
~/Library/Application Support/mihomo-party/logs/clash-party-*.log
~/Library/Application Support/mihomo-party/logs/core-*.log
```

Agent 会把 `~` 展开成运行该 Agent 的用户主目录。以系统服务运行时，建议填写完整路径，例如 `/Users/your-user/Library/Application Support/mihomo-party/logs/core-*.log`，并确认服务用户能读取文件。

匹配表达式：

```text
(?i)error|only
```

`(?i)` 表示忽略大小写；`|` 表示任一关键词命中。表达式使用 Go 正则语法。默认每 5 秒检查一次，网页可设为 1–60 秒。冷却时间默认 0，同文日志的每次新增都会产生独立告警；设置大于 0 时，规则冷却期间的命中直接跳过，不生成告警记录。

## 复用通知模板

现有 `{{agent.name}}`、`{{alert.type}}`、`{{alert.message}}`、`{{message}}` 等变量继续使用。日志事件额外提供：

| 变量 | 内容 |
|---|---|
| `{{alert.message}}` | 命中的日志原文 |
| `{{alert.logFile}}` | 日志文件完整路径 |
| `{{alert.logRuleName}}` | 日志规则名称 |

Bark 渠道仍按 Pika 已有的自定义 Webhook 配置，地址 `https://api.day.app/push`、方法 `POST`，请求体例如：

```json
{
  "device_key": "YOUR_BARK_DEVICE_KEY",
  "title": "{{agent.name}} {{alert.type}}",
  "body": "规则：{{alert.logRuleName}}\n文件：{{alert.logFile}}\n{{alert.message}}",
  "group": "Clash Party 告警-macbook-air",
  "ttl": 600
}
```

模板值沿用 Pika 的 JSON 转义方式，日志中的引号和换行不会破坏 JSON。多个告警类型共用同一个渠道时，可使用 `{{message}}`，由 Pika 生成各类型对应的完整正文。

## 文件与投递行为

- 首次添加规则时，从当时已有文件的末尾开始，不补发历史日志。后续产生的新文件从头读取。
- 停用监控或规则会清除对应未确认事件；重新启用从当时文件末尾开始，不补发停用期间的日志。
- 文件按行读取；尚未写完换行的半行留到下一次扫描。支持文件被截断、替换和按日期轮换。
- 读取位置、规则配置和待确认事件保存到 Agent 配置文件旁的 `log-monitor-state.json`，文件权限为 `0600`。断线期间继续读取并入队，Agent 重启后继续处理。
- 上报复用 Pika 的可靠事件流，并增加日志事件确认。服务端按探针和事件 ID 去重；同文日志的两次新增具有不同 ID，不会被吞掉。
- 每个 Agent 最多保存 4096 个未确认日志事件。队列满时暂停读取，确认释放空间后继续，避免直接丢弃新日志。日志在补读前被应用删除或覆盖时，未读部分仍可能丢失。
- 每条规则最多 16 个路径、256 个匹配文件；每个 Agent 最多 32 条规则。单行超过 64 KiB 会跳过；按 UTF-8 文本处理，每个文件每次扫描最多读取 1 MiB。
- 此功能复用 Pika 现有通知队列与渠道重试；本地事件确认表示服务端已持久化告警，不代表手机已收到推送。渠道发送结果以现有通知机制为准。

## 构建

```bash
go test ./...
npm ci --prefix web --registry=https://registry.npmjs.org
npm run build --prefix web
```

完整二进制发布包由 `scripts/build-release.py` 生成；默认主题使用 `.github/default-theme.ref` 锁定的上游版本。
