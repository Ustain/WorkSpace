# 45. Redis 阻塞常见原因 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **主线程被占满，所有请求 RT 一起升**。不是「网络偶尔抖一下」那么简单。排查先看延迟是实例级还是单命令。

## 2. 核心机制（按频率说原因）

1. **大 key / 热 key 命令**：`KEYS`、`HGETALL`、`SMEMBERS`、`DEL` 大对象。
2. **慢 Lua / 事务里一堆命令**。
3. **fork**：RDB、AOF rewrite，写时复制导致内存和 CPU 抖。
4. **AOF fsync always 或磁盘慢**；AOF rewrite 结束刷缓冲。
5. **过期风暴、淘汰风暴**：同一时刻删很多 key。
6. **主从全量传输 / 输出缓冲区**。
7. **内存达到 maxmemory 频繁淘汰**。
8. **透明大页 THP、swap**：经典运维坑。
9. **集群迁槽** 搬大 key。

**怎么查**

- `SLOWLOG GET`、`INFO commandstats`。
- 延迟监控 `redis-cli --latency`。
- `MEMORY USAGE`、`--bigkeys`。
- OS：iowait、swap、THP。

## 3. 生产坑 / 反例

- **用 KEYS 做定时扫描**：必阻塞。改 SCAN。
- **监控 AVG 不看 P99**：fork 尖刺被平均掉。
- **同一实例既缓存又持久化又当队列**：IO 和 fork 互相放大。

## 4. 60 秒口述

> Redis 阻塞几乎总是主线程没闲着：大 key、KEYS、慢 Lua、fork 持久化、过期/淘汰风暴、磁盘 fsync、swap。  
> 先看 SLOWLOG 和是否在 BGSAVE。治理是拆 key、SCAN、低峰持久化、关 THP、分开职责。6.0 I/O 线程救不了慢命令。
