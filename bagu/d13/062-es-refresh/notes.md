# 62. refresh / flush / translog · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **ES 近实时**：写入成功 ≠ 立刻可搜。默认 **1 秒 refresh** 后才进可搜的 segment。

三个词别混：

- **refresh**：内存 buffer → 新的内存/磁盘 segment，**可被搜**。
- **flush**：segment 持久化并提交，释放 translog 压力。
- **translog**：类似 redo，防 refresh 前崩溃丢数据。

## 2. 核心机制

写路径直觉（题 68 更全）：

1. 写入内存 buffer + append translog。
2. 每 1s（默认）refresh：生成新 segment，可搜，仍可能只在文件系统缓存。
3. 满足条件 flush：fsync 索引文件，translog 可截断。

**卡点**

- `refresh=wait_for` 或每次 refresh 会打吞吐。
- 搜不到刚写的文档，先问是不是还没 refresh，不是丢了。

## 3. 生产坑 / 反例

- **「ES 写入立刻可查」**——错答。
- **高频 refresh=0 当实时** → 小 segment 爆炸，查询变慢。
- **关掉 translog fsync 追求速度**：掉电丢数据。

## 4. 60 秒口述

> ES 是近实时，默认 1 秒 refresh 后才可搜。translog 保证写入持久，flush 才把 segment 真正刷稳。  
> 要搜到刚写的可以 wait_for，但不能每条都这么干。refresh、flush、translog 三件事分别是可搜、落盘、防丢。
