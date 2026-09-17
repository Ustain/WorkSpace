# 11. redo / undo / binlog 分工 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **InnoDB + Server 三套日志各管一件事**。面试最怕讲混。

| 日志 | 层 | 一句话 |
|------|----|--------|
| **redo** | InnoDB | 崩溃后 **重做** 已提交改动（持久性） |
| **undo** | InnoDB | **回滚** + MVCC 旧版本 |
| **binlog** | Server | **主从复制** + 时间点恢复 |

- redo 不是复制；binlog 不是崩溃恢复。
- undo 不是 redo 的反操作日志那么简单：它存旧镜像，给回滚和一致性读用。

## 2. 核心机制，卡在哪会出问题？

**一条 UPDATE 的顺序（简化）**

1. 把旧值写入 **undo**。
2. 改 buffer pool 里的数据页，同时写 **redo**（WAL：先日志后刷脏页）。
3. 事务提交：redo 刷盘（双 1 时），再写 **binlog**，内部 2PC 保证两者齐（题 12）。
4. 后台刷脏页；purge 回收不再需要的 undo。

**redo**

- 物理日志（页内偏移怎么改），循环写、顺序刷，崩溃用它重放。
- `innodb_flush_log_at_trx_commit`：1=每次提交 fsync；2=写 OS 缓存；0=每秒刷。

**undo**

- 逻辑旧版本；ROLLBACK 用它；快照读沿版本链用它。
- 提交后不能立刻删，等没有更老的 Read View。

**binlog**

- 逻辑日志（statement / row / mixed）。现代生产几乎 **ROW**。
- `sync_binlog=1` 每次提交刷盘。从库靠它重放。

**卡点**

- 只刷 redo 没刷 binlog：主库恢复有数据，从库没有 → 主从不一致（所以要内部 2PC）。
- undo 不 purge：历史链超长，查询沿着链走会变慢。

## 3. 生产坑 / 反例

- **「redo 同步到从库」**——错，从库吃 binlog。
- **binlog 格式用 STATEMENT**：不确定函数、触发器在从库结果可能不同；用 ROW。
- **双 1 不是免费**：每次 commit 两次 fsync，TPS 下降；金融账务用双 1，日志型业务可权衡。

## 4. 60 秒口述

> redo 管崩溃重做，undo 管回滚和 MVCC 旧版本，binlog 管复制和时间点恢复。  
> 一次提交：先 undo 留旧值，改内存页并写 redo，commit 刷 redo，再写 binlog，两者用内部两阶段提交对齐。  
> 口试别混：从库不看 redo。要绝对不丢用 innodb_flush_log_at_trx_commit=1 和 sync_binlog=1。
