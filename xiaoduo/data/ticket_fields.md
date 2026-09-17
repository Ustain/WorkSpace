# 工单字段说明

测评附件 `tickets.json` 共 50 条客服工单，时间范围约 2024-06-01 至 2024-06-11。

| 字段 | 类型 | 说明 |
|---|---|---|
| `ticket_id` | string | 工单编号，如 `T001` |
| `created_at` | string | 创建时间，格式 `YYYY-MM-DD HH:MM` |
| `category` | string | 问题分类：退款退货 / 支付问题 / 物流查询 / 账号问题 / 商品咨询 / 投诉 |
| `description` | string | 用户问题描述原文 |
| `priority` | string | 优先级：高 / 中 / 低 |
| `resolution_time_hours` | number | 处理时长（小时） |
| `satisfaction` | number | 满意度评分，1-5，分数越低体验越差 |
| `channel` | string | 进线渠道：在线 / 电话 |
| `is_resolved` | boolean | 是否已解决 |
