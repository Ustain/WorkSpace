# 31. RDB vs AOF vs 混合持久化 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **Redis 怎么把内存落到盘，重启能恢复多少**。Redis 默认仍偏缓存语义：可以丢一点，换性能。

- **RDB**：某一时刻的二进制快照，小、恢复快，两次快照之间的修改会丢。
- **AOF**：写命令追加日志，丢得少（看 fsync 策略），文件大、恢复要重放。
- **混合**（4.0+）：AOF 文件开头是 RDB 快照，后面接增量 AOF，恢复又快丢得又少。

## 2. 核心机制

**RDB**

- `SAVE` 阻塞主线程（别在主库用）；`BGSAVE` **fork** 子进程写盘。
- 触发：`save 900 1` 这类规则，或手动、主从全量。
- 写时复制：fork 后父进程继续写，内核拷贝脏页，内存会抖、会突然多用。

**AOF**

- 命令写完进 AOF 缓冲，按 `appendfsync`：always / everysec / no。
- **everysec** 是常见折中：最多丢约 1 秒。
- always 最安全最慢，很少用。

**混合**

- `aof-use-rdb-preamble yes`：rewrite 时用 RDB 格式打底。

**卡点**

- 只开 RDB：崩溃丢最后一次快照后的数据。
- 只开 AOF everysec：丢 1 秒；rewrite 期间仍要理解 fork。

## 3. 生产坑 / 反例

- **fork 瞬间 RT 升高**：大实例写时复制，内存不够会失败或打 swap。
- **AOF 盘满**：Redis 可能阻塞写。
- **当数据库用却 everysec**：要接受丢秒级；金融状态该落 MySQL。

## 4. 60 秒口述

> RDB 是快照，恢复快、窗口内会丢；AOF 是命令日志，everysec 最多丢一秒；混合是 AOF 里嵌 RDB 头，恢复更快。  
> BGSAVE 和 AOF rewrite 都要 fork，大内存实例会抖。缓存可 RDB+AOF；不能丢的数据不要只靠 Redis。
