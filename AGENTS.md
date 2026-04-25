# AI Agent 开发经验记录

本文档记录 AI Agent 在代码审查和问题修复过程中积累的经验和最佳实践。

## 使用本文档的约定（需求 / 任务流程）

1. **开始需求或任务前**：先阅读 `AGENTS.md`（至少浏览各节标题，并打开与当前改动域相关的历史条目），对照既有根因、做法与审查清单，避免重复踩坑或与已约定行为冲突。
2. **完成需求后**：将本次可复用的结论写入本文档，在文末按日期追加新小节；建议结构包含：背景或问题现象、做法或修复要点、经验总结与审查清单（如有）、相关文件路径。日志与错误信息宜保持英文（与仓库代码规范一致），说明性文字中英文均可，以清晰为准。

## 2024-12-19: 并发安全和资源管理问题修复

### 问题发现

在审查 `cmd/inlets/server.go` 时发现了以下问题：

#### 1. 不必要的延迟操作
**位置**: `cmd/inlets/server.go:133`
**问题**: 配置文件加载失败时，代码会 sleep 10 秒再返回错误，这是不必要的延迟。
```go
// ❌ 错误做法
if err != nil {
    log.Printf("[server] Warning: Failed to load config file %s: %v", configPath, err)
    time.Sleep(10 * time.Second)  // 不必要的延迟
    return fmt.Errorf("failed to load config file: %v", err)
}
```
**修复**: 直接返回错误，移除 sleep。

#### 2. 并发安全问题 - 过早释放锁
**位置**: `cmd/inlets/server.go:388-439` (createGetTokenFunctionWithRef 函数)
**问题**: 在读取配置时，获取读锁后立即释放，然后继续使用配置数据。在热更新场景下，可能导致读取到不一致的配置数据。
```go
// ❌ 错误做法
configRef.mu.RLock()
configFile := configRef.config
if configFile == nil {
    return nil, fmt.Errorf("config file is required")
}
configRef.mu.RUnlock()  // 过早释放锁

// 继续使用 configFile，但此时可能已经被热更新替换
for _, clientCfg := range configFile.Clients {
    // 可能读取到旧配置
}
```
**修复**: 使用 `defer` 在整个函数执行期间保持读锁，确保读取配置的一致性。
```go
// ✅ 正确做法
configRef.mu.RLock()
defer configRef.mu.RUnlock()  // 在整个函数执行期间保持锁

configFile := configRef.config
if configFile == nil {
    return nil, fmt.Errorf("config file is required")
}
// 现在可以安全地使用 configFile
```

#### 3. 资源泄漏 - Timer 未清理
**位置**: `cmd/inlets/server.go:442-476` (watchConfigFile 函数)
**问题**: 函数退出时未清理 `reloadTimer`，可能导致资源泄漏。
```go
// ❌ 错误做法
func watchConfigFile(...) {
    var reloadTimer *time.Timer
    for {
        select {
        case event, ok := <-watcher.Events:
            if !ok {
                return  // 退出时没有清理 timer
            }
        }
    }
}
```
**修复**: 添加 `defer` 确保退出时清理 timer。
```go
// ✅ 正确做法
func watchConfigFile(...) {
    var reloadTimer *time.Timer
    defer func() {
        if reloadTimer != nil {
            reloadTimer.Stop()
        }
    }()
    // ...
}
```

### 经验总结

1. **并发安全原则**:
   - 使用锁保护共享数据时，应该在整个使用期间保持锁，而不是提前释放
   - 使用 `defer` 确保锁的正确释放，避免忘记解锁
   - 对于读操作，使用 `RLock()` 和 `defer RUnlock()` 的组合

2. **资源管理原则**:
   - 所有需要清理的资源（如 Timer、文件句柄、网络连接等）都应该在函数退出时清理
   - 使用 `defer` 确保资源清理，即使在函数提前返回时也能执行

3. **错误处理原则**:
   - 错误发生时应该立即返回，不要添加不必要的延迟
   - 错误信息应该清晰明确，帮助定位问题

4. **代码审查检查清单**:
   - [ ] 检查锁的使用是否正确（是否过早释放）
   - [ ] 检查资源是否正确清理（Timer、连接、文件等）
   - [ ] 检查错误处理是否合理（是否有不必要的延迟）
   - [ ] 检查并发场景下的数据一致性

### 相关文件
- `cmd/inlets/server.go`: 主要修复文件
- `internal/server/server.go`: 服务器实现，使用配置的地方

## 2024-12-19: 代码注释和日志消息规范化

### 问题发现

代码中存在中文的日志消息，不符合代码规范要求。

### 修复内容

将所有中文日志消息改为英文：

1. **配置文件监听相关日志**:
   - `检测到配置文件变化` → `Config file changed detected`
   - `文件监听出错` → `File watcher error`

2. **热更新相关日志**:
   - `热更新失败` → `Hot reload failed`
   - `配置文件已热更新` → `Config file hot reloaded`
   - `更新服务器配置失败` → `Failed to update server configuration`

3. **通知消息**:
   - `[配置更新] 配置文件已重新加载` → `[Config Update] Config file reloaded`
   - `配置文件路径` → `Config file path`
   - `客户端数量` → `Client count`
   - `当前时间` → `Current time`

### 经验总结

1. **代码规范原则**:
   - 代码注释（comments）应使用英文
   - 日志消息（log messages）应使用英文，便于国际化
   - 错误消息应使用英文，便于调试和问题追踪

2. **国际化考虑**:
   - 使用英文可以确保代码在不同语言环境下都能被理解
   - 便于团队协作和代码维护

3. **代码审查检查清单**:
   - [ ] 检查代码注释是否使用英文
   - [ ] 检查日志消息是否使用英文
   - [ ] 检查错误消息是否使用英文
   - [ ] 检查用户可见的字符串是否需要国际化处理

## 2026-04-06: HTTP Tunnel — Hijack 后重复解析首条请求导致阻塞

### 问题现象

- 公网入口 `curl`（尤其关闭代理 `--noproxy '*'`）对隧道域名请求时 **长时间无响应、0 bytes**，像一直 pending。
- 服务端 HTTP 隧道路径上 **看不到首请求的完整处理**（或行为异常），客户端/上游看似正常。

### 根因

`internal/server/tunnel/http.go` 在 `ServeMux` 的 handler 里对连接 **`Hijack()`** 后，在 goroutine 里用 **`bufio.NewReader(conn)` + `http.ReadRequest(reader)`** 去读“第一条”HTTP 请求。

但 **`net/http.Server` 在调用你的 `Handler` 之前已经解析了当前这条请求**：请求行与头部（以及可能的部分 body 处理）已由 server 完成。此时 TCP 上 **下一段可读数据应是 keep-alive 下的第二条请求**，而不是第一条。对第一条连接再 `ReadRequest` 会 **一直阻塞等待下一条请求**，浏览器/curl 则表现为首包永远不回。

### 错误做法（概念）

```go
// ❌ Hijack 后把整条连接当成“还未读过的第一个请求”
conn, _, _ := hijacker.Hijack()
go func() {
    r, err := http.ReadRequest(bufio.NewReader(conn)) // 首条连接上这里会等“第二条”请求 → 挂死
    // ...
}()
```

### 正确做法

1. **在 `Hijack` 之前**按 `net/http` 约定 **读完 `r.Body` 并 `Close`**（文档要求：Hijack 后不要再读 `r.Body`）。
2. 用 **`r.Method`、`r.URL.RequestURI()`、`r.Proto`、**`r.Host`（须单独写出）**、**`r.Header` 与 body 字节** 拼回与原先一致的 **raw HTTP 报文**（`firstData`），供隧道转发。对 **服务端收到的请求**，`Host` 被提升到 **`r.Host`**，**不会**出现在 `r.Header` 里，仅调用 **`r.Header.Write`** 会导致 **首包缺少 `Host:`**，上游/日志会异常。
3. **`Hijack()` 必须接收 `*bufio.ReadWriter`**：使用返回的 **`rw.Reader`** 作为后续 **`http.ReadRequest`** 的输入（若 `rw`/`rw.Reader` 为空再退回 `bufio.NewReader(conn)`），避免丢弃 server 已在缓冲区里的数据。
4. **`handleConnection`**：对 **第一条** 使用 handler 传入的 **`r` + `firstData`** 调用既有 `processRequest`；**仅对同连接上后续 keep-alive 请求** 使用 **`http.ReadRequest(br)`** 循环。

### 经验总结

1. **`Hijack` 与 `net/http` 状态机**：一旦进入 `Handler`，当前 `*http.Request` 就是“已解析的这一条”；裸连接上不一定还能再用 `ReadRequest` 重读同一条。
2. **缓冲区一致性**：`Hijack` 返回的 `bufio.Reader` 可能已有预读字节，必须用同一个 reader 继续读，不能假设 `conn` 起点是下一条请求的开头。
3. **排查线索**：首请求卡住、工具显示 0 bytes、服务端缺少对应 access/隧道日志时，优先怀疑 **重复解析 / 错误的 ReadRequest 起点**。
4. **代码审查检查清单**（HTTP 隧道 / 任意 Hijack 转发 raw HTTP 的场景）:
   - [ ] Hijack 前是否已处理 `r.Body`（读尽并关闭）？
   - [ ] 首条请求是否使用 handler 的 `r`（及自拼 raw），而不是在 hijacked conn 上再 `ReadRequest` 首包？
   - [ ] 后续 keep-alive 是否使用 **`Hijack` 返回的 `bufio.Reader`** 做 `ReadRequest`？
   - [ ] 首包 raw 是否显式包含 **`Host: ` + `r.Host`**（若非空）？

### 回归测试

- `internal/server/tunnel/http_hijack_integration_test.go`：`TestHTTPTunnelHijackFirstRequestDoesNotBlock`（首包不阻塞且含 Host）、`TestHTTPTunnelHijackKeepAliveSecondRequest`（同连接第二条）、`TestHTTPTunnelHijackPOSTBodyBeforeHijack`（Hijack 前 body 进入 `firstData`）。使用 **`fakeHTTPTunnelAdapter`** 模拟客户端通过 callback 回写 HTTP 响应，无需真实 WebSocket。

### 相关文件

- `internal/server/tunnel/http.go`：`Attach` 中 mux handler、`handleConnection`（`firstData` / `br` / 首包与循环）
- `internal/server/tunnel/http_hijack_integration_test.go`：上述集成测试

## 2026-04-06: HTTP 语义流式 body（CapabilityFlagHTTPBodyStream）

### 背景

- 整包隧道模型 `SendHTTPRequest(id, []byte)` 必须把 **完整 HTTP/1.1 报文** 放进内存；大 body 只能依赖 `maxHTTPTunnelRequestBodyBytes` 等上限缓解。
- 现有 **`CapabilityFlagHTTPStreaming`** + `shouldUseStreaming` 仅表示：单条逻辑消息在 WebSocket 上 **分片传输**，收端 **`streamManager` 拼回整块 `[]byte`** 再调 `httpRequestHandler`，**不是**「边读浏览器连接边转发 body」的语义流式。

### 做法

1. **新能力位 `CapabilityFlagHTTPBodyStream`**（与 `HTTPStreaming` 分离）：仅在 **新协议且协商双方均声明** 时启用；legacy / 未协商仍走整包 + body 上限（`readTunnelRequestBody`）。
2. **新二进制类型**（不走 stream 重组）：`0x07` HTTPRequestHead、`0x08` HTTPRequestBody、`0x09` HTTPResponseHead、`0x0a` HTTPResponseBody。`handleBinaryMessage` 对语义类型 **直接 `dispatchSemanticHTTPMessage`**，每帧单独回调；**Body 帧不压缩**，**Head 帧**可按协商做压缩/解压（与服务端 `DecompressBinaryPayloadForCapabilities`、客户端 `decompressTunnelSemanticHead` / `maybeCompressSemanticResponseHead` 对称）。
3. **入口隧道**（`internal/server/tunnel/http.go`）：在 **非 chunked** 且 **`ContentLength >= 0`**（含无 body）时，首包 **只拼头、Hijack 后** 用 `io.LimitReader` + 固定 buffer 读 body 并 `SendHTTPRequestBody`；chunked / 未知长度 **回退** 整包路径。keep-alive 后续请求在同样条件下从 **`req.Body`** 流式读出并分块发送。
4. **响应回写浏览器**：客户端用 `httputil.DumpResponse(resp, false)` 发 Head，再分块发 Body；monitor 上仍走 `["response",{id,data}]`，`handleResponse` 解析二进制后若为 `0x09/0x0a`，经 **`ctx.HTTPStreamDispatch`**（由 `HTTPTunnel.Attach` 注册）写入 hijacked `tcpConn`，**不要**对语义帧 `Take` 单次 callback；`CallbackContainer` 仍在超时等场景使用。
5. **`ProtocolAdapter`**：补充 `SendHTTPRequestHead/Body`、`SendHTTPResponseHead/Body`、`NegotiatedFlags()`；Legacy 对语义 Send 返回 `ErrSemanticHTTPNotSupported`，`NegotiatedFlags` 为 0。

### 经验总结

1. **命名区分**：`HTTPStreaming` = 传输层分片重组；`HTTPBodyStream` = 应用层头/体分帧、边读边发。
2. **Host 与 keep-alive**：重组 **仅头部** 时仍须显式写 `Host:`（`req.Host`），与首包整包路径一致。
3. **`net.Pipe` 测试**：对管道 **`ReadAll` 会等到 EOF**；测固定长度写应用 **`io.ReadFull`**，否则易假死。
4. **审查清单**（语义 HTTP 隧道）:
   - [ ] 未协商 `HTTPBodyStream` 时是否仍只走 `0x01` 整包解码？
   - [ ] chunked 请求是否回退整包 + 上限？
   - [ ] 响应 Head 是否在 monitor 侧按协商解压后再 `Write` 到浏览器？
   - [ ] 语义响应路径是否避免误用单次 `Take` 消费整段响应？

### 回归测试

- `TestHTTPTunnelDispatchSemanticClientResponse`、`TestHTTPTunnelSemanticStreamPOSTReassemblesHeadAndBody`（`http_hijack_integration_test.go`）；`streamRecordAdapter` 实现 `NegotiatedFlags` 与语义 Send 以模拟客户端能力。

### 相关文件

- `internal/server/protocol/binary.go`：消息类型、语义发送/分发
- `internal/server/tunnel/http.go`：`processRequestStream`、`dispatchSemanticClientResponse`
- `internal/server/channels/monitor/auth.go`：`handleResponse` 语义分支
- `internal/client/handlers.go`：`handleHTTPRequest` 按类型分发、流式响应发送
- `internal/client/types.go`、`internal/server/channels/monitor/helpers.go`：能力协商

## 2026-04-20: TCP 公网监听失效与端口占用

### 问题现象

- 服务端已为容器创建 TCP 隧道并 `Listen` 后，若 accept 循环因 **`Accept` 永久错误** 或 **listener 已关闭** 退出，容器上仍保留非 nil 的 **`SourceServer`**，而 `CreateServer` 在 `SourceServer != nil` 时直接跳过，**永远不会再监听**；隧道事件仅在认证成功时触发一次，**监控 WebSocket 未断时客户端不会重连**，用户侧公网 TCP 入口永久不可用。
- 客户端指定或动态分配的隧道端口若已被占用，`Listen` 失败；若仍先发 **`tcp:ready`** 再 `Listen`，客户端会误以为隧道已就绪。

### 做法

1. **Accept 退出后自愈**：在 `runTCPAcceptLoop` 中，对非临时 `Accept` 错误：`Set(containerId, "sourceServer", nil)` 清除陈旧 listener，再调用 **`CreateServer`** 重建监听；临时错误短暂 sleep 后重试 `Accept`。
2. **端口复用**：`CreateServer` 在客户端未指定非零 `TunnelPort` 时，若已有 **`SourcePort`**（例如上次成功监听过的端口），优先复用该端口，避免恢复时随机换端口。
3. **`sourceServer` 置空**：`internal/server/container/tunnel.go` 中 `Set(..., "sourceServer", nil)` 支持将监听句柄清空，便于重建路径与销毁逻辑一致。
4. **绑定失败与客户端退出**：**先 `net.Listen`，成功后再发 `tcp:ready`**。若 `Listen` 失败（含 `address already in use`），通过监控通道发送 `["error", { "message": "...", "fatal": true, "code": "tcp_listen_failed" }]`；客户端在 `handleMonitorMessages` 中对 **`fatal` 为 true 的 error** 执行 **`os.Exit(1)`**，避免进程空转。
5. **测试**：`internal/server/tunnel/tcp_test.go` 中 `TestTCPCreateServerReusesSourcePort`、`TestTCPTunnelListenerRecreatesOnClose` 覆盖端口复用与关闭 listener 后自动重建。

### 经验总结

1. **状态机**：长期运行的 listener 与容器字段 **`SourceServer` 必须同步**；goroutine 退出时要么在进程内重建并更新引用，要么显式清空并依赖下一次控制面事件，不能留下「已关闭 listener + 非 nil 字段」的组合。
2. **消息顺序**：对依赖本地资源（端口、文件）就绪的控制消息，应在 **资源分配成功之后** 再通知对端，否则对端无法区分「未就绪」与「永久失败」。
3. **不可恢复错误**：端口冲突等应 **带结构化 fatal 标志** 通知客户端并退出，便于编排系统重启或换配置；仅靠服务端日志不足以让客户端停止重试或告警。
4. **审查清单**（TCP 隧道）:
   - [ ] `tcp:ready` 是否在 `Listen` 成功之后发送？
   - [ ] accept 退出路径是否清除 `SourceServer` 并触发重建或明确失败？
   - [ ] 动态端口场景恢复是否复用 `SourcePort`？
   - [ ] `Listen` 失败是否通知客户端并 `fatal` 退出？

### 相关文件

- `internal/server/tunnel/tcp.go`：`CreateServer`、`sendTCPListenFatalError`、`runTCPAcceptLoop`
- `internal/server/container/tunnel.go`：`Set` 的 `sourceServer` / `nil`
- `internal/client/client.go`：监控通道 `error` + `fatal`
- `internal/server/tunnel/tcp_test.go`：上述回归测试

## 2026-04-20: 新协议流式重组 — EnsureStream 与 AddChunk

### 问题发现

- `StreamManager.AddChunk` 在流不存在时曾 **在已持锁情况下调用 `CreateStream` 再次加锁**，存在 **死锁**；自动创建的流 **`onComplete` 为 nil**，重组完成后无法交给 HTTP/TCP 处理器，表现为流式消息静默丢失。
- `docs/features/NEW_PROTOCOL_ISSUES.md` 中 🔴「缺少 onComplete」与「流控竞态」需与当前实现对齐：发送路径应 **只用 `TrySend`**；分片序号在 **同一 `streamId` 内从 0 递增** 与按流隔离的重组一致，并非全局序号冲突。

### 修复与约定

1. **`EnsureStream(streamId, onComplete, onError)`**：幂等注册首帧回调；`AddChunk` **仅向已存在流追加**，否则打日志并丢弃，不再自动建流。
2. **`handleBinaryMessage` 流式分支**：若无法解析出非 nil 的 `onComplete`（含流式 TCP 且无任何 `tcpDataHandlers`），**返回错误**；成功时 **`EnsureStream` + `AddChunk`**。
3. **`FlowController.CanSend`**：注释标明 **禁止 `CanSend` + `Send` 做准入**；扣窗口用 **`TrySend`**。
4. **文档**：`NEW_PROTOCOL_ISSUES.md` 顶部增加 **2026-04-20** 修复状态；概览表中原 🔴 三项标为已修复/已澄清。
5. **测试**：`internal/server/protocol/stream_manager_test.go`（无流 AddChunk 不死锁、重组、首回调优先）。

### 相关文件

- `internal/server/protocol/stream_manager.go`、`stream_manager_test.go`
- `internal/server/protocol/binary.go`、`flow_controller.go`
- `docs/features/NEW_PROTOCOL_ISSUES.md`

## 2026-04-20: 数据通道竞态与流重组停滞驱逐

### 做法

1. **Upgrade 后二次校验**：`/_/data` 在 `Upgrade` 成功后再次 `Container.Get` 并比对 `ClientId`；失败则关连接，避免隧道已销毁仍注册 `DataSockets`。
2. **defer 用最新映射**：数据通道 `handleConnection` 退出时用 **`Get(containerId)`** 清理 `DataSockets` / `DataWriteMu` 并调用 **`RemoveDataChannelForStream`**（内部已清流管理器与流控窗口），避免闭包捕获的 `container` 指针过期。
3. **流停滞驱逐**：`Stream` 增加 **`createdAt` / `lastActivity`**；`StreamManager.cleanup` 驱逐超过 **`stallTimeout`（默认 2m）** 无新 chunk 或超过 **`maxStreamAge`（默认 5m）** 的未完成流，调用 **`OnError`** 后 **`RemoveStream`**。

### 相关文件

- `internal/server/channels/data/new.go`
- `internal/server/protocol/stream_manager.go`、`stream_manager_test.go`
- `docs/features/NEW_PROTOCOL_ISSUES.md`

## 2026-04-20: 监控通道可观测性与流控退避

### 做法

1. **认证前 binary**：`/_/monitor` 在 **`isAuthenticated == false`** 时若收到 **binary** 帧，**每条连接只打一条** 说明性日志，提示先完成文本 **`authenticate`**。
2. **`request` 路径**：注释写明新协议下 **HTTP 仍走监控通道** 的 `request`+base64 载荷，**TCP 数据** 走 **`/_/data`**，避免与「全部走 data」混淆。
3. **流控发送等待**：`BinaryProtocolAdapter.waitFlowSendSlot` 使用 **5ms→100ms  capped 指数退避** 替代固定 50ms 忙等。
4. **`HandleBinaryMessage` 分发失败日志**：解析失败已有 hex 预览；**`handleBinaryMessage` 返回错误** 时再打一条（`type`、`streamId`），与解析失败区分。
5. **`NEW_PROTOCOL_ISSUES.md` 文末**：优先级清单标为历史存档，**权威状态** 以文首 **2026-04-20** 与概览表为准；**TCP over WS 不回退监控** 见文档第 12 点。

### 相关文件

- `internal/server/channels/monitor/new.go`
- `internal/server/protocol/binary.go`
- `docs/features/NEW_PROTOCOL_ISSUES.md`

## 2026-04-20: 二进制协议回归测试 — HandleBinaryMessage 与流控等待

### 背景

`handleBinaryMessage` 在流式协商下若缺少对应 handler 必须返回错误（避免 `EnsureStream` 带 nil `onComplete`）；非流式路径上 HTTP 处理器错误应向上传播；`waitFlowSendSlot` 在循环内调用 **`TrySend`**（失败不扣窗口、成功则占用窗口），需确认对端 **`OnAck`** 释放窗口后等待能结束。

### 测试用例（`binary_tcp_test.go`）

1. **`TestHandleBinaryMessage_StreamingHTTPClientRequiresHandler`**：客户端 `Streaming` + `HTTPStreaming`、未注册 `OnHTTPRequest` 时，首帧应报错（`streaming type ... has no handler`）。
2. **`TestHandleBinaryMessage_PropagatesHTTPResponseHandlerError`**：服务端非流式 `HTTPResponse`，`OnHTTPResponse` 返回的 error 经 `HandleBinaryMessage` 原样返回（`errors.Is`）。
3. **`TestHandleBinaryMessage_InvalidWireReturnsError`**：截断/过短线消息，`HandleBinaryMessage` 返回解析错误。
4. **`TestWaitFlowSendSlotUnblocksAfterOnAck`**：`InitializeStream` + `TrySend` 占满窗口后，异步 `OnAck` 再调用 `waitFlowSendSlot`；用较短延迟 +「耗时大于阈值」断言避免忙等误判，全 suite 下已通过。

### 经验总结

- 协议分发层测试优先构造 **最小 `Capabilities` + `BuildBinaryMessage`**，无需真实 WebSocket。
- 测 `waitFlowSendSlot` 时注意：成功退出时 **最后一次 `TrySend` 已在等待循环内执行**，`SendWindow` 会反映该次占用；后续断言应与此一致。

### 审查清单

- [ ] 流式/非流式两条路径对「缺 handler」和「handler 报错」是否都有覆盖？
- [ ] 流控相关测试若涉时间，是否留有 CI 抖动余量？

### 相关文件

- `internal/server/protocol/binary_tcp_test.go`
- `internal/server/protocol/binary.go`（`HandleBinaryMessage` / `waitFlowSendSlot`）

## 2026-04-20: TCP over WebSocket — 上游未就绪时用户连接永久挂起

### 背景与问题现象

新协议下用户访问服务端暴露的 TCP 端口时，服务端已 `Accept` 并保持 **`sourceConn`**，客户端在收到 `tcp:connect` 后对本地上游 **`Dial`**。若**真实上游尚未监听**（或长时间不可达），客户端 **`Dial` 失败**后仅从 `tcpStreams` 删除占位项，**未通知服务端**结束该条流；服务端不关闭用户侧 TCP，表现为**一直挂起**。上游随后启动也**无法挽救同一条**用户连接：客户端不会对同一 `requestId` 再次拨号，用户若不重试则仍卡住。

### 修复要点

1. **客户端**：`Dial` 改为 **`DialTimeout`（如 10s）**；失败时经数据通道发送 **`MessageTypeTCPClose` (0x05)**，再删 stream 并 **`removeDataChannel`**。
2. **协议**：`BinaryProtocolAdapter.handleBinaryMessage` 对 **TCPClose** 早分发；新增 **`OnTCPClose`**（`ProtocolAdapter` / `LegacyProtocolAdapter` 实现）；`shouldUseCompression` 排除 TCPClose。
3. **服务端隧道**：`setupTCPStreamOverWebSocket` 注册 **`OnTCPClose`**，用 **`sync.Once`** 统一关闭 **`sourceConn`**、取消 **TCP 数据订阅**，与上传 goroutine 退出路径一致；下载侧写失败也走同一 **`closeUserConn`**。
4. **客户端入站数据**：`handleTCPDataBinary` 等待占位解析为真实 `net.Conn` 的窗口覆盖 **`DialTimeout + 缓冲`**，减少上游稍慢时用户字节被丢弃。

### 经验总结与审查清单

- **任意「客户端独占」的失败**（上游连不上、鉴权失败等）若会影响用户侧连接，必须有**对侧可见的 teardown**（本例为 TCPClose），否则易出现「一端已放弃、一端仍 Read 阻塞」的挂死。
- **审查清单**：[ ] 新协议下是否所有「建连失败」路径都会关闭用户 `sourceConn` 或等价复位？[ ] 占位 + 异步 `Dial` 时，入站数据等待时间是否覆盖 `Dial` 时长？

### 相关文件

- `internal/client/handlers.go`
- `internal/server/protocol/adapter.go`、`binary.go`、`legacy.go`、`binary_tcp_test.go`
- `internal/server/tunnel/tcp.go`、`http_hijack_integration_test.go`

## 2026-04-20: 服务端 per-client `tunnels` 与 CLI 隧道合并（默认同时支持）

### 背景

凭证鉴权下，**主连接**始终按客户端 CLI 的 `type` / `upstream` / `-p` / `-s` 建隧道，**不在服务端覆盖**鉴权内容。服务端 YAML 的 `tunnels` 通过 **GetToken** 与鉴权成功响应带给协调进程；客户端用 `AuthSnapshotFromOptions` + `MatchTunnelSpecIndex` 判断当前进程对应 YAML 的哪一行（若有），对其余行 `ChildOptionsFromSpec` 拉起子会话。子会话在鉴权里设 `opaqueChild`，**GetToken** 不再附带 `tunnels` 列表，避免递归拉起。

### 做法要点

1. `GetTokenOptions.OpaqueChild` / `Authentication.opaqueChild` / `Options.OpaqueChild`：仅子会话为 true。
2. **Monitor**：不再对入站 `auth` 做 `ApplyTunnelSpecToAuthentication`；`includeTunnelList` = 凭证且 YAML 有条目且非 opaqueChild。
3. **客户端**：`spawnServerConfiguredTunnels` 对 `i != myIdx` 的 YAML 行启动进程；`myIdx == -1` 时启动全部 YAML 行对应的子会话。

### 审查清单

- [ ] 主连接是否始终等于用户 CLI？
- [ ] 子会话是否不带 `config.tunnels`？

### 经验总结

- **不要**在 monitor 里用 YAML 覆盖首包 `auth`，否则「服务端 tunnels」与「客户端自己指定隧道」无法并存；合并策略应是主进程按 CLI 建链，YAML 仅用于列出**额外**会话并在协调进程里 `spawn`。
- **`opaqueChild`**：子进程必须在鉴权里标记，且 **GetToken** 对子连接不返回 `tunnels`，否则子进程会再按 YAML 拉一层，递归爆炸。
- **匹配**：`MatchTunnelSpecIndex` + `tunnelSpecMatchesAuth` 需与 `ApplyTunnelSpecToAuthentication` / `SyncOptsFromTunnelSpec` 对 `remotePort`、`subDomain` 的规则一致（如 TCP `remotePort==0` 表示沿用客户端 `-p`）；自动起的子会话若需固定公网端口，YAML 中应写死 `remotePort`（`ChildOptionsFromSpec` 对 `remotePort==0` 会报错并提示单独起客户端或补配置）。

### 相关文件

- `cmd/inlets/server.go`、`cmd/server/main.go`
- `internal/server/channels/monitor/auth.go`
- `internal/client/tunnel_spec_auth.go`（`AuthSnapshotFromOptions`、`ParseUpstream`）、`handlers.go`、`child_options.go`、`client.go`
- `conf/example/server.yaml`

## 2026-04-22: Client transport mode split (`--server` for v2, `--remote` for legacy)

### Background

The client now supports two explicit transport modes:

1. **v2 mode** uses `--server` as a URL (http/https, optional path prefix).
2. **legacy mode** uses `--remote` + `--remote-tcp-port` with `--legacy`.

This removes ambiguous combinations and makes websocket endpoint resolution predictable.

### Key changes

1. Added `Options.Server` and normalized `--server` parsing in CLI (`http://host`, `http://host:port/path`, `https://host/path`, etc.).
2. Introduced mode validation:
   - `--server` + `--legacy` is rejected.
   - non-legacy + `--remote`/`--remote-tcp-port` is rejected.
3. In `--server` mode, 404 on `/_/monitor` no longer falls back to legacy and returns an explicit guidance error.
4. Added connection target builder tests to verify monitor/data/legacy websocket URLs with and without path prefix.
5. Updated user-facing docs (`README.md`, `conf/example/client.yaml`) to explain v2 vs legacy usage and examples.

### Review checklist

- [ ] Are `--server` URL variants normalized with default ports and preserved path prefix?
- [ ] Does `--server` mode avoid implicit legacy fallback on monitor 404?
- [ ] Are `--remote` and `--remote-tcp-port` used only in legacy mode?
- [ ] Do docs clearly distinguish v2 (`--server`) from legacy (`--legacy --remote ...`)?

### Related files

- `cmd/inlets/client.go`
- `cmd/inlets/client_test.go`
- `internal/client/client.go`
- `internal/client/client_targets_test.go`
- `internal/client/types.go`
- `internal/client/child_options.go`
- `README.md`
- `conf/example/client.yaml`

## 2026-04-24: Public (unauthenticated) monitor session TTL (not HTTP / edge auth)

### Background

A session time limit for **temporary** control-plane logins (monitor `authType` public: no `--token` / `--credentials`) must **not** be tied to HTTP tunnel type or public-URL (edge) auth. Those are separate product concerns.

### Behavior

- `shouldApplyPublicMonitorSessionTTL` — true only when monitor auth is not `token` or `credentials` (public / empty treated as unauthenticated to server).  
- **No** use of `auth.Type` (http/tcp) or merged HTTP edge auth.  
- Client: logs `warn`, exits without reconnect on close reason `public monitor session timeout` (legacy: `public http no-auth timeout`).  

### Related files

- `internal/server/channels/monitor/auth.go`, `auth_test.go`, `common.go`  
- `internal/client/client.go`, `monitor_close_test.go`  
- `conf/example/server.yaml`  
- `docs/features/PUBLIC_MONITOR_SESSION.md`

## 2026-04-25: HTTP tunnel — empty 401 without Content-Length (browser stuck “pending”)

### Symptom

Upstream returns `401` with `WWW-Authenticate` but **no** `Content-Length` / `Transfer-Encoding`, `Connection: keep-alive`, empty body. Client logs `-> 401 Unauthorized` but the browser tab stays loading and may not show Basic auth.

### Root cause

RFC 7230: without `Content-Length` or `Transfer-Encoding`, the end of the message is undefined unless the connection closes. Forwarding only header bytes on a keep-alive hijacked socket looks like an **incomplete** response; the browser waits for more bytes.

### Fix

For **non-chunked** upstream responses, buffer the body (bounded), and if empty, run `insertContentLength0IfUnframedEmpty` on the dumped head so the wire includes `Content-Length: 0` when headers lack framing. Chunked encoding still uses the streaming `resp.Body` reader.

### Related files

- `internal/client/http1_response_framing.go`, `http1_response_framing_test.go`  
- `internal/client/handlers.go` — `readUpstreamAndStreamHTTPResponse`

## 2026-04-25: Chunked 401 from upstream — de-chunked bytes vs `Transfer-Encoding: chunked` in headers

### Symptom

`curl` received status + `Transfer-Encoding: chunked` then **timed out with 0 body bytes**; client log already showed `-> 401`. Typical with **go-zoox-ingress** or similar.

### Root cause

For upstream **chunked** responses, the client’s tunnel path was sending **de-chunked** body bytes to the browser while the serialized **head still contained** `Transfer-Encoding: chunked`. The peer’s chunked decoder then waits for valid chunk **terminators** that never come.

### Fix

`readUpstreamAndStreamHTTPResponse` always `ReadAll`s the body (bounded), then `responseHeadForBufferedUpstreamBody` strips `Transfer-Encoding`, sets `Content-Length` to the buffered length, and re-dumps the head with `httputil.DumpResponse` before sending head/body frames. For an **empty** body (e.g. 401 with no content), it also sets **`Connection: close`** so reverse proxies that mis-handle keep-alive + framing still complete the message.

**Deploy**: Browsers/curl will keep seeing the old `Transfer-Encoding: chunked` hang until the **inlets client** process is rebuilt from this code and restarted; the server binary alone does not apply this logic.

### Related files

- `internal/client/http1_response_framing.go` — `responseHeadForBufferedUpstreamBody`  
- `internal/client/handlers.go`  
- `internal/client/http1_response_framing_test.go` — `TestResponseHeadForBufferedUpstreamBodyStripsChunked`
