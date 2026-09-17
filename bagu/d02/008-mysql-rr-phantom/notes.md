# 8. RR 如何（以及不能完全）解决幻读 · 口试

## 1. 属于哪块？和相邻方案差在哪？

**幻读**：同一事务内，**同一条件** 两次查询，**结果集行数/集合** 不同（别人 **插入或删除** 了满足条件的行）。和不可重复读（**同一行** 被改）不同。

标准 SQL 里 **RR 仍可能发生幻读**。InnoDB 在 RR 下 **加强** 了，但要分 **快照读** 和 **当前读**，不能说「RR 绝对没有幻读」。

## 2. 核心机制：两条防线

**① 快照读（普通 SELECT）→ MVCC 防幻读**

RR 下 **第一次 SELECT** 创建 Read View，之后复用。

- 别的事务 **之后插入** 的行：`trx_id >= max_trx_id` → 对 T1 **不可见**
- T1 再次 `SELECT ... WHERE age>20` → 仍看到同一快照 → **结果集不变**

```
T1: BEGIN; SELECT * FROM t WHERE age>20;  -- 5 行，建 Read View
T2: INSERT age=25; COMMIT;
T1: SELECT * FROM t WHERE age>20;         -- 仍 5 行 ✓（快照读无幻读）
```

**② 当前读（FOR UPDATE / LOCK IN SHARE MODE / UPDATE / DELETE）→ 锁防幻读**

读 **最新已提交版本** 并 **加锁**。InnoDB 用 **next-key lock（记录锁 + 间隙锁）** 锁住索引范围，阻塞别的事务在间隙 **插入**。

```
T1: BEGIN; SELECT * FROM t WHERE age>20 FOR UPDATE;  -- 加 next-key
T2: INSERT age=25;  -- 阻塞，等 T1 提交
```

D3 题 13 细讲 next-key；这题记：**当前读靠锁，不靠 MVCC**。

**为什么说「不能完全」**

| 场景 | 结果 |
|------|------|
| RR + 两次 **快照读** | InnoDB 一般 **无幻读** |
| RR + 两次 **当前读** | next-key **可防** 插入型幻读 |
| 先快照读、再当前读 | 第二次可能看到新行 → **像幻读** |
| T1 快照读没有，T2 插入，T1 **UPDATE ... WHERE** 命中新行 | 更新了「幻行」→ 常叫 **更新幻读** |
| 要所有读写法都严格无幻读 | 用 **Serializable** |

**更新幻读例子**

```
T1: BEGIN; SELECT * FROM t WHERE id=5;  -- 空（快照）
T2: INSERT id=5; COMMIT;
T1: UPDATE t SET c=c+1 WHERE id=5;    -- 影响 1 行！
T1: SELECT * FROM t WHERE id=5;       -- 能看到了
```

快照读时 id=5 不可见，但 UPDATE 是 **当前读 + 写**，会碰到 T2 已提交的行。

## 3. 生产坑 / 反例

- **口试死背「RR 无幻读」**——要对快照读、当前读、更新幻读分开说。
- **以为 MVCC 万能**：`FOR UPDATE` 不走快照，靠锁。
- **RC 下快照读**：每次新 Read View，**会有幻读**（题 6、7 连贯）。
- **完全避免**：业务用 Serializable 或 **SELECT FOR UPDATE 锁范围** 再查再改。

## 4. 60 秒口述

> 幻读是同一条件两次读，行集变了。InnoDB RR 下，普通 SELECT 用 MVCC：Read View 复用，后插入的行不可见，快照读无幻读。当前读用 next-key 锁范围，阻止别人插入，防当前读幻读。但不能说 RR 完全没有：先快照后当前读会看到新行；UPDATE 可能更新到别人新插入的行，叫更新幻读。要严格无幻读用 Serializable 或显式锁范围。
