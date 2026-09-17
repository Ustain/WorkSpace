# 70. etcd vs ZK · 口试

## 1. 属于哪块？和相邻方案差在哪？

都属于 **协调服务 / 小数据强一致 KV**。etcd 用 **Raft**，API 是 HTTP/gRPC KV+lease+watch；ZK 是 ZAB + znode 树。

Kubernetes 选 etcd。新项目协调层也更常见 etcd。

## 2. 核心机制（对比）

| | etcd | ZK |
|--|------|-----|
| 协议 | Raft | ZAB |
| 模型 | 扁平 KV，revision | 树形 znode |
| 会话 | lease | session + 临时节点 |
| 客户端 | HTTP/gRPC，简单 | 会话更重 |
| 生态 | K8s | 老中间件多 |

都是 **多数派、CP、小数据**。锁：etcd 用 lease 防死锁；ZK 用临时顺序节点。

**Raft 直觉**

- 选 Leader，日志复制，过半提交。
- 比 ZAB 更好讲：term、log index、commit index。

## 3. 生产坑 / 反例

- **etcd 存大 value / 无限历史**：要压缩 compaction，否则爆盘。
- **租约太短 + 业务 STW**：锁丢了临界区还在跑（和 Redis 锁同类问题，但一致性更强）。
- **和 ZK 混用两套协调** 增加脑裂面。

## 4. 60 秒口述

> etcd 和 ZK 都是小数据强一致协调。etcd 是 Raft+KV+lease，K8s 在用；ZK 是 ZAB+树+临时节点，老生态多。  
> 新项目偏 etcd。都不要存业务大数据，都要奇数节点过半。锁仍然要注意租约过期后别继续改资源。
