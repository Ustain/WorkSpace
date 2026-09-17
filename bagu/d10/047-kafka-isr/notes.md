# 47. ISR、HW、LEO · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **Kafka 副本怎么判断「跟上了」以及消费者能读到哪**。

- **LEO（Log End Offset）**：该副本日志最后一条的下一条 offset。
- **HW（High Watermark）**：所有 ISR 副本都已经有的最大 offset，**消费者只能读到 HW 之前**。
- **ISR（In-Sync Replicas）**：与 leader 差距在 replica.lag.time.max.ms 内的副本集合，含 leader。

## 2. 核心机制

1. 生产者写 leader。
2. follower fetch 复制。
3. leader 根据 ISR 推进 HW。
4. `acks=all` 要等 ISR 都写入（配置语义下）才成功。

**ISR 收缩**

- follower 太慢被踢出 ISR。
- ISR 只剩 leader 时，`acks=all` 实际上只等一个人，**丢数据风险变大**，所以要设 `min.insync.replicas>=2`。

**卡点**

- 未过 HW 的消息，leader 挂可能丢失，follower 不能当已提交给消费者。
- 口试数字：ISR 动态变化；HW 是可见性水位。

## 3. 生产坑 / 反例

- **acks=all 但 min.insync.replicas=1**：ISR 缩成 1 仍成功，挂了就丢。
- **把 LEO 当消费者可读位置**——应该是 HW。
- **副本在 ISR 外被选成 leader**（unclean.leader.election）：可能丢、可能重复，生产应关掉。

## 4. 60 秒口述

> 每个副本有 LEO 表示自己写到哪。HW 是 ISR 都追上的水位，消费者只能读 HW 前。ISR 是跟上 leader 的副本集合。  
> acks=all 要配 min.insync.replicas，防止 ISR 只剩自己还当成功。 unclean 选举会丢数据，生产关掉。
