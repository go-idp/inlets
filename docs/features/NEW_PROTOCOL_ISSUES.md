# Inlets Go Server 新协议问题分析

本文档记录了 inlets go server 新协议（v2 / 2.0.0）实现中存在的问题。

## 最新修复状态（2026-04-20）

以下与 **`NEW_PROTOCOL_ISSUES` 概览表中 🔴 高优先级及原 🟡 数据通道/流重组** 相关的项已处理或澄清：

1. **流式传输缺少 onComplete / AddChunk 自动创建（已修复）**
   - **根因**：`StreamManager.AddChunk` 在流不存在时调用 `CreateStream`，持锁重入导致 **死锁**，且 `onComplete` 为 nil 时重组完成无法投递。
   - **修复**：去掉自动创建；新增 **`EnsureStream`**（幂等、先注册回调再收 chunk）；`handleBinaryMessage` 在流式路径上若缺少 handler 则 **返回错误**，不再创建空流；无 TCP handler 的流式 TCP 直接报错。
   - **相关代码**：`internal/server/protocol/stream_manager.go`、`internal/server/protocol/binary.go`；测试：`stream_manager_test.go`。

2. **流式传输序列号（澄清，非缺陷）**
   - 分片发送使用 **每个逻辑流 `streamId` 下从 0 递增的 chunk 序号**；不同请求/流使用不同 `streamId`，与 `StreamManager` 按 `streamId` 隔离的重组一致，**不存在跨流序号冲突**。

3. **流控 Send 竞态（已缓解）**
   - 发送路径已使用 **`TrySend` 原子扣减窗口**（见 `sendStreaming` / `sendStreamingViaDataChannel`）；`Send` 委托 `TrySend`。`CanSend` 仅保留只读语义，**发送侧勿用 `CanSend` + `Send` 组合**。

4. **数据通道断开与流清理（已对齐）**
   - `defer` 中通过 **`RemoveDataChannelForStream`** 清理每流 data WS 时，适配器内已 **`RemoveStream`（流管理器）+ 流控窗口**（见 `binary.go`）。`handleConnection` 的 defer 改为 **`Get(containerId)` 取最新映射** 再删 map / 调 `RemoveDataChannelForStream`，避免陈旧 `*TunnelMapping` 指针。

5. **数据通道 Upgrade 前后竞态（已修复）**
   - **`Upgrade` 成功后再次 `Get` container 并校验 `ClientId`**；若隧道已销毁则关闭连接，避免向已释放映射注册 `DataSockets`。

6. **流重组长期卡住（已缓解）**
   - 为 **`Stream` 记录 `createdAt` / `lastActivity`**（每次 `AddChunk` 更新）；`StreamManager.cleanup` 周期性驱逐 **超过 `stallTimeout`（默认 2m）无新 chunk** 或 **超过 `maxStreamAge`（默认 5m）** 的非完成流，并可选调用 **`OnError`** 后 `RemoveStream`。

---

## 最新修复状态（2026-03-15）

以下与“高并发下 HTTPS 请求 pending”直接相关的问题已修复：

1. **HTTP 回调注册竞态（已修复）**
   - 修复方式：请求发送前先注册回调，避免响应先到导致回调丢失。
   - 相关代码：`internal/server/tunnel/http.go`

2. **请求长期挂起无兜底（已修复）**
   - 修复方式：增加请求超时保护，超时返回 `504 Gateway Timeout`。
   - 相关代码：`internal/server/tunnel/http.go`

3. **回调重复消费/残留风险（已修复）**
   - 修复方式：新增 `Take(tcpId, requestId)` 原子取出并删除接口。
   - 相关代码：`internal/server/container/callback.go`, `internal/server/channels/monitor/auth.go`

4. **客户端 EOF 依赖导致 keep-alive 场景卡住（已修复）**
   - 修复方式：客户端按 HTTP 协议解析响应，不再依赖 EOF。
   - 相关代码：`internal/client/handlers.go`

新增测试：

- `internal/server/container/callback_test.go`
- `internal/server/channels/monitor/auth_test.go`
- `internal/server/tunnel/http_test.go`

## 问题概览

| 优先级 | 问题 | 位置 | 影响 |
|--------|------|------|------|
| 🟢 已澄清 | 流式传输序列号（按 streamId 隔离） | `protocol/binary.go` | 见 2026-04-20 节 |
| 🟢 已修复 | 流式传输 EnsureStream / 禁止 nil onComplete | `protocol/stream_manager.go` | 见 2026-04-20 节 |
| 🟢 已缓解 | 流控发送路径统一 TrySend | `protocol/flow_controller.go` | 见 2026-04-20 节 |
| 🟢 已缓解 | 数据通道断开清理（defer + RemoveDataChannelForStream） | `channels/data/new.go` | 见 2026-04-20 节第 4–5 点 |
| 🟢 已缓解 | 流重组停滞驱逐（stall / max age） | `protocol/stream_manager.go` | 见第 6 点 |
| 🟢 低 | 错误处理不完整 | 多处 | 可维护性 |
| 🟢 低 | 忙等待优化 | `protocol/binary.go` | 性能问题 |

---

## 详细问题分析

### 1. 🔴 流式传输序列号不一致

**位置：** `internal/server/protocol/binary.go:685, 761`

**问题描述：**
发送端使用 chunk 索引作为 sequence，但接收端可能期望全局递增序列号。

**代码：**
```go
// 发送时使用 chunk index 作为 sequence
sequence := uint32(i)  // i 是 chunk 索引 (0, 1, 2, ...)
```

**影响：**
- 如果多个流式传输同时进行，序列号可能冲突
- 接收端流重组可能失败
- 可能导致数据丢失或乱序

**建议修复：**
使用全局序列号或 streamId + chunkIndex 的组合作为唯一标识。

---

### 2. 🔴 流式传输缺少 onComplete 回调注册

**位置：** `internal/server/protocol/stream_manager.go:216-229`

**问题描述：**
`AddChunk` 会自动创建 stream，但没有设置 `onComplete` 回调，导致流完成后数据无法传递给处理器。

**代码：**
```go
func (sm *StreamManager) AddChunk(streamId string, sequence int, data []byte, isLast bool) {
    if stream == nil {
        stream = sm.CreateStream(streamId, nil, nil)  // onComplete 是 nil
    }
    stream.AddChunk(uint32(sequence), data, isLast)
}
```

**影响：**
- 流式传输的数据无法被正确处理
- 功能不完整

**建议修复：**
在 `handleBinaryMessage` 中，如果检测到流式传输，需要先创建 stream 并注册回调。

---

### 3. 🔴 流控窗口的竞态条件

**位置：** `internal/server/protocol/flow_controller.go:48-84`

**问题描述：**
`CanSend` 和 `Send` 之间存在竞态条件，多个 goroutine 可能同时通过检查并发送数据，导致窗口超限。

**代码：**
```go
func (fc *FlowController) CanSend(streamId string, size int) bool {
    // ... 检查窗口
    return window.SendWindow+size <= window.MaxWindowSize
}

func (fc *FlowController) Send(streamId string, size int) bool {
    if !fc.CanSend(streamId, size) {  // 再次检查，但中间可能被修改
        // ...
    }
    window.SendWindow += size  // 可能超限
}
```

**影响：**
- 可能导致流控失效
- 内存使用可能超出限制
- 可能导致数据损坏

**建议修复：**
将 `CanSend` 和 `Send` 合并为原子操作，或使用更细粒度的锁。

---

### 4. 🟡 流控窗口未正确清理

**位置：** `internal/server/channels/data/new.go:97-114`

**问题描述：**
数据通道断开时，只清理了数据通道连接，但没有清理流控窗口和流管理器中的 stream。

**代码：**
```go
defer func() {
    // 清理数据通道
    delete(container.DataSockets, streamId)
    delete(container.DataWriteMu, streamId)
    // 但没有清理流控窗口和流管理器中的 stream
}()
```

**影响：**
- 资源泄漏
- 流控窗口可能累积
- 内存使用持续增长

**建议修复：**
在清理数据通道时，同时清理对应的流控窗口和流管理器中的 stream。

---

### 5. 🟡 数据通道验证的竞态条件

**位置：** `internal/server/channels/data/new.go:45-58`

**问题描述：**
在验证 container 存在后、WebSocket 升级之前，container 可能被删除，导致后续处理失败。

**代码：**
```go
container := h.ctx.Container.Get(containerId)
if container == nil {
    http.Error(w, "Container not found", http.StatusNotFound)
    return
}

// 在 Upgrade 之前，container 可能被删除
conn, err := h.upgrader.Upgrade(w, r, nil)
```

**影响：**
- 可能导致 panic 或错误处理
- 稳定性问题

**建议修复：**
在 Upgrade 后再次验证 container，或使用引用计数机制。

---

### 6. 🟡 流式传输的流重组可能失败

**位置：** `internal/server/protocol/stream_manager.go:73-115`

**问题描述：**
如果中间 chunk 丢失，`tryReassemble` 会停止重组，后续 chunk 无法处理，可能导致流卡住。

**代码：**
```go
func (s *Stream) tryReassemble() {
    for {
        chunk, exists := s.Chunks[s.ExpectedSequence]
        if !exists {
            break  // 如果序列号不连续，会停止重组
        }
        // ...
    }
}
```

**影响：**
- 流可能永久卡住
- 需要超时机制或重传机制

**建议修复：**
- 添加超时机制
- 实现 chunk 重传请求
- 或记录缺失的 chunk，等待重传

---

### 7. 🟢 监控通道在认证前处理二进制消息

**位置：** `internal/server/channels/monitor/new.go:67-81`

**问题描述：**
未认证时收到二进制消息会被忽略，但不会记录或拒绝，可能导致客户端误以为消息已处理。

**代码：**
```go
if messageType == websocket.BinaryMessage {
    if isAuthenticated {
        // 处理二进制消息
    }
    // 未认证时直接忽略，没有日志或错误响应
}
```

**影响：**
- 调试困难
- 客户端可能误以为消息已处理

**建议修复：**
记录警告日志或发送错误响应。

---

### 8. 🟢 HTTP 请求在监控通道的处理问题

**位置：** `internal/server/channels/monitor/new.go:126-148`

**问题描述：**
新协议应该通过数据通道发送 HTTP 请求，但监控通道仍然处理 `request` 事件，可能导致混淆。

**代码：**
```go
case "request":
    // Handle HTTP request (base64-encoded binary for new protocol)
    // 新协议应该通过数据通道，这里可能是兼容性处理
```

**影响：**
- 协议设计不清晰
- 可能导致实现混乱

**建议修复：**
明确标记为兼容性处理，或移除该处理逻辑。

---

### 9. 🟢 流控窗口的自动初始化可能导致死锁

**位置：** `internal/server/protocol/flow_controller.go:54-60`

**问题描述：**
在持有读锁时释放并重新获取锁，可能导致死锁或竞态。

**代码：**
```go
if window == nil {
    fc.mu.RUnlock()
    fc.InitializeStream(streamId)  // 需要写锁
    fc.mu.RLock()
}
```

**影响：**
- 可能导致死锁
- 竞态条件

**建议修复：**
使用 `sync.Map` 或重新设计锁策略。

---

### 10. 🟢 流式传输的流控检查使用忙等待

**位置：** `internal/server/protocol/binary.go:676-682`

**问题描述：**
使用 `time.Sleep` 进行忙等待，可能阻塞 goroutine。

**代码：**
```go
for !a.flowController.CanSend(streamId, len(chunk)) {
    time.Sleep(50 * time.Millisecond)  // 忙等待
}
```

**影响：**
- 性能问题
- 资源浪费

**建议修复：**
使用 channel 或条件变量实现更高效的等待机制。

---

### 11. 🟢 二进制消息解析错误处理不完整

**位置：** `internal/server/protocol/binary.go:320-326`

**问题描述：**
解析失败时只返回错误，没有日志记录，难以排查问题。

**代码：**
```go
func (a *BinaryProtocolAdapter) HandleBinaryMessage(message []byte) error {
    binaryMsg, err := ParseBinaryMessage(message)
    if err != nil {
        return err  // 只返回错误，没有日志
    }
    return a.handleBinaryMessage(binaryMsg)
}
```

**影响：**
- 调试困难
- 问题排查不便

**建议修复：**
添加日志记录，包括错误信息和原始消息的前几个字节。

---

### 12. 🟢 数据通道连接失败时的回退机制缺失

**问题描述：**
如果数据通道连接失败，新协议没有回退到监控通道的机制，可能导致功能不可用。

**影响：**
- 容错性差
- 用户体验不佳

**建议修复：**
实现回退机制，在数据通道不可用时自动使用监控通道。

---

## 修复建议优先级

### 高优先级（立即修复）
1. 流式传输序列号不一致
2. 流式传输缺少 onComplete 回调
3. 流控窗口竞态条件

### 中优先级（尽快修复）
4. 流控窗口未正确清理
5. 数据通道验证竞态条件
6. 流式传输流重组失败处理

### 低优先级（逐步优化）
7. 错误处理完善
8. 忙等待优化
9. 其他改进

---

## 测试建议

针对以上问题，建议添加以下测试：

1. **流式传输测试**
   - 多流并发传输
   - 序列号冲突场景
   - Chunk 丢失场景

2. **流控测试**
   - 并发发送压力测试
   - 窗口超限场景
   - 资源清理验证

3. **竞态条件测试**
   - 并发连接/断开
   - 并发数据发送
   - Container 生命周期测试

4. **错误处理测试**
   - 消息解析失败
   - 连接异常断开
   - 数据通道连接失败

---

## 相关文件

- `internal/server/protocol/binary.go` - 二进制协议适配器
- `internal/server/protocol/stream_manager.go` - 流管理器
- `internal/server/protocol/flow_controller.go` - 流控制器
- `internal/server/channels/monitor/new.go` - 新协议监控通道
- `internal/server/channels/data/new.go` - 新协议数据通道

---

**最后更新：** 2024-12-19  
**文档版本：** 1.0

