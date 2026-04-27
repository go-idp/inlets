# Configuration

The server loads **YAML** configuration from a path you pass with `-c` / `--config`, or from default locations (see `conf/example/server.yaml` in the repository).

## Example skeleton

```yaml
domain: tunnel.example.work
port: 8080
tcpPort: 8443
secure: false
token: your-secret-token-here

clients:
  - clientId: client1
    clientSecret: secret1
    config:
      version: "2.0.0"
    bandwidthLimit:
      upload: 1024000
      download: 1024000
    # Optional: extra tunnels merged with this client's CLI (credentials mode)
    # tunnels:
    #   - name: web
    #     type: http
    #     upstream: 127.0.0.1:9000
    #     subDomain: myapp

bandwidthLimits:
  global:
    upload: 512000
    download: 512000
  clients:
    client1:
      upload: 1024000
      download: 1024000

notification:
  provider: dingtalk
  url: https://oapi.dingtalk.com/robot/send?access_token=YOUR_ACCESS_TOKEN

# Optional: limit duration of unauthenticated monitor sessions
# publicHTTPNoAuth:
#   timeout: 10m
#   warnLead: 2m
```

## Key fields

- **`domain`** — Base domain for HTTP tunnels; browser `Host` must match `<sub>.<domain>` (port in `Host` allowed for local tests).
- **`port` / `tcpPort`** — HTTP/WebSocket and TCP hub ports.
- **`secure`** — Whether URLs are generated as HTTPS.
- **`token`** — Global token auth (optional when using `clients` entries).
- **`clients`** — Credential pairs, optional per-client protocol version, bandwidth, and optional **`tunnels`** list for extra sessions.
- **`bandwidthLimits`** — Global and per-client byte-per-second caps.
- **`notification`** — Webhook provider and URL for alerts.
- **`publicHTTPNoAuth`** — Session TTL when clients connect **without** token or credentials on the monitor channel.

## Hot reload

Changing the YAML on disk triggers a reload in the running server (watch the logs for success or errors). Invalid config should fail safely; verify after edits.

## Full sample

See **`conf/example/server.yaml`** in the repo for commented edge auth, tunnel rows, and defaults.
