# jeepay 学习笔记

> jeequan 开源聚合支付。与我的网关形态最接近的 Java 同类实现，其**命名约定路由渠道适配器**、**CAS 条件更新幂等**、**MQ 延迟消息驱动线性重试**是三个核心看点。

## 1. 项目概况

- **定位**：聚合支付系统（服务商/商户两级模式），支持微信/支付宝等主流渠道，Java 生态里 Star 最高的支付系统之一
- **技术栈**：Spring Boot / MyBatis-Plus / 可插拔 MQ（ActiveMQ / RabbitMQ / RocketMQ 多实现统一抽象）
- **模块划分**：
  - `jeepay-core`：实体与常量（`MchNotifyRecord` 等）
  - `jeepay-service`：DB 服务层（`PayOrderService` CAS 更新）
  - `jeepay-payment`：对外 API + 渠道回调入口 + 商户通知
  - `jeepay-components/mq`：MQ 统一抽象（IMQSender / IMQLoadStrategy）

## 2. 回调核心链路

```
渠道 → ChannelNoticeController.doNotify
     → getBean(ifCode + "ChannelNoticeService")   // 命名约定路由适配器
     → 渠道实现 parseParams + doNotice（验签、构造 ChannelRetMsg）
     → PayOrderService.updateIng2Success（CAS 条件更新）
     → 商户侧：PayMchNotifyService.notify4Payment → MQ → PayOrderMchNotifyMQReceiver
     → HTTP POST 商户地址，失败线性退避重发
```

### 2.1 接入与适配器路由

`jeepay-payment/.../ctrl/payorder/ChannelNoticeController.java`：

- 同时提供 `doSyncReturn`（用户支付完的**同步跳转**）与 `doNotify`（**异步回调**）两个入口——两类流量分开处理
- 路由方式：`SpringBeansUtil.getBean(ifCode + "ChannelNoticeService")`，按渠道编码取对应实现（微信 → `wxpayChannelNoticeService`）——**命名约定路由**，新增渠道只需实现 `IChannelNoticeService` 接口并按规范命名

### 2.2 验签与应答

- 验签、解析、构造统一结果 `ChannelRetMsg` 全部在渠道适配器内完成，平台侧不感知渠道差异
- **应答报文由适配器决定**：`notifyResult.getResponseEntity()` 直接返回——微信返回 XML `SUCCESS`、支付宝返回纯文本 `success`，应答格式是渠道知识，归属适配器
- `ChannelRetMsg.shouldNotify` 区分 success / fail / unknown 三种语义，**unknown** 时给渠道模糊应答促其重试——比我"验签后立刻 200"多了一层语义

### 2.3 幂等：CAS 条件更新

`jeepay-service/.../PayOrderService.java`：

```sql
UPDATE pay_order SET state = 'SUCCESS', success_time = ...
WHERE pay_order_id = ? AND state = 'ING'   -- 状态条件更新
```

- `updateIng2Success` / `updateIng2Fail` 带 `WHERE state = 'ING'`——**数据库层原子 CAS**，并发重复回调只有一个能更新成功
- 更新失败（影响行数=0）→ 说明订单已被处理或状态不允许 → 返回失败应答，让渠道重试再对一次账
- 不依赖 Redis、不依赖锁，单条 SQL 解决幂等——最朴素也最可靠

### 2.4 存储模型：商户通知记录

`jeepay-core/.../MchNotifyRecord.java`：

- 状态机：`ING(1) → SUCCESS(2) / FAIL(3)`
- 字段：`notifyCount`（已通知次数）、`notifyCountLimit`（默认 6）、`lastNotifyTime`、`resResult`（最近响应摘要）
- 写入双保险：业务层先查重（存在即跳过）+ **数据库唯一约束兜底** save（捕获 DuplicateKey）

### 2.5 分发与重试：MQ 延迟消息驱动

`jeepay-payment/.../service/PayMchNotifyService.java` + `mq/PayOrderMchNotifyMQReceiver.java`：

- 只对**终态**订单发起商户通知；通知报文 = 业务报文 + `reqTime` + **MD5 签名**
- 通知请求走 MQ 异步：`IMQSender.send(PayOrderMchNotifyMQ)`，消费者实际发 HTTP
- 消费者逻辑：
  - `state != ING` → 直接跳过（状态机防重）
  - `notifyCount >= notifyCountLimit(6)` → 停止
  - HTTP 超时 20s；响应忽略大小写等于 `SUCCESS` 才算通知成功（与微信官方判定口径一致）
  - 失败 → **线性梯度延迟重发 MQ**：0s / 30s / 60s / 90s / 120s / 150s（6 次用完 FAIL 终态）
- 首次通知成功 → `updateNotifySent` 在订单上标记"已通知"，订单详情可直接展示通知状态

### 2.6 兜底

- 定时任务 `ReissueTask`（补单）：定期查上游渠道订单接口，校正卡单——**回调与轮询双通道**，回调为主、补单兜底
- 通知无独立扫表兜底任务（依赖 MQ 可靠性 + 订单上的"已通知"标记可人工重发）

## 3. 亮点设计（附代码位置）

1. **命名约定路由适配器**：`getBean(ifCode + "ChannelNoticeService")`，零注册代码（`ChannelNoticeController.java`）
2. **CAS 条件更新幂等**：`WHERE state = 'ING'` 单 SQL 原子防重（`PayOrderService.java`）
3. **应答语义三分**：success / fail / unknown，unknown 促渠道重试（`ChannelRetMsg`）
4. **MQ 延迟消息做退避**：线性 0/30/60/90/120/150s，无需定时扫描（`PayOrderMchNotifyMQReceiver.java`）
5. **同步跳转与异步回调分流**：两类流量、两个入口，互不干扰

## 4. 与我的支付渠道回调网关对比

| 维度 | 我的方案 | jeepay | 差异分析 |
|---|---|---|---|
| 渠道适配 | 适配器模板类（显式注册） | 命名约定路由 | 显式注册 IDE 可查、更稳；命名约定零代码但靠字符串约定 |
| 入站幂等 | 唯一索引 + Redis | CAS `WHERE state=ING` | 三种流派之一：DB 原子条件更新，无额外组件；我的多一层 Redis 挡并发可降 DB 压力 |
| 应答策略 | 验签后立刻 200 | 适配器返回 success/fail/unknown 三语义 | unknown 语义值得引入：业务处理不确定时不应草率返回成功 |
| 出站分发 | Kafka 异步分发下游 | MQ 延迟消息 + 消费者直发 | 同构；它把重试间隔编码进延迟消息，我把重试留给补偿扫描 |
| 重试策略 | 补偿扫描 + 状态机重放 | 线性 6 次后退避终止 | 我的重放无次数上限语义，可加 notifyCountLimit |
| 兜底 | 补偿扫描 | 渠道查单补单任务 | 我的扫通知流水、它扫订单，方向一致 |
| 通知判定 | 下游 200 即成功 | 响应体必须含 SUCCESS | 更严格的成功判定，防止下游"假 200" |

## 5. 可落地到我的项目的改进

1. 入站状态机更新改用 **CAS 条件 UPDATE**（`WHERE status = 'PROCESSING'`），把幂等收敛到 DB 层，Redis 只做挡量
2. 通知流水加 `notify_count / notify_limit` 字段，重试带上限与**线性退避**，避免无限重放
3. 下游成功判定从"HTTP 200"升级为"200 + 响应体业务码校验"
4. 应答渠道的报文构造收进适配器（渠道知识归渠道），网关主链路只编排

## 6. 一句话总结

jeepay 证明支付回调的核心只需要三件朴素的事：**命名约定路由做扩展、CAS 状态更新做幂等、MQ 延迟消息做退避**——没有花哨组件，全部用数据库与消息队列的原生能力换可靠性。
