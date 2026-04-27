# 使用示例

## 客户端

### 连本地服务端调试

```bash
inlets client --server http://127.0.0.1:8080 -t dev-token http 127.0.0.1:9000
```

### `--server` 带路径前缀

```bash
inlets client --server https://tunnel.example.com/base -t token http 127.0.0.1:9000
```

### 固定子域

```bash
inlets client --sub-domain myapp -t token http 127.0.0.1:9000
```

### TCP：SSH

```bash
inlets client --credentials prod:secret tcp -p 20100 127.0.0.1:22
```

### Legacy v1

```bash
inlets client --legacy --remote tunnel.example.com:443 --remote-tcp-port 8443 http 127.0.0.1:9000
```

**勿**与 `--server` 同时使用。

## 服务端

```bash
inlets server -d tunnel.example.com -t your-secret-token
inlets server -c /etc/inlets/config.yaml
inlets server -d tunnel.example.com -t token -p 9000 --tcp-port 9443
inlets server -d tunnel.example.com -t your-token --secure=false
```

## 本地 forward

```bash
inlets forward -s 0.0.0.0:8080 -t 127.0.0.1:3000
```

## 环境变量

客户端/服务端参数可用 `INLETS_` 前缀，详见 [命令行参考](../reference/cli)。
