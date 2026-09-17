# 21. count(*) / count(1) / count(col) · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **InnoDB 怎么数行**。MyISAM 表级有计数，`count(*)` 可以 O(1)；InnoDB 是 MVCC，必须数可见行，**不能 O(1)**。

## 2. 核心机制

**语义**

- `count(*)` / `count(1)`：数行（InnoDB 优化器通常同等对待，别背「* 更慢」）。
- `count(col)`：数 **col 非 NULL** 的行。col 有 NULL 时结果更小。
- `count(distinct col)`：去重，更贵。

**InnoDB 怎么执行**

- 选一个代价最小的索引扫叶子（尽量覆盖、更瘦的二级索引）。
- 每行还要判断对当前 Read View 是否可见，所以有未提交/旧版本时不能信一个「表行数」。
- `SHOW TABLE STATUS` 的 Rows 是估计值，不能当精确 count。

**卡点**

- 大表精确 count 就是慢；要精确就扫，要快就近似。

## 3. 生产坑 / 反例

- **「count(1) 比 count(*) 快」**——InnoDB 下过时结论。
- **实时 count 大表**：用 Redis 计数、汇总表、或 `EXPLAIN` 的 rows 近似。
- **count(col) 当总行数**：col 可空就会少算。

## 4. 60 秒口述

> InnoDB 没有表级精确行数，count 要扫索引并按 MVCC 判断可见行。count(*) 和 count(1) 基本一样；count(col) 不数 NULL。大表精确计数会慢，生产用缓存或汇总表。别把 TABLE STATUS 的 Rows 当准确值。
