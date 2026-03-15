# Changelog

All notable changes to this project will be documented in this file.

## 2026-03-15

### Fixed

- Fixed a server-side HTTP callback race in tunnel request handling by registering callbacks before request dispatch.
- Added server-side timeout fallback for tunneled HTTP requests to avoid indefinite pending requests; timed out requests now return `504 Gateway Timeout`.
- Added atomic callback consumption semantics (`Take(tcpId, requestId)`) to prevent duplicate callback execution and stale callback retention.
- Fixed client-side upstream HTTP response handling to parse responses via HTTP semantics instead of waiting for EOF, improving behavior with keep-alive upstream connections.

### Tests

- Added `internal/server/container/callback_test.go` to verify callback fetch-and-remove behavior.
- Added `internal/server/channels/monitor/auth_test.go` to ensure response callbacks are consumed exactly once.
- Added `internal/server/tunnel/http_test.go` to validate timeout response behavior.

### Documentation

- Updated `README.md` and `README.zh.md` with a stability update section for HTTPS pending behavior under high concurrency.
- Updated `docs/features/NEW_PROTOCOL_ISSUES.md` with latest fix status and test coverage.
