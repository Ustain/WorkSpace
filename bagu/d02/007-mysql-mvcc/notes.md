# 7. MVCC、Read View、版本链 · 口试

## 1. 属于哪块？和相邻方案差在哪？

**MVCC（Multi-Version Concurrency Control）**：用 **多版本 + 快照读**，让读写 **尽量不互斥**，实现 RC/RR 下的并发。

- **不是** 不用锁：写仍要锁；**当前读**（`SELECT ... FOR UPDATE`、纯 INSERT/UPDATE/DELETE 读）走锁，不走 MVCC 快照。
- **和 Serializable 比**：Serializable 读也加锁；MVCC 普通 SELECT 读 **旧版本**，不阻塞写（在可见性规则内）。

InnoDB 每行隐藏列：

- **DB_TRX_ID**：最后修改该行的事务 id
- **DB_ROLL_PTR**：指向 **undo log** 里上一个版本的指针 → 串成 **版本链**

## 2. 核心机制

**版本链（undo 链）**

```
当前行 (trx_id=100, balance=200)
    ↓ DB_ROLL_PTR
undo: (trx_id=80,  balance=150)
    ↓
undo: (trx_id=50,  balance=100)
```

UPDATE 不原地覆盖旧值给别的事务看，而是 **留 undo**，新事务按可见性规则决定读哪个版本。

**Read View（读视图）**

事务 **第一次快照读** 时生成（RR），或 **每条快照读** 都生成（RC）。核心字段（记概念即可）：

- **m_ids**：生成 Read View 时 **活跃（未提交）** 的事务 id 列表
- **min_trx_id**：m_ids 最小值（或当前最小活跃 id）
- **max_trx_id**：下一个将分配的事务 id（即当前已分配最大+1）
- **creator_trx_id**：创建该 Read View 的事务 id

**可见性规则（口试简化版）**

对版本链上某一版的 `trx_id`：

1. `trx_id == creator_trx_id` → **可见**（自己的修改）
2. `trx_id < min_trx_id` → **可见**（Read View 创建前已提交）
3. `trx_id >= max_trx_id` → **不可见**（Read View 创建后才开始的事务）
4. `min_trx_id <= trx_id < max_trx_id`：
   - 在 m_ids 里 → **不可见**（未提交）
   - 不在 m_ids 里 → **可见**（已提交）
5. 不可见 → 沿 **DB_ROLL_PTR** 找更早版本，直到可见或链结束

**RC vs RR 差别（必考）**

| | RC | RR |
|--|----|----|
| Read View | **每次** SELECT 新建 | **第一次** SELECT 创建，之后复用 |
| 同事务两次读同一行 | 可能不同（不可重复读） | 相同（可重复读） |
| 幻读（快照读） | 可能 | InnoDB 下同一 Read View，范围结果一致 |

## 3. 生产坑 / 反例

- **MVCC ≠ 无锁**：写锁、当前读锁、死锁仍会有。
- **长事务**：Read View 不释放 → undo 链堆积 → **undo 表空间膨胀**，影响 purge。
- **把所有 SELECT 都当 MVCC**：加 `FOR UPDATE` 是 **当前读**，读最新已提交/加锁版本，不走快照。
- **RC 改 RR 或反过来**：业务语义变，要压测；不是改个参数就完事。

## 4. 60 秒口述

> InnoDB 用 MVCC 实现快照读：行有 trx_id 和 roll_ptr，修改留 undo 形成版本链。Read View 判断哪个版本可见：自己的、View 前已提交的可见；View 后开始的或未提交的不可见，不可见就沿 undo 找旧版。RC 每次 SELECT 新 View；RR 第一次 SELECT 建 View 后复用，所以同事务 repeatable。MVCC 不替代写锁和当前读；长事务会导致 undo 堆积。
