# FAQ

## v2 vs legacy?

Use **`--server`** with an `http(s)` URL for **v2**. Use **`--legacy`** with **`--remote`** and **`--remote-tcp-port`** only when the server is old (v1). Never combine `--server` and `--legacy`.

## Why did my monitor disconnect with “public monitor session timeout”?

You connected **without** `--token` or `--credentials`. The server may enforce a maximum session length for temporary users. Use credentials or token, or adjust `publicHTTPNoAuth` on the server. See [Public monitor session TTL](/features/PUBLIC_MONITOR_SESSION).

## Browser or curl hangs on HTTP through the tunnel

Check client and server versions: recent releases fix framing for empty or chunked upstream responses and callback races. Upgrade both sides. If it persists, capture logs and upstream response headers.

## TCP connects but hangs

Ensure the **local upstream** is listening before clients hit the public port. The client uses timeouts when dialing upstream; if the service starts later, retry the connection from the user side.

## Can I run multiple tunnels?

Yes: run multiple client processes, or use **credentials** with server YAML **`clients[].tunnels`** so extra rows spawn child sessions (see [Client advanced](../client/advanced)).

## Where is the full protocol described?

Contributor-oriented notes: [New protocol issues](/features/NEW_PROTOCOL_ISSUES).

## How do I build from source?

See [Install](../guide/install).
