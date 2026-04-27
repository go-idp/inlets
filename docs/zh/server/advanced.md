# 服务端进阶

## 带宽

- 每个 **`clients[]`** 可设 `bandwidthLimit`。
- **`bandwidthLimits.global`** 作用于全局；**`bandwidthLimits.clients.<clientId>`** 可覆盖。

## 通知

配置 `notification.provider` 与 `url`，将隧道事件推到钉钉、飞书、企业微信或 Slack。可用 `interval` 降频。

## 公共监控会话

客户端 **未** 使用 `--token` / `--credentials` 时，可启用 **`publicHTTPNoAuth`** 限时策略（与 HTTP 隧道类型、边缘 Basic 无关）。详见 [公共监控会话时限](/zh/features/PUBLIC_MONITOR_SESSION)。

## YAML 多隧道

在 **credentials** 模式下，`clients[].tunnels` 可列出额外隧道；主进程仍对应用户 CLI，其它行可由协调逻辑拉起子会话。自动拉起的 TCP 行通常需写明 **`remotePort`**。见 `conf/example/server.yaml` 与 [客户端 · 进阶](../client/advanced)。

## 稳定性

新版本包含 HTTP 隧道 **超时（504）**、回调竞态修复等；详见 [发行说明](/zh/features/RELEASE_NOTES_2026-03-15)。

协议实现细节：[新协议说明](/zh/features/NEW_PROTOCOL_ISSUES)。
