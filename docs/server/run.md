# Run

## CLI

```bash
inlets server -d <domain> [options]
```

Common flags:

| Flag | Description |
| --- | --- |
| `-d`, `--domain` | Public domain for tunnel URLs (typical setups). |
| `-p`, `--port` | HTTP / WebSocket listen port (default `8080`). |
| `--tcp-port` | TCP tunnel listen port (default `8443`). |
| `-t`, `--token` | Default shared token (when not using file-based clients only). |
| `-c`, `--config` | Path to YAML configuration. |
| `-s`, `--secure` | HTTPS for generated URLs where applicable (default `true` in many builds). |

Examples:

```bash
inlets server -d tunnel.example.com -t your-token
inlets server -c /etc/inlets/config.yaml
inlets server -d tunnel.example.com -p 9000 --tcp-port 9443
```

## Environment variables

Server flags can be set with the `INLETS_` prefix (CLI overrides env):

- `INLETS_DOMAIN`, `INLETS_SERVER_PORT`, `INLETS_SERVER_TCP_PORT`, `INLETS_SECURE`
- `INLETS_NOTIFICATION_PROVIDER`, `INLETS_NOTIFICATION_URL`

Full tables: [CLI reference](../reference/cli).

## Process model

Run Inlets under **systemd**, **supervisor**, or a container orchestrator. Ensure:

- The config file path is stable if you rely on hot reload.
- Ports are published and match `port` / `tcpPort` in YAML or CLI.
- TLS is handled consistently (Inlets `--secure` / YAML `secure` vs reverse proxy).

## Next

- [Configuration](./configuration) — YAML structure and search paths.
- [Advanced](./advanced) — bandwidth, notifications, operational tips.
