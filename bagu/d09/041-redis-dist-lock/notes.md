# 41. 分布式锁正确写法与 Redlock 争议 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **互斥访问共享资源**。单实例 Redis 锁能做 **尽量互斥**，做不到和 DB 一样的线性一致。ZK/etcd 锁更偏 CP。

## 2. 核心机制（单 Redis 正确姿势）

**加锁**

```
SET lock_key unique_token NX PX 30000
```

- **NX**：没有才加。
- **PX**：必须带过期，持有者崩溃也能开。
- **unique_token**：谁加的谁才能删。

**解锁（Lua 校验再删）**

```
if redis.call("get", KEYS[1]) == ARGV[1] then
  return redis.call("del", KEYS[1])
else return 0 end
```

先 GET 再 DEL 不是原子的：可能删掉别人的新锁。

**续期**

- 业务可能超过 TTL：看门狗续期（Redisson）。
- 续期失败要当没锁，不能继续改共享资源。

**Redlock**

- 向 N 个独立 Redis 依次加锁，多数成功且耗时 < TTL 才算成功。
- Martin Kleppmann 批评：时钟跳变、GC 停顿、锁过期后临界区还在跑，仍不安全。
- 面试立场：**讲清争议**；强一致用 ZK/etcd 或 DB 约束；Redis 锁适合防重复提交、短临界区。

## 3. 生产坑 / 反例

- **SET NX 不带过期**：死锁。
- **DEL 不校验 token**：误删。
- **TTL 太短业务没跑完**：两个线程进临界区。
- **主从复制锁**：主加锁未复制就切换，新主没有锁 → 双持有。这是 Redis 锁最大结构性风险。

## 4. 60 秒口述

> 正确写法是 SET key 唯一值 NX PX，解锁用 Lua 比对再删。必须带过期和持有者校验。  
> 主从切换可能丢锁，所以 Redis 锁不是金融级互斥。Redlock 有争议，时钟和 GC 仍可能双持有。要强一致用 etcd/ZK 或数据库约束。
