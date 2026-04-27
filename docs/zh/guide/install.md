# 安装

Inlets 编译为单个 **`inlets`** 二进制，内含 **client**、**server**、**forward** 子命令。

## 环境

- 从源码构建需要 **Go 1.21+**。
- 仅构建文档站需要 **Node.js 18.12+** 与 **pnpm**（见 `docs/README.md`）。

## 源码构建

```bash
git clone https://github.com/go-idp/inlets.git
cd inlets
go build -o inlets ./cmd/inlets
```

可将二进制放入 `PATH`，例如 `~/bin/inlets`。

## 验证

```bash
inlets --version
```

`inlets --help` 可查看子命令。

## 网络与 TLS

- **服务端**需放行 **HTTP/WebSocket** 端口（默认 `8080`）及 **TCP 隧道**端口（默认 `8443`）。
- **TLS** 可在 Inlets 前由反向代理终结，或使用配置/参数中的 HTTPS 相关选项。

完成后继续 [快速上手](./quick-start)。
