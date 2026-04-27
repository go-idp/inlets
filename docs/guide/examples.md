# Examples

Copy-paste recipes. Adjust domains, tokens, and ports.

## Client

### Local dev against a local server

```bash
inlets client --server http://127.0.0.1:8080 -t dev-token http 127.0.0.1:9000
```

### v2 server URL with path prefix

If the monitor lives under a subpath:

```bash
inlets client --server https://tunnel.example.com/base -t token http 127.0.0.1:9000
```

### HTTP with fixed subdomain

```bash
inlets client --sub-domain myapp -t token http 127.0.0.1:9000
```

### TCP: production SSH

```bash
inlets client --credentials prod:secret tcp -p 20100 127.0.0.1:22
```

### Legacy protocol (v1)

```bash
inlets client --legacy --remote tunnel.example.com:443 --remote-tcp-port 8443 http 127.0.0.1:9000
```

Do **not** combine `--legacy` with `--server`.

## Server

### Basic

```bash
inlets server -d tunnel.example.com -t your-secret-token
```

### Config file

```bash
inlets server -c /etc/inlets/config.yaml
```

### Custom ports

```bash
inlets server -d tunnel.example.com -t token -p 9000 --tcp-port 9443
```

### Disable HTTPS for generated URLs (when appropriate)

```bash
inlets server -d tunnel.example.com -t your-token --secure=false
```

## Forward helper

Local TCP forward (not the tunnel itself):

```bash
inlets forward -s 0.0.0.0:8080 -t 127.0.0.1:3000
```

## Environment variables

Client and server flags can be set with the `INLETS_` prefix. See the [CLI reference](../reference/cli) and repository `README.md` for the full list.
