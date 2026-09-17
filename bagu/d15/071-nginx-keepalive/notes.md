# 71. 长连接、缓冲、gzip · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **Nginx 连接和传输优化**。和限流不同：这题管 **连接复用、包大小、CPU**。

## 2. 核心机制

**keepalive**

- 对客户端：`keepalive_timeout`，减少握手。
- 对 upstream：`keepalive` 连接池，否则每个请求都和后端新建 TCP/TLS。
- HTTP/1.1 默认可复用；要配 `Connection` 头和连接池大小。

**缓冲**（题 66 已提，这里补与长连接关系）

- 慢客户端 + 不缓冲 = 上游连接被占满。
- 开缓冲保护上游，代价是延迟和临时磁盘。

**gzip**

- 压缩文本（json/html/js），降带宽，耗 CPU。
- 对已压缩的图片/视频不要再压。
- 小响应压了更慢（`gzip_min_length`）。

**卡点**

- upstream keepalive 没开：后端 TIME_WAIT 爆炸、TLS 握手占 CPU。
- gzip 在 Nginx 和在应用压两遍。

## 3. 生产坑 / 反例

- **WebSocket / gRPC** 要单独升级和超时，套用短 HTTP 超时会掐连接。
- **压缩级别 9**：CPU 打满，用 4～6。
- **keepalive 无限长**：僵尸连接占满 worker。

## 4. 60 秒口述

> 对客户端和 upstream 都要 keepalive，避免反复握手。缓冲保护后端不被慢客户端拖死。gzip 压文本省带宽但吃 CPU，小包和已压缩内容别压。  
> 长连接超时要按协议调，WebSocket 不能当短请求配。
