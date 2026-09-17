# 18. Explain 关键字段怎么看 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **优化器选了什么执行计划**。慢 SQL 先 Explain，再谈加索引。`EXPLAIN ANALYZE`（8.0）还能看真实耗时。

和「慢日志」比：慢日志告诉你慢了；Explain 告诉你 **怎么扫的**。

## 2. 核心机制：口试只抓这几列

**id / select_type**

- id 大的先执行（子查询）；SIMPLE 最常见。
- `DEPENDENT SUBQUERY` 往往是相关子查询，容易慢。

**table**

- 驱动表是谁。多表 JOIN 时 **小结果集当驱动** 才合理。

**type（访问类型，必背从差到好）**

`ALL` < `index` < `range` < `ref` < `eq_ref` < `const/system`

- **ALL**：全表扫。
- **index**：扫整棵索引（覆盖全索引）。
- **range**：索引范围（`>`、`BETWEEN`、`IN`）。
- **ref**：非唯一索引等值。
- **eq_ref**：JOIN 时唯一/主键等值，一行对一行。
- **const**：主键/唯一等值，最多一行。

面试可以说：OLTP 希望 `ref/eq_ref/const`；`ALL` 必须解释为什么能接受。

**possible_keys / key**

- possible 是候选；**key 才是真正用的**。key 为 NULL 就是没走索引。

**key_len**

- 用了联合索引多长。能判断最左用到第几列（注意字符集：utf8mb4 一字符 4 字节）。

**rows**

- 估计扫描行。和真实差一个数量级 → 统计信息过期，`ANALYZE TABLE`。

**Extra（高频）**

| Extra | 含义 |
|-------|------|
| Using index | 覆盖索引，不回表 |
| Using index condition | ICP，索引层多过滤 |
| Using where | Server 层再过滤 |
| Using filesort | 额外排序 |
| Using temporary | 临时表（GROUP/DISTINCT 等） |
| Using join buffer | JOIN 没很好用上索引 |

## 3. 生产坑 / 反例

- **只看 type 不看 rows**：`range` 但 rows=半张表，仍然慢。
- **type=index 以为很好**：那是索引全扫，大表照样慢。
- **统计信息陈旧**：计划选错索引，看起来「有索引却没用」。

## 4. 60 秒口述

> Explain 先看 type、key、rows、Extra。type 从 ALL 到 const，OLTP 要避免 ALL；key 为 NULL 就是没走索引；rows 是估计扫描量；Extra 里 Using index 是覆盖，filesort/temporary 要警惕。  
> 联合索引看 key_len 判断最左用到哪。计划离谱就 ANALYZE TABLE。8.0 用 EXPLAIN ANALYZE 对真实时间。
