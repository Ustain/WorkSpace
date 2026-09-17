# 49. acks / retries / 幂等 Producer · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **生产者侧丢消息 vs 重复**。acks 决定「写到哪才算成功」；retries 决定失败重试；幂等 Producer 把重试导致的重复压掉（**仅限单分区、同一 Producer 会话**）。

## 2. 核心机制

**acks**

| 值 | 含义 | 风险 |
|----|------|------|
| 0 | 发出去就当成功 | 最易丢 |
| 1 | leader 写完 | leader 挂、ISR 没跟上会丢 |
| all/-1 | ISR 都确认（配合 min.insync.replicas） | 最安全，延迟高 |

**retries**

- 网络抖动重试会 **重复**，除非开幂等。
- 还要设 `max.in.flight.requests.per.connection`：幂等时建议 1～5（新版本允许 >1 仍保序）。

**幂等 Producer**

- `enable.idempotence=true`：PID + 序号，Broker 去重。
- 解决的是 **生产者重试重复**，不是消费者重复处理，也不是业务表幂等。
- 跨会话（进程重启新 PID）不去重。

**卡点**

- acks=all 必须 `min.insync.replicas>=2`，否则 ISR 缩成 1 仍「成功」。
- 幂等 ≠ 事务 ≠ 业务精确一次（题 54、55）。

## 3. 生产坑 / 反例

- **acks=all 就说绝不丢**——磁盘、ISR、unclean 选举都会破功。
- **开 retries 不开幂等**：乱序或重复。
- **所有 topic 都 all**：监控类可 acks=1 换吞吐。

## 4. 60 秒口述

> acks=0 只发、1 等 leader、all 等 ISR，生产关键链路用 all 加 min.insync.replicas。retries 防抖动但会重复，所以要幂等 Producer：Broker 用 PID 和序号去重。  
> 幂等只管生产者重试，不管消费端和业务表。别把 acks=all 说成绝对不丢。
