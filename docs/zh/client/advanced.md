# 客户端进阶

## Legacy

```bash
inlets client --legacy --remote tunnel.example.com:443 --remote-tcp-port 8443 http 127.0.0.1:9000
```

勿与 `--server` 同用。

## 环境变量

主要标志均有 `INLETS_*` 对应项，命令行优先。完整表见 [命令行参考](../reference/cli)。

## 服务端 `tunnels` 合并

credentials 鉴权下，服务端可返回 YAML 中的 **`tunnels`** 列表：当前进程保持 CLI 隧道，其它行可自动拉起 **子进程**（带 `opaqueChild` 等防递归）。YAML 里自动起的 TCP 隧道通常需 **`remotePort`**。

## 协议说明

见 [新协议说明](/zh/features/NEW_PROTOCOL_ISSUES)。

## forward

```bash
inlets forward -s 0.0.0.0:8080 -t 127.0.0.1:3000
```

本地端口转发工具，非云隧道。
