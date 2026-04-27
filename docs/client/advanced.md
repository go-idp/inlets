# Client advanced topics

## Legacy protocol

```bash
inlets client --legacy --remote tunnel.example.com:443 --remote-tcp-port 8443 http 127.0.0.1:9000
```

Do **not** use `--server` together with `--legacy`.

## Environment variables

All major flags have `INLETS_*` equivalents (CLI wins on conflict). Highlights:

- `INLETS_SERVER`, `INLETS_TOKEN`, `INLETS_CREDENTIALS`, `INLETS_CLIENT_ID`, `INLETS_CLIENT_SECRET`
- `INLETS_TUNNEL_PORT`, `INLETS_SUB_DOMAIN`
- `INLETS_LEGACY`, `INLETS_REMOTE`, `INLETS_REMOTE_TCP_PORT`
- `INLETS_HEALTHCHECK_INTERVAL`, `INLETS_REPORT_URL`

Full list: [CLI reference](../reference/cli).

## Server YAML `tunnels` merge

When you authenticate with **credentials**, the server may return a **`tunnels`** list from YAML. The process you started keeps your **CLI** tunnel; **other** rows can spawn **child** processes automatically. Child sessions use an internal **opaque** marker so they do not recurse.

YAML TCP rows that are auto-started generally need an explicit **`remotePort`**. See server example comments and repository `AGENTS.md` entries on per-client tunnels.

## Protocol details

Capability bits, streaming, and TCP-over-WebSocket semantics are documented for contributors in [New protocol notes](/features/NEW_PROTOCOL_ISSUES).

## Forward command

Local port forward helper (not a cloud tunnel):

```bash
inlets forward -s 0.0.0.0:8080 -t 127.0.0.1:3000
```
