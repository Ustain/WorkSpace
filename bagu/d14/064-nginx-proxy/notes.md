# 64. 反向代理与 upstream · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **Nginx 最常见用法**：客户端只看见 Nginx，后面是一群 upstream 应用。

- **反向代理**：代理后面的服务，对客户端隐藏。
- **正向代理**：代理客户端访问外网（公司出口）。
- **upstream**：后端服务器组，配合 `proxy_pass`。

## 2. 核心机制

```
location /api/ {
    proxy_pass http://backend;
}
upstream backend {
    server 10.0.0.1:8080;
    server 10.0.0.2:8080;
}
```

Nginx 处理：连接 → 选 location → 反代到 upstream → 回写客户端。

还能做 TLS 终结、静态资源、缓存、改头、限流（题 66）。

**卡点**

- `proxy_pass` 末尾斜杠会改 URI，配错 404。
- 后端要拿到真实 IP：`X-Forwarded-For` / `Real-IP`，注意伪造。

## 3. 生产坑 / 反例

- **upstream 无健康检查**：死节点还在打（`max_fails`/`fail_timeout`）。
- **缓冲默认把大响应堆内存/临时文件**：要按接口调 `proxy_buffering`。
- **WebSocket 没升级头**：长连接失败。

## 4. 60 秒口述

> Nginx 反向代理把请求转到 upstream 一组后端，客户端不直接碰应用。顺便做 HTTPS、静态、负载均衡。  
> 和正向代理方向相反。配 proxy_pass 注意斜杠和真实 IP 头。后端挂了要有失败重试和摘除。
