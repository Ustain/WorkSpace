# 34. 内存淘汰 LRU / LFU · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **maxmemory 到了怎么踢 key**。过期是 TTL；淘汰是内存不够。没设 maxmemory，Redis 会一直用到 OS 杀它。

## 2. 核心机制

**常见策略**

| 策略 | 行为 |
|------|------|
| noeviction | 写报错，读还在 |
| allkeys-lru | 所有 key 里踢最久未用 |
| volatile-lru | 只在 **带 TTL** 的 key 里 LRU |
| allkeys-lfu | 所有 key 踢访问频率低的 |
| volatile-lfu | 仅 TTL key 里 LFU |
| volatile-ttl | 踢快过期的 |
| volatile-random / allkeys-random | 随机 |

缓存场景常用 **allkeys-lru** 或 **allkeys-lfu**。把 Redis 当存储时常用 noeviction，避免把持久数据踢掉。

**近似 LRU**

- Redis 不维护精确链表（太贵），是抽样若干 key 踢最旧的。
- LFU 用访问计数，更抗「扫一次就变热」。

**卡点**

- volatile-* 要求 key 有过期时间；没 TTL 的永远不在候选集。
- 淘汰发生在主线程，瞬时延迟会升。

## 3. 生产坑 / 反例

- **缓存和锁/队列共用一个实例 + allkeys-lru**：锁 key 被踢 → 互斥失效。要拆实例或 noeviction + 监控。
- **maxmemory 设太满**：碎片 + 复制积压，实际提前 OOM。
- **以为 LRU 精确**：是近似抽样。

## 4. 60 秒口述

> 内存到 maxmemory 按策略淘汰。缓存用 allkeys-lru 或 lfu；有 TTL 的才走 volatile-*；当存储用 noeviction。  
> Redis 的 LRU 是抽样近似。锁和缓存不要混在会淘汰的实例里。过期和淘汰是两件事。
