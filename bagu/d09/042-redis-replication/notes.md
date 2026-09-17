# 42. 主从复制：全量与增量 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **Redis 副本怎么追上主**。和 MySQL binlog 类似但实现不同：全量靠 RDB，增量靠 backlog。

## 2. 核心机制

**全量（PSYNC 失败或第一次）**

1. 从库发 PSYNC。
2. 主库 BGSAVE 打快照，同时把之后的写记在 **replication buffer**。
3. 把 RDB 传给从库，从库载入（期间通常不能服务，看磁盘载入）。
4. 再放增量。

**增量（部分重同步）**

- 主库维护 **repl backlog** 环形缓冲 + replication offset + runid。
- 从库断线重连，offset 还在 backlog 里 → 只发缺的那段。
- backlog 太小、断太久 → 退回全量。

**卡点**

- 全量时主库 fork、传输大 RDB，从库 load，都是抖动点。
- 写流量大 + backlog 小 = 频繁全量。

## 3. 生产坑 / 反例

- **从库持久化关闭 + 重启**：容易反复全量。
- **一条慢从库**：replication buffer 把主库内存打满（client-output-buffer）。
- **读写分离读从**：复制异步，刚写完立刻读从可能没有。

## 4. 60 秒口述

> Redis 复制先尝试部分重同步：靠 backlog 和 offset 补洞。补不上就全量：主库 BGSAVE 传 RDB 再放增量。  
> backlog 要够大，避免反复全量。全量会 fork 和载入，主从都抖。复制是异步的，读从库要接受延迟。
