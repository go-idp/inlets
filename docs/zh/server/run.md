# 运行

## 命令行

```bash
inlets server -d <domain> [选项]
```

| 参数 | 说明 |
| --- | --- |
| `-d`, `--domain` | 隧道对外域名 |
| `-p`, `--port` | HTTP/WebSocket 端口（默认 `8080`） |
| `--tcp-port` | TCP 隧道端口（默认 `8443`） |
| `-t`, `--token` | 共享 token |
| `-c`, `--config` | YAML 配置路径 |
| `-s`, `--secure` | 是否生成 HTTPS URL（视构建默认而定） |

```bash
inlets server -d tunnel.example.com -t your-token
inlets server -c /etc/inlets/config.yaml
```

## 环境变量

可用 `INLETS_DOMAIN`、`INLETS_SERVER_PORT`、`INLETS_SERVER_TCP_PORT`、`INLETS_SECURE`、通知相关变量等，见 [命令行参考](../reference/cli)。

## 部署建议

使用 **systemd**、容器或进程守护运行；端口与 YAML 中 `port` / `tcpPort` 一致；TLS 与反向代理策略要统一。

下一篇：[配置文件](./configuration)。
