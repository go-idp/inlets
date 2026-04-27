# 鉴权

监控通道常见三种方式：

## Token

```bash
inlets client --server https://tunnel.example.com -t your-token http 127.0.0.1:9000
```

环境变量：`INLETS_TOKEN`。

## Credentials

服务端 YAML `clients` 中配置的 `clientId` / `clientSecret`。可同时使用 `--client-id` 与 `--client-secret`（优先于 `--credentials`）。

```bash
inlets client --credentials client1:secret1 http 127.0.0.1:9000
```

## Public（无监控凭据）

不传 token/credentials 时，可能被视作临时用户并受 **`publicHTTPNoAuth`** 约束。与 HTTP 隧道类型、公网 Basic **无关**。详见 [公共监控会话时限](/zh/features/PUBLIC_MONITOR_SESSION)。

## 边缘鉴权（HTTP）

在服务端隧道条目的 **`auth`** 上配置，保护公网 URL。见 `conf/example/server.yaml`。
