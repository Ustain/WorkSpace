# 57. ZK 时代 vs KRaft · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **Kafka 元数据怎么选主、怎么存 Topic/分区状态**。

- **ZK 时代**：Controller 借 ZooKeeper 做选举和元数据，运维两套系统。
- **KRaft**（2.8 起实验，3.3+ 生产可用）：Kafka 自己用 Raft 管元数据，**去掉 ZK**。

## 2. 核心机制

**ZK 模式痛点**

- 部署复杂：Kafka + ZK。
- ZK 成为元数据瓶颈，分区极多时 Controller 加载慢、故障转移慢。
- 元数据变更路径长。

**KRaft**

- 部分 Broker 当 **controller quorum**，Raft 复制元数据日志。
- 元数据就是一条内部日志，failover 更快、规模更好。
- 客户端协议不变，运维少一个组件。

**卡点**

- 面试别说「KRaft 让消息也用 Raft 复制」——**用户数据仍是 ISR 复制**，Raft 管的是元数据。
- 老集群迁移要按官方流程，不是改个配置立刻无痛。

## 3. 生产坑 / 反例

- **把 KRaft 当消息强一致算法** → 范围错。
- **小集群为去 ZK 强行升大版本** 而不测兼容。
- **Controller 节点数要奇数**，和哨兵一样。

## 4. 60 秒口述

> 老架构用 ZK 选 Controller、存元数据，运维重、分区多时 failover 慢。KRaft 用 Raft 在 Kafka 内部管元数据，去掉 ZK。  
> 业务消息复制还是 leader/ISR，没变成 Raft 日志。新集群直接 KRaft，老集群按迁移文档走。
