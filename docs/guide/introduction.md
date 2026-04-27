# What is Inlets

**Inlets Go** is a client and server for **HTTP** and **TCP** tunnels over long-lived **WebSocket** connections. You run the client next to a local service; the server exposes a public hostname or TCP port and forwards traffic through the tunnel.

Compared with a generic reverse proxy, Inlets is built around a **control plane** (monitor channel) and **data paths** tuned for tunneling: authentication, heartbeats, reconnection, and (in v2) optional binary streaming on a dedicated data channel.

## When to use it

Typical scenarios (similar ideas to classic intranet-penetration tools, focused on HTTP/TCP):

1. **Expose a local web app** — You develop on `127.0.0.1:8080` and want a stable `https://yourapp.example.com` without opening your home router.
2. **SSH or other TCP services** — Map a public port on the tunnel server to `127.0.0.1:22` (or any TCP upstream) for remote access.
3. **Callbacks and webhooks** — Third parties call a public URL that reaches your laptop or CI runner.
4. **Demos and staging** — Share a temporary URL with token or credential-based access.

## What you get

- **HTTP tunnel** — Subdomain or host routing to your local HTTP server; WebSocket upgrades and semantic streaming are supported when negotiated.
- **TCP tunnel** — Listen on the server; each inbound connection is relayed to your upstream.
- **Auth** — Shared token, per-client credentials, or a temporary “public” monitor session (often time-limited by server policy).
- **Protocols** — **v2** (default) with `--server` URL and capability negotiation; **legacy v1** with `--legacy` for older servers.
- **Server ops** — YAML config with **hot reload**, per-client and global **bandwidth limits**, and **notifications** (DingTalk, Feishu, WeCom, Slack).

## Next steps

- [Install](./install) — build from source.
- [Quick start](./quick-start) — first client and server in minutes.
- [Examples](./examples) — copy-paste commands for common setups.
