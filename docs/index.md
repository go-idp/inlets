---
layout: home

hero:
  name: Inlets Go
  text: HTTP & TCP tunnels
  tagline: Long-lived WebSocket tunnels that expose local services to the public internet.
  actions:
    - theme: brand
      text: Introduction
      link: /guide/introduction
    - theme: alt
      text: Quick start
      link: /guide/quick-start
    - theme: alt
      text: GitHub
      link: https://github.com/go-idp/inlets

features:
  - icon: 🌐
    title: HTTP & TCP
    details: Reverse-proxy HTTP and relay TCP through a single control plane and data channels.
  - icon: 🔐
    title: Flexible auth
    details: Token, credentials, or public monitor sessions with optional time limits.
  - icon: 🔁
    title: Reconnect & heartbeat
    details: Built-in keepalive, drift handling, and automatic reconnection on the client.
  - icon: ⚙️
    title: Modern protocol
    details: v2 capability negotiation with a legacy mode for older servers.
  - icon: 🖥️
    title: Server features
    details: Hot-reload config, bandwidth limits, and notifications (DingTalk, Feishu, Slack, WeCom).
  - icon: 🧪
    title: Tested
    details: Integration coverage for HTTP hijack, TCP relay, streaming, and protocol edges.
---

## Quick start

Build the binaries from the repository root:

```bash
go build -o inlets ./cmd/inlets
```

Run a public HTTP tunnel (example):

```bash
inlets client http 127.0.0.1:9000
```

Read [What is Inlets](/guide/introduction), browse the sidebar (Introduction → Server → Client → Reference), or jump to the [CLI reference](/reference/cli) and [architecture](/reference/architecture).
