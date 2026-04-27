# HTTP tunnel

Expose a **local HTTP** server (or WebSocket-capable app) on the public hostname provided by the tunnel server.

## Command shape

```bash
inlets client [--server <url>] [auth flags] http <upstream>
```

`upstream` is a port (`9000`) or `host:port` (`127.0.0.1:9000`).

## Examples

```bash
# Default v2 server URL from build / env
inlets client http 127.0.0.1:9000

# Explicit server + token + subdomain
inlets client --server https://tunnel.example.com -t token --sub-domain myapp http 127.0.0.1:9000
```

## Upstream Basic auth

If your **local** service requires Basic authentication, set:

- `INLETS_UPSTREAM_HTTP_USERNAME`
- `INLETS_UPSTREAM_HTTP_PASSWORD`

## WebSocket and streaming

Under v2 with negotiated capabilities, **WebSocket upgrades** and **semantic HTTP streaming** follow dedicated code paths. If something works on localhost but not through the tunnel, confirm both client and server are on a recent version and check logs.

## Public monitor note

Omitting token/credentials may hit a **time-limited** monitor session. See [Public monitor session TTL](/features/PUBLIC_MONITOR_SESSION).

## See also

- [Authentication](./authentication)
- [Advanced](./advanced) — legacy mode, env vars, server-driven extra tunnels
