# 公共（无凭据）监控会话时限

## 是什么

隧道控制面（monitor WebSocket）支持三种鉴权：**token**、**credentials**（`clientId:clientSecret`）、**public**（不向服务端提供凭据）。

若客户端**未**传入 `--token` 或 `--credentials`，服务端将其视为**临时用户**；为限制滥用，可在配置时长后自动关闭监控连接。

该策略**仅**针对客户端与服务端之间的 **monitor 鉴权**，**不**依赖：

- HTTP 与 TCP 隧道类型  
- 「边缘」或公网 URL 鉴权（公网主机上的 Basic、YAML 里隧道 `auth` 等）

后者仍是独立能力。

## 服务端行为

- **判定**仅在 `shouldApplyPublicMonitorSessionTTL`（`auth.authType` 非 `token` 且非 `credentials`）。
- **定时器**在 `authenticate` 成功后由 `schedulePublicMonitorSessionTTL` 启动。
- 客户端会收到 `warn`；到期时连接以文案 **`public monitor session timeout`** 正常关闭（旧服务端可能仍发 `public http no-auth timeout`；客户端二者均识别）。

## 配置

服务端 YAML 中 `publicHTTPNoAuth`（名称历史兼容）：

```yaml
publicHTTPNoAuth:
  timeout: 10m   # 省略时默认值
  warnLead: 2m  # 省略时默认值；关闭前提前 warn 的时长
```

## 客户端示例

**临时（公共）会话** — 可能受时限约束；客户端会记录服务端 `warn`，会话被关时可能退出：

```bash
inlets client --server https://tunnel.example.com http 127.0.0.1:9000
```

**已登记客户端** — 本特性不对其施加「公共会话」时限：

```bash
inlets client --server https://tunnel.example.com --credentials client1:secret1 http 127.0.0.1:9000
```

```bash
inlets client --server https://tunnel.example.com -t your-token http 127.0.0.1:9000
```

## 相关代码

- `internal/server/channels/monitor/auth.go` — `shouldApplyPublicMonitorSessionTTL`、`schedulePublicMonitorSessionTTL`  
- `internal/client/client.go` — `shouldExitOnPublicHTTPNoAuthTimeoutClose`（命名历史原因；处理上述两种关闭原因）  
- `conf/example/server.yaml` — `publicHTTPNoAuth` 示例注释  
