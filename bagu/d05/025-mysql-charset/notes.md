# 25. utf8 vs utf8mb4，排序规则 · 口试

## 1. 属于哪块？和相邻方案差在哪？

属于 **字符集和 collation**。MySQL 里名叫 `utf8` 的其实是 **utf8mb3**，最多 3 字节，**存不下 emoji 和部分生僻字**。`utf8mb4` 才是真正的 UTF-8（最多 4 字节）。

## 2. 核心机制

**字符集**

- 决定怎么编码、一字符几字节、索引长度怎么算。
- utf8mb4 下 `VARCHAR(255)` 在 InnoDB 索引里可能顶满 767/3072 限制（老版本 767，utf8mb4 只能 191 字符）。这是历史坑。

**collation 排序规则**

- 比较、ORDER BY、UNIQUE 是否大小写敏感、口音敏感。
- `utf8mb4_general_ci`：快、比较粗。
- `utf8mb4_unicode_ci` / `utf8mb4_0900_ai_ci`（8.0）：更符合 Unicode 规则。
- `_ci` = case insensitive；`_bin` = 按字节，大小写敏感。

**JOIN / 比较**

两边字符集或 collation 不同，可能触发转换 → **索引失效**。

**卡点**

- 库是 utf8mb4，连接层 `character_set_client` 仍是 latin1 → 乱码。要三层一致：库表列、连接、应用。

## 3. 生产坑 / 反例

- **以为 utf8 能存 emoji**——不能，应用一写 4 字节就 `Incorrect string value`。
- **改字符集锁大表**：等于改列类型，走 DDL 拷表。
- **UNIQUE 在 ci 规则下**：`Abc` 和 `abc` 冲突，业务以为能并存。

## 4. 60 秒口述

> MySQL 的 utf8 是 3 字节残缺版，emoji 要用 utf8mb4。collation 决定比较和唯一约束是否忽略大小写，两边不一致 JOIN 可能用不上索引。  
> 连接字符集也要 utf8mb4。老版本二级索引长度限制会让 VARCHAR(255) 建不了，需要前缀索引或升级。
