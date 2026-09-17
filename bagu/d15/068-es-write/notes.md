# 68. 写入链路与近实时可见 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **一条 index 请求怎么进 Lucene**。和题 62 同一条路，这题按步骤讲。

## 2. 核心机制

1. 请求打到协调节点，按 routing 转到主分片。
2. 主分片写 **translog** + 内存 buffer，成功后再转副本（看 `wait_for_active_shards`）。
3. 默认 refresh 1s：buffer 变成新 segment，**此时可搜**，文档是近实时。
4. 后台 merge 小 segment，减少文件数。
5. flush 把 segment 持久化，translog 滚动。

**可见性**

- `GET by id` 实时（走 translog/实时读）。
- **搜索** 走 refresh 后的 segment，所以搜和 GET 行为不同。

**卡点**

- 副本没成功：按参数可能仍返回成功或失败。
- merge 太猛抢 IO，搜和写都慢。

## 3. 生产坑 / 反例

- **写入后立刻搜**：偶发搜不到，不是丢，是 1s 窗口。
- **每条 refresh=true**：吞吐打崩。
- **副本=0 还当已经冗余**。

## 4. 60 秒口述

> 写先上主分片：translog 防丢，buffer 等 refresh 进 segment 才可搜，默认约 1 秒。GET 按 id 能实时，搜索不能。  
> 副本按策略同步。追求吞吐就别每条 refresh，追求搜到就 wait_for 或 GET。
