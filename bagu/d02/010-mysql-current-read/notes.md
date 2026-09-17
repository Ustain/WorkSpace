# 10. 当前读 vs 快照读 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **InnoDB 读走哪条路径**：读历史版本还是读最新并加锁。这是题 7、8 的接口层分类。

- **快照读**：普通 `SELECT`，读 Read View 下可见的版本，**不加锁**（一般情况）。
- **当前读**：读 **最新已提交行** 并 **加锁**，保证读到的行别人改不了（直到自己事务结束）。

和「隔离级别」比：隔离级别决定 Read View 怎么建；读类型决定 **走不走锁**。

## 2. 核心机制，卡在哪会出问题？

**快照读**

```sql
SELECT * FROM t WHERE id = 1;          -- 快照读
SELECT * FROM t WHERE age > 20;        -- 仍是快照读
```

- RC：每次 SELECT 新 View → 可能读到别人刚提交的修改。
- RR：第一次 SELECT 建 View，之后复用 → 同一行内容稳定。
- 不阻塞别人写（写的是新版本）；自己也不等别人的写锁（可见旧版即可）。

**当前读（必背清单）**

```sql
SELECT * FROM t WHERE id = 1 FOR UPDATE;       -- 排他当前读
SELECT * FROM t WHERE id = 1 LOCK IN SHARE MODE; -- 共享当前读（8.0 也可用 FOR SHARE）
UPDATE / DELETE / INSERT 在定位行时也是当前读
```

- 走最新数据 + 行锁；RR 下范围条件还会加 **间隙锁 / next-key**。
- `INSERT ... SELECT`、`REPLACE`、部分 DDL 扫描也可能当前读。

**对照**

| | 快照读 | 当前读 |
|--|--------|--------|
| 读什么 | 可见版本 | 最新已提交 |
| 锁 | 通常无 | 有（行/间隙/next-key） |
| 用途 | 查询展示 | 读完要改、防并发改 |
| 幻读 | RR 下 View 挡住新行 | 靠 next-key 挡插入 |

**卡点**

- 业务「先 SELECT 再 UPDATE」中间别人改了 → 快照读看不到，UPDATE 却命中（更新幻读，题 8）。
- 该加锁的地方用了普通 SELECT → 丢失更新。

## 3. 生产坑 / 反例

- **库存扣减只 SELECT 再算再 UPDATE**：必须 `SELECT ... FOR UPDATE` 或原子 `UPDATE ... WHERE stock>=1`。
- **把所有 SELECT 当当前读**：列表查询加 FOR UPDATE 会把间隙锁打满，吞吐崩、死锁多。
- **RC 下当前读**：没有 gap 锁（通常），幻读仍可能，但死锁比 RR 少——这是很多团队改 RC 的原因。

## 4. 60 秒口述

> 快照读是普通 SELECT，走 MVCC，读可见版本，不加锁。当前读是 FOR UPDATE、LOCK IN SHARE MODE，以及 UPDATE/DELETE 定位行，读最新并加锁。  
> 展示类查询用快照读；读完要改必须当前读或一条原子 UPDATE。RR 当前读会带间隙锁防插入，所以比 RC 更容易死锁。别把「查一下」和「锁住再改」混成同一种读。
