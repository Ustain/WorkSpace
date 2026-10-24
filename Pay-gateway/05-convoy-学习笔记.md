

# convoy 学习笔记

> Go 实现的开源 webhook 网关，与我的支付渠道回调网关**形态最接近**：收上游 webhook + 分发给下游端点。其**多 secret 版本化签名轮换**、**策略接口化重试**、**投递前熔断准入**是三个可直接吸收的设计。

## 1. 项目概况

- **定位**：开源 webhook 网关（outbound 为主，同时支持 inbound source 验证），Frain 公司商业化产品 Convoy 的开源核心
- **技术栈**：Go（gin）+ PostgreSQL + **asynq**（基于 Redis 的任务队列）
- **架构分层**：`api/`（HTTP 层，含 ingest.go 入站接收）→ `queue/`（队列）→ `worker/`（scheduler 调度 + consumer 消费 + task 投递）→ `net/dispatcher.go`（HTTP 发送器）

## 2. 核心链路

```
上游 webhook → ingest.go（Source 验证：HMAC/sha512 等多方案）
  → 存 Event → 按 Subscription 过滤出 Endpoints（fan-out，支持 URL 模板）
  → 生成 EventDelivery（每端点一条投递任务）
  → asynq 队列 → worker/task/process_event_delivery.go
  → 熔断准入（CircuitBreakerManager.CanExecute）→ 签名 → HTTP POST
  → 失败 → retry queue（策略计算延迟）→ NumTrials 上限 → 死信/手动重发
```

### 2.1 领域模型与路由

- `Project → Event → EventDelivery → Endpoint`：一次事件投递，**每个目标端点一条 EventDelivery**（fan-out 粒度的任务单元），状态独立
- Subscription 挂接端点与事件过滤规则；支持 **URL 模板**（`url.ContainsTemplate`，端点 URL 含 `{}` 占位符，投递时注入具体目标 `resolveEventDeliveryTargetURL`）——一个端点定义适配动态多目标
- **双向能力**：`pkg/verifier/verifier.go` 支持验证上游来源（HMAC / sha512 等多方案 Source Verification）——既有"收"也有"发"

### 2.2 签名：多 scheme 版本化 + secret 无缝轮换（核心亮点）

`pkg/signature/signature.go`：

- 高级签名 header：`t=<unix_ts>,v1=<hex(hmac_sha256(secret, "<ts>.<payload>">))>`——时间戳参与签名
- `Scheme` 结构：`Schemes []Scheme`（版本数组）+ `Secret []string`（**每版本允许多个 active secret 并存**，注释明示 "used to implement rolled secrets"）
- 轮换时：新旧 secret 同时对同一 payload 双签（`v1=...,v1=...`），下游任一 secret 校验通过即放行——**零停机换密钥**，轮换完成后再下线旧 secret

### 2.3 重试：策略接口化 + asynq 承载

`retrystrategies/`：

```go
type RetryStrategy interface { NextDuration(attempts uint64) time.Duration }
```

- `DefaultRetryStrategy`：固定间隔
- `ExponentialBackoffRetryStrategy`：`interval × 2^attempts`，封顶 `maxRetrySeconds`（默认 7200s），叠加 0~5s 随机 **jitter**（`d += jitter/2`）
- 队列：asynq（Redis）；retry queue 与主队列分离；`NumTrials` 控制总尝试次数，**asynq 任务预算同步抬高**（`defaultAsynqMaxRetries = 25`，注释解释了防止大限额被库默认值静默截断的细节）
- 失败任务写入独立 retry queue（`RetryEventProcessor / RetryEventQueue`），与首次投递物理隔离——重试不挤占新投递吞吐

### 2.4 投递前熔断准入

`worker/task/process_event_delivery.go`：

```go
if deps.CircuitBreakerManager != nil && circuitBreakingEnabledForOrg(...) {
    breakerErr := deps.CircuitBreakerManager.CanExecute(ctx, endpoint.UID)
    if breakerErr != nil { return &CircuitBreakerError{...} }
}
```

- 投递**先问熔断器**再发请求——连续失败的端点直接短路，连 HTTP 请求都省
- 配套 `worker/task/endpoint_disable.go` / `internal/endpoints/disable/`：端点自动禁用与所有权检查
- 熔断按 org 级开关（license/enablement 控制）——多租户下的功能灰度

### 2.5 状态机与兜底

- EventDelivery 状态：`Scheduled → Processing → Success / Failure / Retry`，Metadata 记录 `NumTrials / MaxRetrySeconds`
- 手动重发：`process_retry_event_delivery.go` 独立任务类型；dashboard 可触发
- 死信语义：重试耗尽后任务归档（asynq archived），保留可查

### 2.6 其他工程细节

- 限流：org/endpoint 级 Redis 或 Postgres 限流器（`internal/pkg/limiter/`）
- 批量事件：一次 API 调用多 payload，各自 fan-out
- 承载 meta event：convoy 自身的事件（如 endpoint created）也可走投递管道（`process_meta_event.go`）——与 svix 的 operational webhook 同思路

## 3. 亮点设计（附代码位置）

1. **多 secret 版本化签名轮换**：新旧密钥并存双签，零停机换钥（`pkg/signature/signature.go`）
2. **RetryStrategy 策略接口**：固定/指数退避可插拔、可配置（`retrystrategies/`）
3. **投递前熔断准入**：CanExecute 前置，失败端点零成本短路（`worker/task/process_event_delivery.go`）
4. **重试队列物理隔离**：retry queue 与主 queue 分离，互不拖累
5. **URL 模板端点**：动态目标注入（`process_event_delivery.go`）

## 4. 与我的支付渠道回调网关对比

| 维度 | 我的方案 | convoy | 差异分析 |
|---|---|---|---|
| 整体形态 | 渠道回调→验签→应答→Kafka 分发下游 | 上游 webhook→验证→fan-out→asynq 投递 | **高度同构**，可直接对标每个环节 |
| 入站验证 | 适配器验渠道签名（RSA/MD5/平台证书） | Source Verification 多方案可插拔 | 它把验证方案抽象成策略，我绑在适配器里 |
| 出站签名 | 无 | 时间戳签名 + 多 secret 轮换 | 我下游无法验证请求来源，缺安全层 |
| 重试 | 补偿扫描 + 状态机重放 | 策略接口 + 独立重试队列 + NumTrials | 我的重试与首次投递同通道，重试风暴会挤占新事件 |
| 熔断 | 下游加超时和熔断 | 投递前 CanExecute 准入 | 我的熔断在调用层统计触发，它是任务层显式准入，语义更清晰 |
| 任务粒度 | 通知流水 = 下游维度 | EventDelivery = 端点×事件 独立任务 | 同构，我的流水状态机对应它的 delivery 状态 |
| 兜底 | 补偿扫描 | asynq archived + 手动重发 | 都是"可查可重放"，我的扫描周期 vs 它的即时重试 |

## 5. 可落地到我的项目的改进

1. 下游通知加**时间戳 HMAC 签名**，预留多 secret 数组支持轮换（convoy 的 Scheme 设计可直接抄）
2. 重试走**独立 Kafka retry topic**（延迟分级），与首投分离防相互拖累
3. 熔断从"调用层被动统计"升级为"**任务层显式准入**"：投递任务执行前查熔断状态表，开启的断路器直接置 delivery 失败（原因=circuit_open）
4. 重试策略抽成接口（线性/指数/固定延迟可配置），配置挂在下游端点上
5. EventDelivery/通知流水加 `num_trials` 上限字段，重试耗尽进终态可人工重发

## 6. 一句话总结

convoy 就是一个"通用化的支付回调网关"：上游验证、fan-out 任务化、重试队列隔离、熔断前置准入、签名支持轮换——它验证了我的架构方向，并补上了我缺的安全签名、重试隔离与准入式熔断三块。
