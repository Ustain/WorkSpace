# 67. mapping：text vs keyword · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **ES 字段类型**。mapping 定错，后面查询/聚合全错，还要 reindex。

- **text**：分词，倒排给全文检索，**默认不能做聚合/排序**（除非 fielddata，很贵）。
- **keyword**：不分词，精确过滤、聚合、排序。

常见做法：字符串 **多字段** `title` text + `title.keyword`。

## 2. 核心机制

动态 mapping 会把字符串同时成 text+keyword（有长度限制）。显式 mapping 更可控。

**其他易混**

- `nested` vs object：数组对象要独立查用 nested。
- 数值、日期用对类型，不然范围和聚合错。
- `index: false` 只存不搜。

**卡点**

- 用 text 做 term 精确匹配：可能分词后对不上。
- 用 keyword 做中文全文：整句当一个词，搜不出来。

## 3. 生产坑 / 反例

- **上线后改 mapping 类型**：已有字段不能随便改，要 reindex。
- **无限动态字段**：mapping 爆炸，集群内存涨。
- **id、枚举、邮箱用 text**。

## 4. 60 秒口述

> text 分词用于搜；keyword 整段用于过滤聚合排序。字符串经常两个都要。mapping 一旦写进字段很难改，要 reindex。  
> 动态 mapping 方便但会炸字段。枚举和 id 用 keyword，文章内容用 text。
