---
layout: home

hero:
  name: Inlets Go
  text: HTTP 与 TCP 隧道
  tagline: 基于 WebSocket 的长期连接，把本地服务安全暴露到公网。
  actions:
    - theme: brand
      text: 什么是 Inlets
      link: /zh/guide/introduction
    - theme: alt
      text: 快速上手
      link: /zh/guide/quick-start
    - theme: alt
      text: GitHub
      link: https://github.com/go-idp/inlets

features:
  - icon: 🌐
    title: HTTP 与 TCP
    details: 统一控制面与数据通道，反向代理 HTTP、中继 TCP。
  - icon: 🔐
    title: 灵活鉴权
    details: Token、credentials，或未凭据的公共监控会话（可配置时限）。
  - icon: 🔁
    title: 重连与心跳
    details: 内置保活、漂移处理与客户端自动重连。
  - icon: ⚙️
    title: 现代协议
    details: v2 能力协商，兼容旧服务器的 legacy 模式。
  - icon: 🖥️
    title: 服务端能力
    details: 配置热更新、带宽限制、钉钉/飞书/Slack/企业微信等通知。
  - icon: 🧪
    title: 可测试
    details: 覆盖 HTTP 劫持、TCP 中继、流式与协议边界等集成测试。
---

## 快速开始

在仓库根目录编译：

```bash
go build -o inlets ./cmd/inlets
```

示例：启动 HTTP 隧道：

```bash
inlets client http 127.0.0.1:9000
```

请阅读 [什么是 Inlets](/zh/guide/introduction)，或从侧栏按「入门 → 服务端 → 客户端 → 参考」浏览；也可直接查看 [命令行参考](/zh/reference/cli) 与 [架构](/zh/reference/architecture)。
