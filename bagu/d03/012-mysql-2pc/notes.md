# 12. 两阶段提交如何保证 binlog 与 redo 一致 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **InnoDB redo 和 Server binlog 怎么一次提交、两次落盘还不撕裂**。不是分布式 XA 业务事务（那个也叫 2PC，但这题是引擎内部）。

为什么要 2PC：redo 和 binlog 是两份日志。只成功一份，崩溃后会出现：

- 主库有、binlog 无 → 从库永远缺这笔；
- binlog 有、redo 无 → 从库多、主库恢复后没有。

## 2. 核心机制，卡在哪会出问题？

**内部 2PC（prepare → commit）**

1. **prepare**：InnoDB 把 redo 写成 prepare 状态并刷盘（事务已持久，但还不能对外说提交成功）。
2. **写 binlog**：Server 把该事务事件写入 binlog 并按 `sync_binlog` 刷盘。
3. **commit**：InnoDB 把 redo 标成 commit（可延迟，靠 binlog 当裁判）。

**崩溃恢复谁说了算**

- binlog 里 **有** 完整事件 → 认为已提交，redo prepare 的事务 **提交**。
- binlog 里 **没有** → prepare 事务 **回滚**。

所以 **binlog 是最终裁决**。这保证：主库恢复结果 = 从库能重放的内容。

**组提交（group commit）**

多个事务排成队，一次 fsync 刷一波 binlog/redo，摊薄 fsync 成本。没有组提交，双 1 会把 TPS 打穿。

**卡点**

- prepare 后、写 binlog 前崩溃 → 回滚，客户端没收到成功，正确。
- binlog 写完、InnoDB commit 前崩溃 → 恢复时按 binlog 提交，客户端可能已收到成功或需重试幂等。

## 3. 生产坑 / 反例

- **把这题讲成 Seata/XA 跨库事务**——范围错了；先说「InnoDB 与 binlog 内部 2PC」。
- **只开 `sync_binlog=1` 却 redo 不刷**：仍可能丢。双 1 要两个参数一起。
- **半同步复制**：还要等至少一个从库收到 binlog，那是复制可靠性，叠在 2PC 之上。

## 4. 60 秒口述

> InnoDB redo 和 Server binlog 必须成对出现，否则主从会裂。内部两阶段：先 redo prepare 刷盘，再写 binlog 刷盘，最后 InnoDB commit。崩溃时以 binlog 有没有完整事务为准：有就提交 prepare，没有就回滚。  
> 组提交把多次 fsync 合成一次，不然双 1 太慢。这和业务上的分布式 2PC 不是同一件事。
