# svix-webhooks 学习笔记

> Rust 实现的专业 webhook 发送平台（svix 商业版开源核心）。它是 webhook 基础设施的"满配形态"：**时间戳防重放签名、带 jitter 的配置化重试调度、端点自动禁用、operational webhook 可观测闭环**，四个方面都值得我的网关对标。

## 1. 项目概况

- **定位**：给 SaaS 平台用的"发 webhook 给终端用户"的基础设施服务（Stripe-like webhook 体验的开源实现）
- **技术栈**：Rust（axum + tokio）+ PostgreSQL + Redis（队列/缓存），队列支持 redis / postgres / memory 三实现
- **代码结构**：`server/svix-server/src/` 下 `core/`（安全、模型）、`queue/`（队列抽象）、`v1/`（API）、`worker.rs`（投递调度核心，单文件含全部重试逻辑）

## 2. 核心链路

```
API 收到创建消息请求
  → 按 event_type / channels 过滤出目标端点（filtered_endpoints）
  → future::join_all 并发 fan-out
  → worker.rs：sign_msg 签名 → HTTP POST
  → 成功 ack / 失败按 retry_schedule 延迟任务重回队列
  → 连续失败超过宽限期 → endpoint.disabled + 发 operational webhook 告知开发者
```

### 2.1 领域模型

`Application → Endpoint → Message → MessageAttempt` 四级：

- Application：接入方（一个 SaaS 客户）
- Endpoint：接收 URL + secret + 版本号 + 过滤规则（event_types / channels）
- Message：event_type + payload + `msg_id`（唯一键，客户端可自带做幂等）
- MessageAttempt：每次投递尝试的完整记录（状态码、响应体**截断 20000 字节**存储）

### 2.2 签名方案（对标学习重点）

`svix-server/src/core/security.rs` 的 `sign_msg`：

```
signed_content = "{msg_id}.{timestamp}.{body}"
signature = v1,base64(HMAC-SHA256(secret, signed_content))
headers: svix-id / svix-timestamp / svix-signature
```

- 对比 btcpayserver 的裸 HMAC：**多了 msg_id 与 timestamp 参与签名**，接收端校验时间戳容差即**防重放攻击**
- 同时支持 **Ed25519 非对称签名**（`v1a,` 前缀）——接收端只需公钥，无需分发共享 secret
- header 前缀可 whitelabel（`webhook-id` 等），SDK 全语言覆盖（JS/Python/Go/Java/Ruby/PHP/C#/Rust/Kotlin…）

### 2.3 重试调度（worker.rs）

- `retry_schedule` 配置化数组，默认约 `[立即, 5s, 5m, 30m, 2h, 5h, 10h, 10h]`——**先密后疏**，8 档覆盖 24h 窗口
- **±20% jitter**（JITTER_DELTA）：同批失败的端点重试时间打散，防惊群
- 429 / 超时额外加 `OVERLOAD_PENALTY_SECS = 60s` 惩罚——下游已经限流，还紧贴着重试就是雪上加霜
- **尊重下游 Retry-After header**：clamp 到 `[base, base×2]` 区间——下游说几分钟后见就几分钟后再来
- 重试不靠 sleep、不靠扫表：**延迟任务直接进队列**（`queue_tx.send(task, Some(retry_delay))`），队列原生支持延迟投递

### 2.4 可靠性语义

- **at-least-once**：任务 ack/nack，nack 后重回队列重新投递；`msg_id` 唯一键 + 接收端按 id 去重
- 消费失败处理失败 → 重新入队，worker 崩溃任务不丢
- worker 并发上限可配（`worker_max_tasks`、NUM_WORKERS 原子计数）

### 2.5 端点治理：自动禁用与恢复

`worker.rs` + `core/cache`：

- 每个端点有 `endpoint_failure_disable_after`（宽限期）配置：失败时间戳缓存 Redis（过期时间 = disable_in×2）
- 宽限期内持续全失败 → `endpoint.disabled = true`，停止投递省资源
- 禁用时**发 operational webhook（`EndpointDisabled`）主动告知开发者**——坏消息要送达到人而不是埋在日志里

### 2.6 可观测闭环：operational webhooks

svix 把**基础设施自身的事件**也用 webhook 通知出去：

- `MessageAttemptFailing`（第 4 次连续失败预警）
- `MessageAttemptExhausted`（重试耗尽）
- `MessageAttemptRecovered`（恢复成功）
- `EndpointDisabled`（端点被禁用）

即"投递系统自己的状态变化"成为一类一等公民事件——接收方可订阅 svix 的故障事件做自己的监控。

### 2.7 安全

- SSRF 防护：`whitelist_subnets`（禁内网网段）、`whitelist_names`（禁解析到内网的域名）、关闭 TLS 校验需显式 `dangerous_` 前缀开关
- 每次投递全量链路追踪（attempt 记录：请求头/响应头摘要/耗时/状态码）

## 3. 亮点设计（附代码位置）

1. **三段式签名防重放**：`id.timestamp.body` 参与签名 + 时间戳容差校验（`core/security.rs`）
2. **配置化重试表 + jitter + 过载惩罚 + Retry-After**：调度策略精细（`worker.rs`）
3. **延迟重试进队列**：不用 sleep 不用扫表，队列原生延迟（`worker.rs`）
4. **宽限期自动禁用 + 恢复**：保护系统资源、自动止损（`worker.rs` + cache）
5. **operational webhook**：系统自身状态事件化，可观测闭环（`worker.rs`）

## 4. 与我的支付渠道回调网关对比

| 维度 | 我的方案 | svix | 差异分析 |
|---|---|---|---|
| 对下游签名 | 未包含（网关只验上游） | HMAC+时间戳+id 防重放 / Ed25519 | 我对下游无签名约定，下游无法验证请求真伪 |
| 重试策略 | 补偿扫描 + 状态机重放 | 配置化调度表 + jitter + 惩罚 + Retry-After | 我的重试无 jitter，同批失败会同时打下游；无过载感知 |
| 重试载体 | Kafka + 补偿扫描 | 队列原生延迟任务 | 等价，但延迟消息表达力更强、无扫描空转 |
| 端点治理 | 下游超时 + 熔断 | 宽限期自动禁用 + 恢复 + 主动通知 | 我的熔断后缺少"禁用通知开发者"和自动恢复探测 |
| 幂等 | MySQL 唯一索引 + Redis | msg_id 唯一键 + at-least-once | 同构；它把幂等键暴露给接收方约定 |
| 尝试记录 | 通知流水 | attempt 记录（响应截断 20000B） | 响应体截断存摘要值得照抄 |

## 5. 可落地到我的项目的改进

1. 对下游通知加**签名约定**：`X-Sign: sha256=HMAC(secret, "{msgId}.{timestamp}.{body}")`，下游可验真防重放
2. 重试间隔加 **±20% jitter**；遇下游 429/超时追加惩罚延迟；支持 Retry-After
3. 下游连续失败超阈值 → **自动禁用** + 发内部事件通知运维，恢复后自动重启用
4. 尝试记录只存响应**摘要与状态码**（截断），控制流水表膨胀
5. 定义"网关 operational 事件"（投递失败预警/耗尽/恢复），接入现有告警通道

## 6. 一句话总结

svix 把 webhook 从"发个 HTTP 请求"做成了**有安全（防重放签名）、有礼貌（jitter/Retry-After/过载惩罚）、有自愈（自动禁用恢复）、有交代（operational 事件）**的基础设施——它就是我的网关出站侧的完整参考答案。
