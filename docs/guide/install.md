# Install

Inlets ships as a **single Go binary** (`inlets`) that contains the **client**, **server**, and **forward** commands.

## Requirements

- **Go** 1.21+ (to build from this repository), or a prebuilt binary if your distribution provides one.
- For the docs site only: **Node.js** 18.12+ and **pnpm** (see `docs/README.md`).

## Build from source

From the repository root:

```bash
git clone https://github.com/go-idp/inlets.git
cd inlets
go build -o inlets ./cmd/inlets
```

Install on your `PATH` (optional):

```bash
# example: user-local bin
mv inlets "$HOME/bin/inlets"
export PATH="$HOME/bin:$PATH"
```

## Verify

```bash
inlets --version
# or
inlets -V
```

You should see the client, server, and forward subcommands in `inlets --help`.

## Firewall and TLS

- **Server**: open the **HTTP/WebSocket** port (default `8080`) and the **TCP tunnel** port (default `8443`) if you use TCP tunnels.
- **TLS**: many deployments terminate TLS in front of Inlets (reverse proxy) or use server `--secure` / config `secure` depending on your setup.

After install, continue with [Quick start](./quick-start).
