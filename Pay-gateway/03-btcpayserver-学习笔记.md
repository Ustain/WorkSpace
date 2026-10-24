# btcpayserver 学习笔记

> .NET 自托管比特币支付服务器。重点学习其 webhook 通知子系统：**错误码细分重试判定**、**按端点串行保序**、**重试投递链**三个设计对通用 webhook 网关同样成立。

## 1. 项目概况

- **定位**：BTCPay Server——自托管的比特币/链上支付服务器（收单、发票、 payouts），webhook 子系统用于把发票/付款事件通知给商户系统
- **技术栈**：C# / .NET / PostgreSQL（Dapper）、后台任务队列（BackgroundTaskQueue）
- **关注范围**：仅 webhook 子系统，位于 `BTCPayServer/Plugins/Webhooks/`（链上细节不在本笔记范围）

## 2. Webhook 核心链路

```
业务事件（InvoicePaymentSettled 等）
  → WebhookTriggerProvider 派生 WebhookEvent（按事件类型过滤订阅）
  → WebhookSender（MultiProcessingQueue 按 WebhookId 串行）
  → HMAC-SHA256 签名 → HTTP POST
  → 落 WebhookDeliveryData（每次尝试一条记录）
  → 失败按 ShouldRetry 判定 → 固定延迟序列自动重投（新建 delivery，OriginalDeliveryId 串联）
```

### 2.1 事件模型与订阅

`BTCPayServer.Client/Models/WebhookEventType.cs`：

- 事件类型是**显式枚举**（InvoiceCreated / InvoiceReceivedPayment / InvoicePaymentSettled / InvoiceInvalid / PayoutCompleted 等 30+），不是自由字符串——事件契约稳定、文档友好
- Webhook 注册模型（`WebhookData`）：URL + Secret + `AutomaticRedelivery`（可关自动重投）+ `Active` + **按事件类型订阅过滤**——一个 webhook 只收自己关心的事件

### 2.2 签名机制

`Plugins/Webhooks/WebhookSender.cs`：

```
BTCPay-Sig: sha256={Hex(HMAC-SHA256(secret, body))}
```

- 对称签名、body 直接作为被签内容，**无时间戳、无消息 ID、无防重放**——点对点信任模型下的最简方案（对比 svix 的 `id.timestamp.body` 三段签名可看出差距）

### 2.3 存储模型：每次尝试一条 delivery 记录

`WebhookDeliveryData`（OData 实体）：

- `DeliveryId`、`WebhookId`、`Url`、`Time`、`LastHttpStatus`、`ErrorMessage`、`RequestPayloadId`、`Blob`
- **重试 = 新建 delivery 记录**（`CreateRedeliveryRequest`），带 `IsRedelivery=true`、`OriginalDeliveryId` 指向首次投递——完整重试链路可追溯
- payload 可被 prune（`Pruned=true`）：老投递只留元数据删请求体省空间，代价是**pruned 的投递不能手动重发**——存储与重放能力的取舍

### 2.4 投递判定：ShouldRetry 错误码细分

`WebhookSender.ShouldRetry`：

- **重试**：5xx、429（限流）、408（超时）、网络异常（null response）
- **不重试**：其余 4xx（400/401/403/404——重试无意义，是调用方问题）、3xx、2xx
- 这是"**哪些失败值得重试**"的正确答案：区分"下游临时故障"与"调用方永久错误"

### 2.5 保序与并发：按 WebhookId 串行

- `MultiProcessingQueue`：同一 webhook（端点）的投递任务**串行执行**，注释明示 "makes sure the events sent to a webhook are sent in order"
- 不同 webhook 之间并行——**端点内有序、端点间并行**，是 webhook 保序的标准解法

### 2.6 兜底与可观测

- `AutomaticRedelivery` 固定延迟序列：10s / 1m / 10m / 10m / 10m / 10m / 10m / 10m（约 8 次、总窗口约 70 分钟）
- 手动 Redeliver 入口（dashboard 触发 `Redeliver`），pruned 记录除外
- 每条 delivery 记录含 HTTP 状态码与错误摘要，dashboard 可查投递历史

## 3. 亮点设计（附代码位置）

1. **ShouldRetry 错误码细分**：只对 5xx/429/408/网络错误重试（`WebhookSender.cs`）
2. **端点内串行保序**：MultiProcessingQueue 按 WebhookId 分队（`WebhookSender.cs`）
3. **重试投递链**：每次重试新建 delivery，`OriginalDeliveryId` 串联全链路
4. **事件类型显式枚举 + 订阅过滤**：契约稳定，端点按需订阅
5. **payload prune**：老记录删请求体省空间（明确取舍：放弃重放换存储）

## 4. 与我的支付渠道回调网关对比

| 维度 | 我的方案 | btcpayserver | 差异分析 |
|---|---|---|---|
| 方向 | 收渠道回调→分发给下游（inbound+outbound） | 业务事件→商户（纯 outbound） | 形态不同，但分发/重试层通用 |
| 重试判定 | 下游失败即重试（未细分） | 错误码细分：4xx 不重试 | **我的重试在白白消耗**：参数类错误重试多少次都失败 |
| 保序 | Kafka 异步分发 | 按 WebhookId 串行队列 | 可对齐：Kafka key=下游端点，分区天然保序 |
| 投递记录 | 通知流水状态机 | 每次尝试一条记录+链式关联 | 我的流水是否保留"每次尝试"粒度可对照补强 |
| 签名 | 验上游渠道签名 | 对下游 HMAC 签名（无防重放） | 它没有时间戳防重放——反向印证 svix 方案的必要性 |
| 重试间隔 | 补偿扫描周期 | 固定延迟序列 10s→10m | 都不指数退避；对下游不够友好，可改进 |
| 事件契约 | 标准事件对象 | 显式枚举+订阅过滤 | 我的下游订阅过滤能力可考虑加入 |

## 5. 可落地到我的项目的改进

1. 出站重试前加 **ShouldRetry 判定**：4xx（除 408/429）直接标记终态失败，不再重放
2. Kafka 分发以**下游端点 ID 为消息 key**：端点内有序（对应 MultiProcessingQueue 的语义）
3. 通知流水记录**每次投递尝试**（HTTP 状态、错误摘要），失败链路可追溯
4. 重试间隔至少改成固定序列递增（10s/1m/10m…），优于当前均匀扫描

## 6. 一句话总结

btcpayserver 的 webhook 子系统体量小但判断准：**用错误码细分决定要不要重试、用按端点串行换保序、用链式 delivery 记录换可追溯**——三个都是低成本高收益的工程判断。
