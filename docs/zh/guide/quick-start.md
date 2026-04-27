# 快速上手

## 1. 启动服务端

```bash
inlets server -d tunnel.example.com -t your-secret-token
```

生产环境建议使用 **YAML 配置**，见 [服务端 · 配置文件](../server/configuration)。

## 2. 客户端 HTTP 隧道

```bash
inlets client --server https://tunnel.example.com -t your-secret-token http 127.0.0.1:9000
```

无 TLS 测试时 `--server` 可用 `http://`。需要固定子域时加 `--sub-domain myapp`。

## 3. 客户端 TCP 隧道（如 SSH）

```bash
inlets client --server https://tunnel.example.com -t your-secret-token tcp -p 20100 127.0.0.1:22
```

用户连接 `tunnel.example.com:20100`（或你公布的地址），流量转到本机 22 端口。

## 4. 无 token 的临时会话

不传 `--token` / `--credentials` 时，服务端可能对监控连接 **限时**，见 [公共监控会话时限](/zh/features/PUBLIC_MONITOR_SESSION)。

## 延伸阅读

- [使用示例](./examples)
- [客户端 · 鉴权](../client/authentication)
- [参考 · 架构](../reference/architecture)
