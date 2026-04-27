# TCP 隧道

将服务端 **公网 TCP 端口** 映射到本机 `host:port`。

## 命令

```bash
inlets client [--server <url>] [鉴权参数] tcp -p <公网端口> <upstream>
```

## 示例

```bash
inlets client -t token tcp -p 20100 127.0.0.1:22
inlets client --credentials id:secret tcp -p 20100 127.0.0.1:22
```

环境变量 `INLETS_TUNNEL_PORT` 可代替 `-p`。

## 上游不可用

新版本在 **Dial** 失败时会通知对端 teardown，避免用户连接长期挂死。请先保证上游进程已监听。

## 监听顺序

服务端仅在 **Listen 成功** 后宣告就绪；端口占用会通过 **fatal** 错误让客户端退出，便于运维修正。
