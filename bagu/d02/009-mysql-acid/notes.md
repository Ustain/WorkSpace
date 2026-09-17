# 9. ACID 各自靠什么实现 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **InnoDB 事务保证**，不是 SQL 语法。四个字母各靠一套机制，不要说「全靠锁」或「全靠日志」。

- **A Atomicity 原子性**：要么全成，要么全不成 → 靠 **undo log** 回滚。
- **C Consistency 一致性**：约束/业务不变量成立 → 靠 **应用约束 + 引擎约束 + 前三者配合**（不是单独一种日志）。
- **I Isolation 隔离性**：并发事务互不干扰到约定级别 → 靠 **锁 + MVCC**。
- **D Durability 持久性**：提交后宕机不丢 → 靠 **redo log**（崩溃恢复）+ 刷盘策略。

和「binlog 保证 ACID」比：binlog 是 **复制/时间点恢复**，不是 InnoDB 崩溃恢复的主路径。

## 2. 核心机制，卡在哪会出问题？

**原子性**

- 修改前把旧值写进 undo；ROLLBACK 沿 undo 把行改回去。
- 提交后 undo 还要留着给 MVCC 读旧版本，直到没人需要才 purge。
- 卡点：长事务不提交 → undo 堆积，表空间胀。

**隔离性**

- 快照读：MVCC + Read View（题 7）。
- 当前读 / 写：行锁、间隙锁、next-key（题 13）。
- 卡点：隔离级别选错（RC 不可重复读；RR 更多 gap 锁）。

**持久性**

- 改数据页先写 redo（WAL），commit 时 redo 刷盘（`innodb_flush_log_at_trx_commit`）。
- 崩溃后用 redo 重做已提交、用 undo 回滚未提交。
- 卡点：`innodb_flush_log_at_trx_commit=0/2` 更快，掉电可能丢最后几秒提交。

**一致性**

- 引擎：主键/唯一/外键/NOT NULL。
- 业务：转账借贷平衡要应用自己保证。
- 卡点：只开事务不等于业务一致；隔离级别不够仍可能读到「中间态」。

**和 binlog 的分工（面试常追）**

| | redo | undo | binlog |
|--|------|------|--------|
| 层 | InnoDB | InnoDB | Server |
| 干什么 | 崩溃重做 | 回滚 + MVCC | 复制 / PITR |
| 属于 ACID | D | A（兼 I） | 不是单机 ACID 核心 |

redo 与 binlog 还要用内部 2PC 对齐（题 12），否则主从和本地恢复对不齐。

## 3. 生产坑 / 反例

- **把 binlog 说成崩溃恢复**——崩溃恢复是 redo；复制是 binlog。
- **`innodb_flush_log_at_trx_commit=1` + `sync_binlog=1` 才是「双 1」**，最安全也最慢。
- **一致性不是隔离级别的副产品**：唯一约束仍可能在 RR 下被别的事务插入抢掉，要靠锁/唯一索引。

## 4. 60 秒口述

> ACID 拆开说：原子性靠 undo 回滚；隔离性靠锁加 MVCC；持久性靠 redo WAL，commit 刷 redo；一致性靠约束加业务，前三者配合。  
> binlog 不负责单机崩溃恢复，负责复制。生产要「不丢」就双 1，要吞吐就放宽刷盘，接受掉电丢最后一点提交。长事务会让 undo 胀，把原子性和隔离性的实现拖垮。
