# 83. ES 查询变慢 · 口试

## 1. 现象

搜索 RT 升、协调节点 CPU 高、拒绝请求、GC。写入也可能慢（merge 抢 IO）。

## 2. 止血

- 限流查询、砍深分页、关掉昂贵聚合。
- 扩副本分流读（若瓶颈在分片 CPU）。
- 拒绝超大 `size`、通配前缀查询。

## 3. 定位

- 慢日志、hot threads、heap、分片是否倾斜。
- 是否深分页 `from+size`、script、通配符、text 上聚合（fielddata）。
- 分片过多过小、refresh 太勤、磁盘 IO。

## 4. 根治

- 深分页改 `search_after`。
- mapping 纠正：聚合走 keyword。
- 按时间滚动索引、合并不合理分片。
- 热温架构、禁止前端开放任意 DSL。

## 5. 60 秒口述

> ES 变慢先限流和禁深分页、禁通配。定位慢日志和分片倾斜、错误 mapping。根治 search_after、keyword 聚合、滚动索引。别用 ES 当 SQL 扫全库。
