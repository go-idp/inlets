# Public (unauthenticated) monitor session time limit

## What it is

The tunnel control plane (monitor WebSocket) can authenticate in three ways: **token**, **credentials** (`clientId:clientSecret`), or **public** (no server credentials).

If the client connects **without** `--token` or `--credentials`, the server treats it as a **temporary user**. To limit abuse, the server can automatically close the monitor connection after a configurable duration.

This policy is **only** about client–server monitor authentication. It does **not** depend on:

- HTTP vs TCP tunnel type  
- “Edge” or public-URL auth (Basic on the public hostname, server tunnel `auth` in YAML, etc.)

Those remain separate features.

## Server behavior

- **Decision** is made only in `shouldApplyPublicMonitorSessionTTL` (`auth.authType` not `token` and not `credentials`).  
- **Timer** is started after successful `authenticate` via `schedulePublicMonitorSessionTTL`.  
- The client only receives `warn` events and, at expiry, a normal close with text `public monitor session timeout` (older servers may still send `public http no-auth timeout`; the client accepts both).

## Configuration

`publicHTTPNoAuth` in server YAML (name kept for compatibility):

```yaml
publicHTTPNoAuth:
  timeout: 10m   # default if omitted
  warnLead: 2m  # default if omitted; warn this long before close
```

## Client examples

**Temporary (public) session** — time limit may apply; client logs server `warn` and may exit when the server closes the session:

```bash
inlets client --server https://tunnel.example.com http 127.0.0.1:9000
```

**Registered client** — no public-session time limit for this feature:

```bash
inlets client --server https://tunnel.example.com --credentials client1:secret1 http 127.0.0.1:9000
```

```bash
inlets client --server https://tunnel.example.com -t your-token http 127.0.0.1:9000
```

## Related code

- `internal/server/channels/monitor/auth.go` — `shouldApplyPublicMonitorSessionTTL`, `schedulePublicMonitorSessionTTL`  
- `internal/client/client.go` — `shouldExitOnPublicHTTPNoAuthTimeoutClose` (name historical; handles both close reason strings)  
- `conf/example/server.yaml` — `publicHTTPNoAuth` example comments  
