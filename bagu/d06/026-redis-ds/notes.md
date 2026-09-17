# 26. 五类结构与底层编码 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **Redis 对象层 vs 编码层**。对外五种类型：string / list / hash / set / zset（还有 stream 等）。对内同一类型会随元素个数、长度 **切换编码**，省内存。

不要把「类型」和「编码」说成一回事：`TYPE key` 看类型，`OBJECT ENCODING key` 看编码。

## 2. 核心机制（记现在主流，Redis 7）

| 类型 | 小对象编码 | 大了变成 |
|------|------------|----------|
| string | int / embstr | raw |
| list | listpack | quicklist（listpack 链表） |
| hash | listpack | hashtable |
| set | intset / listpack | hashtable |
| zset | listpack | skiplist + dict |

阈值由 `hash-max-listpack-entries` 等配置控制。元素一超限，一次性转成哈希表/跳表，**不可逆**（一般不会缩回去）。

**为什么要两套**

- listpack/intset：紧凑连续内存，缓存友好，元素少时比指针哈希香。
- hashtable/skiplist：O(1) 或 O(log n)，元素多时必须用。

**卡点**

- 一个 hash 从 listpack 转 hashtable，内存会突然涨一截。
- 大 key 往往是「一个 hash/list 塞了几十万 field」。

## 3. 生产坑 / 反例

- **用一个巨大 hash 存整张用户表**：编码升级后内存和阻塞风险都高（题 43）。
- **背过时词 ziplist**：7.0 起 list/hash/zset 小对象是 listpack，口试提一句「老版本 ziplist，新版本 listpack」加分。
- **只说五类结构不说编码**：面试官下一问就是「为什么小 hash 省内存」。

## 4. 60 秒口述

> Redis 五种常用类型底下还有编码。元素少用 listpack、intset 这种紧凑布局；多了换成 hashtable 或跳表。类型不变，编码会变，OBJECT ENCODING 能看到。  
> 生产要把单个 key 的元素数量压在编码阈值附近，避免一个 key 膨胀后内存陡增、命令变慢。
