# dax-pay 学习笔记

> dromara 开源聚合支付系统。与我的网关同属"收渠道回调 + 通知商户"形态，但其**锁包事务**、**终态守卫**、**任务表/记录表分离**是三个可直接借鉴的硬核设计。

## 1. 项目概况

- **定位**：聚合支付平台（微信/支付宝/云闪付收银、分账、转账、对账），面向商户与服务商
- **技术栈**：Java 21 / Spring Boot 3.x / MyBatis-Plus / Redis / Artemis MQ / xxl-job / ShedLock
- **模块划分**：
  - `daxpay-channel-*`（wechat / alipay / union…）：每渠道独立模块，负责验签解密与渠道交互
  - `daxpay-payment-*`：平台业务侧（订单状态机、回调处理、商户通知、对账）

## 2. 回调核心链路

```
渠道 → 渠道模块 CallbackController（URL 含 {mchNo}/{channelMchNo}）
     → 官方 SDK 验签解密 → 转标准 CallbackData
     → 平台侧 PayCallbackService（分布式锁 + CAS 状态机 + 落库）
     → 终态后 → MchNoticeTask（任务表）→ MQ → NoticeSendEngine → HTTP/MQ 通知商户
```

### 2.1 接收与验签：URL 携带商户号

微信 V3 回调 body 是 AES-GCM 加密的，**验签前无法解析出商户号**。dax-pay 把 `{mchNo}/{channelMchNo}` 编进回调 URL 模板，收到请求直接定位验签密钥：

- `daxpay-channel-wechat/.../controller/callback/WechatCallbackController.java`：`@PostMapping("/callback/{mchNo}/{channelMchNo}/...")` + `@IgnoreAuth` 放行
- 渠道侧用官方 `NotificationParser` 验签 → 解密 → 转平台标准 `CallbackData`，平台侧不感知渠道差异

### 2.2 幂等与状态机：锁包事务 + 终态守卫

`daxpay-payment-core/.../trade/runtime/service/callback/PayCallbackService.java`，注释里写明设计决策：

- **Redis 分布式锁**：`tryExecute("payment:trade:" + tradeId)`，获取不到锁 → 直接 IGNORE（另一线程正在处理同一笔回调）
- **锁包事务**（关键细节）：持锁（无事务）→ 通过 self 代理（避免同类调用 `@Transactional` 失效）再开事务 → 保证**锁释放与事务提交同步**，消除"锁已释放、事务未提交"的并发脏窗口
- **终态守卫**（状态机 CAS）：
  - 已 `SUCCESS` → 幂等忽略
  - `FAIL / CLOSE / CANCEL` 终态却收到成功回调 → **不翻转状态**，落"异常订单"转人工处置（防资损）
  - `INIT / PROCESSING` → 正常流转到终态

### 2.3 存储模型：任务表与记录表分离

| 表 | 职责 | 关键字段 |
|---|---|---|
| `pay_callback_record`（记录表） | 渠道入站回调**审计**，只存不重放 | 原始报文快照、验签结果、处理结果 |
| `mch_notice_task`（任务表） | 商户通知**调度锚点** | event、transport(http/mq)、format、`nextTime`、sendCount、delayCount、errorMsg(截断300) |
| `mch_notice_record`（记录表） | 每次通知尝试的审计 | httpStatus、requestDigest |

重试调度以**任务表**为锚点（`nextTime` 扫表驱动），每次尝试另落记录——调度与审计解耦。

### 2.4 应答策略

回调验签通过、状态机处理完成后再应答渠道；通知商户失败**不影响**对渠道的应答（入站与出站解耦）。

### 2.5 分发与重试

- `NoticeSendEngine`：事务提交后经 Artemis `scheduleImmediateAfterCommit` 排程首次通知（不与业务事务耦合）
- 失败 → `scheduleRetry(task, delaySeconds)` → `task.setNextTime(now.plus(delay))`
- **重试梯度（仿微信官方）**：HTTP 通知 15s/15s/30s/3m/10m/20m/30m×3/60m/3h×3/6h…，最多 **16 次**；MQ 通知最多 3 次、默认 60s——按 `transport` 区分策略
- 发送器抽象 `NoticeTransportSender`（http / mq 可插拔），报文 format 支持 system / easy_pay 兼容格式

### 2.6 兜底与可观测

- `NoticeRetryJob`（`trade/notice/job/`）：每 2 分钟扫"孤儿任务"（success=false 且 nextTime 到期或为空）重投 MQ，防止 MQ 丢消息导致通知悬挂
- ShedLock 防多节点重复执行
- 回调原始报文全量落库，事后可审计可追溯

## 3. 亮点设计（附代码位置）

1. **URL 编码商户号**：加密报文下从 URL 定位验签密钥（`WechatCallbackController.java`）
2. **锁包事务 + self 代理**：锁与事务同生命周期（`PayCallbackService.java` 注释明示）
3. **终态守卫转人工**：终态订单收到反向回调不翻转、转异常订单（防状态倒灌资损）
4. **任务表/记录表分离**：重试调度与审计解耦（`MchNoticeTask.java`）
5. **MQ + 扫表双保险**：MQ 延迟排程为主，2 分钟兜底扫表为辅（`NoticeRetryJob.java`）

## 4. 与我的支付渠道回调网关对比

| 维度 | 我的方案 | dax-pay | 差异分析 |
|---|---|---|---|
| 入站幂等 | MySQL 唯一索引兜底 + Redis 去重挡并发 | Redis 分布式锁 + CAS 状态机 | 我的靠"唯一键"原子性；它靠"锁+条件流转"，两者可组合：唯一索引挡重复、状态机挡语义重复 |
| 状态保护 | 通知流水状态机 | 终态守卫（终态不翻转、转人工） | 我的缺少"终态收到反向回调"分支，有状态倒灌风险 |
| 锁与事务 | 未显式处理边界 | 锁包事务，锁释放=事务提交 | 我用唯一索引天然原子，无此问题；但同思路适用于非唯一键场景 |
| 通知存储 | 单通知流水表 | 任务表+记录表分离 | 分离后重试调度更清晰、审计互不干扰 |
| 出站重试 | Kafka 异步 + 补偿扫描重放 | MQ 排程 + nextTime 扫表兜底 | 机制同构，我的"补偿扫描"即它的 NoticeRetryJob |
| 渠道适配 | 适配器模板类 | 渠道独立 Maven 模块 | 我单服务内适配器；它物理隔离模块，渠道间零耦合 |

## 5. 可落地到我的项目的改进

1. 状态机补"**终态守卫**"分支：已终态的订单收到成功回调 → 不更新流水，标记异常转人工
2. 通知流水拆出"**待通知任务**"视图（或表），补偿扫描直接扫任务表而非全量流水
3. 出站重试梯度对齐微信官方 16 次表（15s→6h），替代均匀间隔
4. 借鉴 `requestDigest`（报文摘要）而非全量存原始报文，控制流水表体积

## 6. 一句话总结

dax-pay 把回调当成"**分布式事务问题**"来做——锁、事务、状态机三位一体，任务表驱动通知，兜底扫描保最终一致；其对"锁-事务边界"和"终态守卫"的处理是我方案里缺失的两块。
