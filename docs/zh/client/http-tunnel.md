# HTTP 隧道

将 **本地 HTTP**（及支持 WebSocket 的应用）暴露到隧道服务端分配的公网主机名。

## 命令

```bash
inlets client [--server <url>] [鉴权参数] http <upstream>
```

`upstream` 可为端口或 `host:port`。

## 示例

```bash
inlets client http 127.0.0.1:9000
inlets client --server https://tunnel.example.com -t token --sub-domain myapp http 127.0.0.1:9000
```

## 本地上游 Basic

设置 `INLETS_UPSTREAM_HTTP_USERNAME`、`INLETS_UPSTREAM_HTTP_PASSWORD`。

## WebSocket / 流式

v2 能力协商开启后，升级与语义 HTTP 流有专门路径；若本地正常、隧道异常，请核对版本与日志。

## 无凭据监控

可能触发服务端 **限时** 策略，见 [公共监控会话时限](/zh/features/PUBLIC_MONITOR_SESSION)。
