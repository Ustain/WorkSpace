# 66. 限流与缓冲 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **边缘保护后端**。限流在 Nginx 做，比每个服务自己挡更省。缓冲避免慢客户端拖死 upstream。

## 2. 核心机制

**限流**

- `limit_req`：漏桶，限制 **请求速率**（QPS），可设 burst。
- `limit_conn`：限制 **并发连接数**。
- 键常用 `$binary_remote_addr`，也可用用户 id（要信任头）。

超限：返回 503，或 `nodelay` 处理 burst 行为。

**缓冲**

- `proxy_buffering`：Nginx 先收完/收一截后端响应再给客户端，保护上游。
- 上传 `client_max_body_size`、`client_body_buffer_size`。
- 大文件可关缓冲，流式转发。

**卡点**

- burst 太大等于没限。
- 按 IP 限流，公司 NAT 误伤。
- 只限入口一层，内部服务仍可能被打爆。

## 3. 生产坑 / 反例

- **限流 503 没有降级页/缓存** → 用户体验崩。
- **缓冲开太大磁盘打满**（proxy_temp）。
- **WebSocket/长轮询用 limit_req 按请求算不准**。

## 4. 60 秒口述

> Nginx 用 limit_req 限 QPS、limit_conn 限并发，burst 要克制。缓冲让慢客户端不要占着上游连接。  
> 限流键别只会 IP。这是止血层，后端还要有自己的过载保护。
