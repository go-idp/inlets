# Server overview

The **Inlets server** terminates public **HTTP** (including WebSocket upgrades) and **TCP** connections, then forwards them to connected clients over the tunnel.

## Responsibilities

- **Control plane** — WebSocket endpoint (typically `/_/monitor`) for authentication, tunnel lifecycle, and HTTP request/response envelopes on the classic path.
- **HTTP edge** — Host-based routing to the right tunnel; optional edge auth (see server YAML `tunnels[].auth`).
- **TCP hub** — Listens on `tcpPort`; accepts user connections and pairs them with client-side upstream dials.
- **Policy** — Per-client bandwidth limits, global limits, public-session TTL for unauthenticated monitor logins, notifications.

## Protocol

- **v2** — Default for modern clients: capability negotiation, optional dedicated **data channel** (`/_/data`) for high-throughput binary frames. TCP streams may negotiate **`TCPEarlyStreamRegister`** so the client is ready before the first relayed byte; servers apply a short compatibility delay only when that capability is absent (older v2 clients).
- **Legacy v1** — Older wire format; clients use `--legacy` and `--remote` / `--remote-tcp-port`.

## Configuration styles

1. **CLI flags** — Quick tests (`-d`, `-t`, `-p`, `--tcp-port`, `-c`, …).
2. **YAML file** — Production: `clients`, `tunnels`, `bandwidthLimits`, `notification`, `publicHTTPNoAuth`, etc. Supports **hot reload** when the file changes.

See [Run](./run) and [Configuration](./configuration) for details.
