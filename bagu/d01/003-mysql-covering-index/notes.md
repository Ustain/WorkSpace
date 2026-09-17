# 3. 覆盖索引与索引下推 · 口试

## 1. 属于哪块？和相邻方案差在哪？

都属于 **二级索引怎么少读聚簇树、少做无用回表**，是回表（题 2）的优化手段。

- **覆盖索引**：查询要的列 **索引叶子里都有** → **完全不回表**，直接从索引当结果集。
- **索引下推 ICP（Index Condition Pushdown）**：**不能覆盖**（还要其它列）时，把 **联合索引里靠后的列** 的过滤 **下推到存储引擎在索引层做**，减少 **回表次数**。MySQL 5.6+。

和「不用索引全表扫」比：都走索引。和覆盖比：ICP 仍要回表，只是回得少。

## 2. 核心机制，卡在哪会出问题？

**覆盖索引**

- 条件：`SELECT / WHERE / GROUP BY` 用到的列，都能从这棵索引的 **叶子** 拿到。
- InnoDB 二级索引叶子 **自带主键**，所以 `INDEX(name)` 实际叶子是 `(name, id)`。
- `SELECT id, name WHERE name='a'` → 叶子够用 → **不回聚簇**。
- `SELECT *` 或要索引里没有的列 → **不能覆盖**，必须回表。
- Explain：`Extra = Using index`（索引即数据，不是「用了索引」这么简单）。

**索引下推 ICP**

联合索引 `INDEX(name, age)`：

```sql
SELECT * FROM user WHERE name LIKE '张%' AND age = 20;
```

没有 ICP（老逻辑）：

1. Server 层在索引上只用 **最左列 name** 定位，扫到所有 `张*` 的 `(name, age, id)` 条目
2. **每条** 都回表取整行
3. Server 层再用 **age=20** 过滤 → 很多回表是白做的

有 ICP：

1. 引擎在 **索引叶子** 上同时用 `name LIKE '张%'` **和 age=20` 过滤
2. 只对 **命中的条目** 回表 → 回表次数大减

Explain：`Extra = Using index condition`（和 `Using index` 不同，ICP 仍可能回表）。

**区别对照**

| | 覆盖索引 | ICP |
|--|----------|-----|
| 是否回表 | 不回 | 仍回，但少回 |
| 条件 | 要的列全在索引里 | 联合索引，后置列在 WHERE |
| Extra | Using index | Using index condition |

## 3. 生产坑 / 反例

- **`SELECT *`**：几乎不可能覆盖；列表页改 **只查需要的列 + 联合索引**。
- **把 Using index 和 Using index condition 搞混**：前者不回表，后者只是索引层多过滤。
- **以为 ICP 能替代覆盖**：ICP 要 `SELECT *` 仍要回表；能覆盖就尽量覆盖。
- **联合索引列顺序**：ICP 只能推 **索引里已有、且已用到最左前缀** 之后的列；`(age, name)` 对 `WHERE name='张'` 可能走不了索引或推不了 name。

## 4. 60 秒口述

> 覆盖索引：查询列都在二级索引叶子里（含自动带的主键），引擎直接从索引读结果，不回聚簇，Explain 是 Using index。不是拿整行，是只拿查询要的那几列。  
> 索引下推：不能覆盖时，联合索引把 WHERE 里后置列的过滤推到 InnoDB 在索引叶子做，减少无效回表，Explain 是 Using index condition。  
> 生产上避免 SELECT *，列表查询用联合索引覆盖常查列；ICP 是兜底优化，覆盖索引更彻底。
