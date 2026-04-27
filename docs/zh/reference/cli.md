# 命令行参考

权威说明以 `inlets --help` 及各子命令 `--help` 为准。下表与仓库 `README.md` 对齐，便于检索。

## 客户端

### HTTP

```bash
inlets client [--server <url>] [鉴权] http <upstream>
```

### TCP

```bash
inlets client [--server <url>] [鉴权] tcp -p <公网端口> <upstream>
```

### 常用参数

| 参数 | 说明 | 默认 |
| --- | --- | --- |
| `http` / `tcp` | 隧道类型 | 必填 |
| `upstream` | 本机端口或 `host:port` | 必填 |
| `--sub-domain` | HTTP 子域 | — |
| `-p`, `--port` | TCP 公网端口 | — |
| `-t`, `--token` | Token | — |
| `--credentials` | `clientId:clientSecret` | — |
| `--client-id` / `--client-secret` | 凭据对，优先于 `--credentials` | — |
| `--server` | v2 服务端 URL | 构建默认 |
| `--legacy` | v1 协议 | `false` |
| `-r`, `--remote` | Legacy 地址 | — |
| `--remote-tcp-port` | Legacy TCP 回调端口 | `8443` |
| `--healthcheck-interval` | 心跳间隔（ms） | `30000` |
| `--report-url` | 错误上报 Webhook | — |

### 客户端环境变量（`INLETS_*`）

`INLETS_SERVER`、`INLETS_TOKEN`、`INLETS_CREDENTIALS`、`INLETS_CLIENT_ID`、`INLETS_CLIENT_SECRET`、`INLETS_TUNNEL_PORT`、`INLETS_SUB_DOMAIN`、本地上游 Basic、`INLETS_LEGACY`、`INLETS_REMOTE`、`INLETS_REMOTE_TCP_PORT`、`INLETS_HEALTHCHECK_INTERVAL`、`INLETS_REPORT_URL` 等。

## 服务端

```bash
inlets server -d <domain> [选项]
```

| 参数 | 说明 | 默认 |
| --- | --- | --- |
| `-d`, `--domain` | 域名 | — |
| `-p`, `--port` | HTTP/WebSocket | `8080` |
| `--tcp-port` | TCP 隧道 | `8443` |
| `-s`, `--secure` | HTTPS URL | 视构建 |
| `-t`, `--token` | Token | — |
| `-c`, `--config` | YAML | 搜索路径 |
| 通知参数 | 钉钉/飞书/企业微信/Slack | — |

### 服务端环境变量

`INLETS_DOMAIN`、`INLETS_SERVER_PORT`、`INLETS_SERVER_TCP_PORT`、`INLETS_SECURE`、通知相关等。

## forward

```bash
inlets forward -s <监听> -t <目标>
```

## 文档站

维护文档：`cd docs && pnpm install && pnpm dev`，见 [README](../../README.md)。
