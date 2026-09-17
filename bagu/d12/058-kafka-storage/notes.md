# 58. 日志分段与清理策略 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **分区日志怎么在磁盘上切文件、过期怎么删**。Kafka 不把 Topic 当队列清空，默认是 **日志保留**。

## 2. 核心机制

**Segment**

- 一个分区由多个 segment 组成（默认约 1GB 或按时间切）。
- 只有 **最后一个** 是 active，顺序写；旧 segment 只读，可删可压。
- 同时有 offset 索引、时间索引，方便定位。

**清理**

- **delete**：按 `retention.ms` / `retention.bytes` 删过期 **整个旧 segment**（不是按条精确删）。
- **compact**：按 key 留最新值（changelog、状态类 Topic）。同一 key 旧值在压缩后消失。
- 也可两者结合。

**卡点**

- 保留 7 天不是「每条刚好 7 天」：没关段的 active segment 还在。
- compact 不是立即，依赖 cleaner 线程和脏比。
- 没有 key 的 compact 没意义。

## 3. 生产坑 / 反例

- **磁盘涨满**：retention 太大、生产太猛、或 compact 跟不上。
- **把 Kafka 当无限硬盘**：必须设保留或监控。
- **compact Topic 当审计日志**：历史会被收掉。

## 4. 60 秒口述

> 分区日志切成 segment，只对最新段追加。过期按段删除，或按 key compact 留最新。  
> 消费者慢超过 retention 会丢（offset 越界）。磁盘规划要看保留时间乘流量。审计日志用 delete 策略，状态快照用 compact。
