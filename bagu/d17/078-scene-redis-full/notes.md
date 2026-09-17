# 78. Redis 内存打满 · 口试

## 1. 现象

写报 OOM / `OOM command not allowed`；淘汰狂飙；延迟升高；实例被 OS kill；监控 used_memory ≈ maxmemory。

## 2. 止血

- 若 `noeviction`：临时改淘汰策略或扩容 maxmemory（机器要有余量）。
- 紧急删已知大 key：`UNLINK`，不要 `KEYS`+`DEL`。
- 限流写路径、关掉会灌数据的任务。
- 主从先保主可写，别在高峰全量同步。

## 3. 定位

- `INFO memory`、碎片率、`MEMORY DOCTOR`。
- 大 key / 过期没删的冷数据 / 突发写入 / 副本积压缓冲。
- 是否 fork 写时复制把 RSS 打爆（看起来像内存满）。

## 4. 根治

- 设合理 maxmemory + LRU/LFU，缓存与锁拆实例。
- 拆大 key，TTL + 随机，业务限大小。
- 监控：内存、淘汰数、拒绝写、大 key 巡检。

## 5. 60 秒口述

> 内存满先看策略：不能写就扩或开淘汰，删大 key 用 UNLINK，写限流。定位是大 key、没 TTL、碎片还是 fork。根治拆实例、TTL、巡检大 key。别 KEYS 扫描。
