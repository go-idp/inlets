# CLI reference

Authoritative help: `inlets --help` and subcommands `inlets client --help`, `inlets server --help`. This page mirrors the repository `README.md` tables for quick lookup.

## Client

### HTTP tunnel

```bash
inlets client [--server <url>] [auth flags] http <upstream>
```

### TCP tunnel

```bash
inlets client [--server <url>] [auth flags] tcp -p <publicPort> <upstream>
```

### Client flags

| Parameter | Description | Default |
| --- | --- | --- |
| Subcommand `http` / `tcp` | Tunnel type | Required |
| `upstream` | Local port or `host:port` | Required |
| `--sub-domain` | HTTP: custom subdomain | — |
| `-p`, `--port` | TCP: public port on server (`INLETS_TUNNEL_PORT`) | — |
| `-t`, `--token` | Token authentication | — |
| `--credentials` | `clientId:clientSecret` | — |
| `--client-id` / `--client-secret` | Overrides `--credentials` when both set | — |
| `--server` | v2 URL (`http://` / `https://`, optional path) | build default |
| `-r`, `--remote` | Legacy: server `host:port` | — |
| `--remote-tcp-port` | Legacy: TCP callback port | `8443` |
| `--healthcheck-interval` | Heartbeat / auth timeout (ms) | `30000` |
| `--legacy` | Use v1 protocol | `false` |
| `--report-url` | Error report webhook | — |

### Client environment variables (`INLETS_*`)

| Variable | Purpose |
| --- | --- |
| `INLETS_SERVER` | v2 server URL |
| `INLETS_TOKEN`, `INLETS_CREDENTIALS` | Auth |
| `INLETS_CLIENT_ID`, `INLETS_CLIENT_SECRET` | Credential pair |
| `INLETS_TUNNEL_PORT`, `INLETS_SUB_DOMAIN` | TCP / HTTP helpers |
| `INLETS_UPSTREAM_HTTP_USERNAME`, `INLETS_UPSTREAM_HTTP_PASSWORD` | Local upstream Basic |
| `INLETS_LEGACY`, `INLETS_REMOTE`, `INLETS_REMOTE_TCP_PORT` | Legacy mode |
| `INLETS_HEALTHCHECK_INTERVAL`, `INLETS_REPORT_URL` | Ops |

## Server

```bash
inlets server -d <domain> [options]
```

| Parameter | Description | Default |
| --- | --- | --- |
| `-d`, `--domain` | Tunnel domain | — |
| `-p`, `--port` | HTTP/WebSocket port | `8080` |
| `--tcp-port` | TCP hub port | `8443` |
| `-s`, `--secure` | HTTPS URLs | `true` (typical) |
| `-t`, `--token` | Shared token | — |
| `-c`, `--config` | YAML path | search paths / `$HOME/.config/inlets.yml` |
| `--notification-provider` | `dingtalk`, `feishu`, `wecom`, `slack` | — |
| `--notification-url` | Webhook URL | — |

### Server environment variables

| Variable | Purpose |
| --- | --- |
| `INLETS_DOMAIN` | Domain |
| `INLETS_SERVER_PORT`, `INLETS_SERVER_TCP_PORT` | Ports |
| `INLETS_SECURE` | HTTPS toggle |
| `INLETS_NOTIFICATION_PROVIDER`, `INLETS_NOTIFICATION_URL` | Alerts |

## Forward

```bash
inlets forward -s <listen> -t <target>
```

Local TCP forward helper (not the cloud tunnel).

## Docs site

To work on this documentation: `cd docs && pnpm install && pnpm dev`. See [maintainer README](../README.md).
