# Authentication

Three styles of **monitor channel** authentication are common:

## Token (`-t` / `--token`)

Shared secret configured on the server (`token` in YAML or CLI `-t`). Simple for single-tenant or internal use.

```bash
inlets client --server https://tunnel.example.com -t your-token http 127.0.0.1:9000
```

Env: `INLETS_TOKEN`.

## Credentials (`--credentials` or `--client-id` + `--client-secret`)

Per-client id/secret defined under `clients` in server YAML. When both `--client-id` and `--client-secret` are set, they **override** `--credentials`.

```bash
inlets client --credentials client1:secret1 http 127.0.0.1:9000
```

Env: `INLETS_CREDENTIALS`, or `INLETS_CLIENT_ID` + `INLETS_CLIENT_SECRET`.

## Public (no monitor credentials)

If you pass **neither** token nor credentials, the server may classify the session as **temporary** and apply **`publicHTTPNoAuth`** limits (timeout + warn). This is **only** about control-plane login, not HTTP edge Basic auth.

```bash
inlets client --server https://tunnel.example.com http 127.0.0.1:9000
```

Read: [Public monitor session TTL](/features/PUBLIC_MONITOR_SESSION).

## Edge auth (HTTP)

Server YAML can attach **`auth`** rules to HTTP tunnel entries (Bearer, Basic, etc.). That protects the **public URL**, separate from monitor authentication above.

See `conf/example/server.yaml` comments.
