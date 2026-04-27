# 客户端概述

**Inlets 客户端**部署在 **上游服务** 附近，负责：

1. 连接服务端 **monitor** 并完成鉴权。
2. 在 v2 下按需建立 **data** 通道。
3. 将服务端下发的 **HTTP** 请求或 **TCP** 流转发到本地上游。

## 隧道类型

| 模式 | 命令形式 | 典型用途 |
| --- | --- | --- |
| HTTP | `inlets client … http <upstream>` | Web、API、WS 应用 |
| TCP | `inlets client … tcp -p <公网端口> <upstream>` | SSH、数据库等 |

## 传输模式

- **v2（默认）** — `--server` 传入 `http(s)://` URL（可选路径）。
- **Legacy** — `--legacy` + `--remote` + `--remote-tcp-port`。

**不要**同时使用 `--server` 与 `--legacy`。

## 弹性

心跳、健康检查与断线 **重连**；在协商开启时使用不易半连接卡死的传输路径。

继续：[HTTP 隧道](./http-tunnel)、[TCP 隧道](./tcp-tunnel)、[鉴权](./authentication)。
