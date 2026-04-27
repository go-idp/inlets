# TCP tunnel

Map a **public TCP port** on the tunnel server to a **local** `host:port` (for example SSH).

## Command shape

```bash
inlets client [--server <url>] [auth flags] tcp -p <publicPort> <upstream>
```

`-p` / `--port` is the **server-side** listening port. `upstream` is your local service (`127.0.0.1:22`).

## Examples

```bash
inlets client -t token tcp -p 20100 127.0.0.1:22
inlets client --credentials clientId:secret tcp -p 20100 127.0.0.1:22
```

## Environment

`INLETS_TUNNEL_PORT` can set the public port when you prefer env over `-p`.

## Failure behavior

If the client **cannot dial** the upstream (service down, firewall), modern builds signal teardown so the user connection does not hang forever on the server side. Prefer running a reachable upstream before accepting public traffic.

## Listen ordering

The server sends **ready** signals only after a successful `Listen` on the public port. Port conflicts surface as fatal errors to the client so operators can fix configuration.

## See also

- [Authentication](./authentication)
- [Server run](../server/run) — `tcpPort` must be reachable
