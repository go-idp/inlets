# Release Notes - 2026-03-15

## Highlights

This release improves tunnel stability for HTTPS traffic under higher concurrency and resolves cases where some requests could remain pending.

## What Changed

- Fixed HTTP callback registration race in server tunnel processing.
- Added tunnel request timeout protection and `504 Gateway Timeout` fallback.
- Added atomic callback consumption (`Take`) to avoid duplicate callback handling.
- Switched client upstream response handling from EOF-based completion to HTTP protocol parsing.

## Impact

- Reduced risk of indefinite pending requests when traffic increases.
- Improved compatibility with keep-alive upstream services.
- Better cleanup behavior for callback lifecycle.

## Validation

- `go test ./...` passes.
- Added tests:
  - `internal/server/container/callback_test.go`
  - `internal/server/channels/monitor/auth_test.go`
  - `internal/server/tunnel/http_test.go`

## Upgrade Notes

- No CLI parameter changes required.
- Existing deployments can upgrade directly.
- If you still observe pending requests, verify upstream service latency and connection behavior, and review server/client logs for timeout entries.
