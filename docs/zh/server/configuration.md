# 配置文件

服务端通过 **YAML** 配置，可用 `-c` 指定路径，或按仓库示例中的搜索顺序查找默认文件。

## 骨架示例

```yaml
domain: tunnel.example.work
port: 8080
tcpPort: 8443
secure: false
token: your-secret-token-here

clients:
  - clientId: client1
    clientSecret: secret1
    config:
      version: "2.0.0"
    bandwidthLimit:
      upload: 1024000
      download: 1024000

bandwidthLimits:
  global:
    upload: 512000
    download: 512000

notification:
  provider: dingtalk
  url: https://oapi.dingtalk.com/robot/send?access_token=YOUR_ACCESS_TOKEN

# publicHTTPNoAuth:
#   timeout: 10m
#   warnLead: 2m
```

## 重要字段

- **`domain`** — HTTP 隧道基域；浏览器 `Host` 需为 `<sub>.<domain>`。
- **`port` / `tcpPort`** — HTTP 与 TCP 监听端口。
- **`secure`** — 对外 URL 是否使用 HTTPS。
- **`token`** — 全局 token（与 `clients` 可并存，视部署而定）。
- **`clients`** — `clientId` / `clientSecret`、协议版本、带宽、可选 **`tunnels`** 多隧道。
- **`bandwidthLimits`** — 全局与按客户端限速（字节/秒）。
- **`notification`** — 钉钉/飞书/企业微信/Slack Webhook。
- **`publicHTTPNoAuth`** — 无 token、无 credentials 的监控会话最长存活等。

## 热更新

修改磁盘上的 YAML 后，运行中的服务可 **热加载**（请关注日志中的成功/失败信息）。

完整注释示例见仓库 **`conf/example/server.yaml`**。
