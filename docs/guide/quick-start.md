# Quick start

This page gets you from zero to a working **HTTP tunnel** and mentions **TCP** in one command line each. Replace hostnames, tokens, and ports with your own.

## 1. Start the server

Minimal example (token auth, HTTP on 8080, TCP hub on 8443):

```bash
inlets server -d tunnel.example.com -t your-secret-token
```

For real deployments, prefer a **YAML config file** and set `domain`, `port`, `tcpPort`, `clients`, etc. See [Server configuration](../server/configuration).

## 2. Run the client (HTTP)

Point the client at your server and a **local upstream**:

```bash
inlets client --server https://tunnel.example.com -t your-secret-token http 127.0.0.1:9000
```

- Use `http://` when testing without TLS on the server front door.
- Add `--sub-domain myapp` if your server assigns hostnames as `myapp.tunnel.example.com`.

## 3. Run the client (TCP)

Expose local SSH on public port `20100`:

```bash
inlets client --server https://tunnel.example.com -t your-secret-token tcp -p 20100 127.0.0.1:22
```

Users connect to `tunnel.example.com:20100` (or your published address); the stream is forwarded to your machine.

## 4. Public monitor session (no token)

If you omit `--token` and `--credentials`, the server may treat the session as **temporary** and close it after a configured time. See [Public monitor session TTL](/features/PUBLIC_MONITOR_SESSION).

## Where to go next

- [Examples](./examples) — dev vs prod patterns, legacy mode, base path on `--server`.
- [Client authentication](../client/authentication) — token vs credentials vs public.
- [Architecture](../reference/architecture) — control plane vs data plane.
