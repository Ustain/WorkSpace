# 88. Stream vs Pub/Sub · 口试

## 1. 属于哪块？和相邻方案差在哪？

都属于 Redis 消息能力，模型差很远。

- **Pub/Sub**：火和忘，**不持久、无积压、掉线错过**。
- **Stream**：日志型，有 id、可积压、有 **消费组**、可 ack，更像微型 Kafka。

## 2. 核心机制

**Pub/Sub**

- 订阅在线才收到。进程重启消息没了。
- 极轻，通知类、缓存失效广播（还要接受丢）。

**Stream**

- `XADD` 追加，`XREADGROUP` 消费组，`XACK`。
- pending 列表：派了没 ack 的可 PEL 捞回来。
- 可设 `MAXLEN` 裁剪，防无限涨。

**卡点**

- Stream 不是 Kafka：吞吐、堆积、多消费者运维都弱一档。
- Pub/Sub 在 Cluster 要注意，早期跨槽有坑（了解即可）。

## 3. 生产坑 / 反例

- **用 Pub/Sub 做订单** → 丢。
- **Stream 不 XACK** → PEL 膨胀。
- **当 Kafka 用无限保留** → Redis 内存满。

## 4. 60 秒口述

> Pub/Sub 实时广播不落盘，断线就丢。Stream 是 Redis 里的消息日志，有消费组和 ack，可重放一小段。  
> 可靠消费用 Stream 或真 MQ；通知用 Pub/Sub。Stream 要裁剪长度，别当 Kafka。
