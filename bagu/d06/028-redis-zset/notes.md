# 28. ZSet：跳表 + listpack · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **有序集合怎么同时按 member 找和按 score 排**。为什么不是红黑树：跳表实现简单、范围扫描自然、和 Redis 风格匹配。

## 2. 核心机制

**元素少**：listpack 里紧凑存放 `member + score`，省内存。

**元素多**：两套结构一起：

1. **dict**：member → score，按成员 O(1) 查分。
2. **skiplist**：按 score（同分再比 member）有序，查排名、范围 O(log n)。

跳表多层链表：底层全量有序，上层是「快进」指针，期望层高随机。插入时更新各层前后指针。

**典型命令**

- `ZADD` / `ZSCORE` / `ZRANK`
- `ZRANGE` / `ZRANGEBYSCORE`（排行榜、延时队列按时间戳当 score）

**卡点**

- 同分：用 member 再比，保证唯一顺序。
- 和 list 的区别：list 按下标，zset 按分数；和 hash 的区别：hash 无序。

## 3. 生产坑 / 反例

- **排行榜一个 zset 塞全站用户**：大 key + 热 key。按段拆、只保留 Top N。
- **score 用浮点**：精度问题，钱不要当 score 除非放大成整数。
- **ZRANGE 0 -1 取出全部**：阻塞。

## 4. 60 秒口述

> ZSet 小的时候用 listpack；大了是哈希表加跳表。哈希用 member 查分数，跳表按分数排序和范围。跳表比红黑树好写，范围扫描就是顺着层走。  
> 适合排行榜和按时间的延时队列。不要一个 key 塞全量用户，也不要 ZRANGE 一把拉完。
