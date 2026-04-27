# Server advanced topics

## Bandwidth limits

- **Per client** — Under each `clients[]` entry, `bandwidthLimit.upload` / `download` (bytes per second).
- **Global** — `bandwidthLimits.global` applies to all traffic unless overridden.
- **Overrides** — `bandwidthLimits.clients.<clientId>` can refine limits per credential.

Tune based on fair-use policy and upstream capacity.

## Notifications

Supported providers include **DingTalk**, **Feishu**, **WeCom**, and **Slack**. Set `notification.provider` and `notification.url` to your incoming webhook. Optional `interval` reduces alert noise.

Use this for tunnel connect/disconnect or error surfacing in ops channels.

## Public monitor session TTL

When clients connect **without** `--token` or `--credentials`, the server may close the monitor WebSocket after `publicHTTPNoAuth.timeout`, with `warnLead` warnings beforehand. This is **independent** of HTTP tunnel type and edge Basic auth.

Details: [Public monitor session TTL](/features/PUBLIC_MONITOR_SESSION).

## Multi-tunnel rows (`clients[].tunnels`)

With **credentials**, the server can list extra tunnel specs in YAML. The **main** client process still follows its CLI; additional rows can spawn **child** client sessions automatically. HTTP rows may include `subDomain` and `auth` (edge). TCP rows should set **`remotePort`** when the extra tunnel must bind a specific public port.

See comments in `conf/example/server.yaml` and client [Advanced](../client/advanced).

## Stability and timeouts

Recent releases add HTTP tunnel **request timeouts** (504 on expiry) and safer callback lifecycle. Operational note: if you still see rare pending requests, check upstream latency and logs.

Release context: [Release notes (2026-03-15)](/features/RELEASE_NOTES_2026-03-15).

## Protocol implementation

For streaming, flow control, and data-channel behavior, see [New protocol notes](/features/NEW_PROTOCOL_ISSUES).
