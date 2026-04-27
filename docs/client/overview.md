# Client overview

The **Inlets client** runs near your **upstream** service (loopback or LAN). It:

1. Connects to the server **monitor** WebSocket and authenticates.
2. For **v2**, may open a **data** WebSocket for binary streaming when negotiated.
3. Accepts **HTTP** requests or **TCP** stream assignments from the server and proxies them locally.
4. Sends responses or byte streams back through the tunnel.

## Tunnel types

| Mode | Command shape | Typical use |
| --- | --- | --- |
| HTTP | `inlets client … http <upstream>` | Local web app, APIs, WebSocket apps |
| TCP | `inlets client … tcp -p <publicPort> <upstream>` | SSH, databases, custom TCP |

## Transport modes

- **v2 (default)** — `--server https://host` (or `http://`, optional path). Capability negotiation, recommended for new deployments.
- **Legacy v1** — `--legacy` with `--remote host:port` and `--remote-tcp-port`. For older servers only.

Never mix `--server` with `--legacy`.

## Resilience

The client maintains **heartbeat** / health checks and **reconnects** when the control channel drops. Large responses and TCP relays use paths that avoid hanging on half-closed connections when negotiated.

## Next pages

- [HTTP tunnel](./http-tunnel)
- [TCP tunnel](./tcp-tunnel)
- [Authentication](./authentication)
