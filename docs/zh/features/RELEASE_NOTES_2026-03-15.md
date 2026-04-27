# 发行说明 · 2026-03-15

## 要点

本版本提升高并发下 HTTPS 隧道的稳定性，修复部分请求长时间处于 pending 的情况。

## 变更摘要

- 修复服务端隧道处理中 HTTP 回调注册的竞态。
- 增加隧道请求超时保护，超时返回 `504 Gateway Timeout`。
- 增加原子回调消费（`Take`），避免重复处理回调。
- 客户端上游响应由「依赖 EOF 结束」改为按 HTTP 协议解析完成。

## 影响

- 流量升高时，请求无限期挂起的风险降低。
- 与 keep-alive 上游的兼容性更好。
- 回调生命周期清理更可靠。

## 验证

- `go test ./...` 通过。
- 新增测试：
  - `internal/server/container/callback_test.go`
  - `internal/server/channels/monitor/auth_test.go`
  - `internal/server/tunnel/http_test.go`

## 升级说明

- 无需变更 CLI 参数。
- 现有部署可直接升级。
- 若仍见 pending，请检查上游延迟与连接行为，并查看服务端/客户端日志中的超时记录。
