# 常见问题

## v2 和 legacy？

**v2** 使用 **`--server`** 传入 `http(s)` URL。**legacy** 使用 **`--legacy`** + **`--remote`** + **`--remote-tcp-port`**。二者不要混用。

## 为什么会出现「public monitor session timeout」？

未传 **`--token`** 或 **`--credentials`** 时，服务端可能对临时监控会话限时。请改用凭据或 token，或调整 `publicHTTPNoAuth`。见 [公共监控会话时限](/zh/features/PUBLIC_MONITOR_SESSION)。

## 浏览器或 curl 经隧道一直转圈

请升级客户端与服务端；新版本修复了空体/分块响应帧、回调竞态等问题。若仍复现，请抓日志与上游响应头。

## TCP 连上但无数据

确认**本地上游**已监听；上游晚启动时请让用户侧**重试连接**。

## 能否多条隧道？

可以：多进程，或在 **credentials** 下使用服务端 YAML **`clients[].tunnels`** 自动起子会话（见 [客户端 · 进阶](/zh/client/advanced)）。

## 协议哪里讲得更细？

见 [新协议说明](/zh/features/NEW_PROTOCOL_ISSUES)。

## 如何源码安装？

见 [安装](/zh/guide/install)。
