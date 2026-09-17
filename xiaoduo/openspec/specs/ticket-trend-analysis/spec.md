# ticket-trend-analysis Specification

## Purpose
为客服主管提供可复现的工单趋势分析能力：从工单明细中汇总关键维度、识别可解释异常，并输出可独立阅读的报告。

## Requirements

### Requirement: Load Ticket Dataset
系统 SHALL 读取测评提供的工单 JSON，解析全部记录及其字段：`ticket_id`、`created_at`、`category`、`description`、`priority`、`resolution_time_hours`、`satisfaction`、`channel`、`is_resolved`。输入缺失或格式非法时，系统 MUST 以非零退出码失败并输出可读错误。

#### Scenario: Successful load of fifty tickets
- **WHEN** 输入文件包含 50 条合法工单
- **THEN** 系统加载全部 50 条记录并进入分析

#### Scenario: Invalid input file
- **WHEN** 输入文件不存在或不是合法 JSON 数组
- **THEN** 系统退出码非 0，并且不生成报告文件

### Requirement: Multi-Dimension Aggregation
系统 SHALL 至少从以下维度汇总工单，并说明每个维度对主管决策的价值：按日数量趋势、分类分布、分类随时间变化、优先级与未解决情况、处理时长、满意度、渠道、渠道与分类的关联。

#### Scenario: Daily and category summaries exist
- **WHEN** 分析完成
- **THEN** 报告包含每日工单量和每个分类的数量、占比、平均处理时长、平均满意度

#### Scenario: Channel and priority breakdown
- **WHEN** 分析完成
- **THEN** 报告包含渠道分布、渠道×分类交叉表，以及高优先级未解决工单清单

### Requirement: Explainable Anomaly Detection
系统 SHALL 识别值得主管关注的异常，并为每条异常给出判断依据、相关工单号和建议动作。检测 MUST 至少覆盖：分类突增、重复问题、高优未解决、处理积压、低满意度集中、资金风险描述。

#### Scenario: Category surge is flagged
- **WHEN** 某分类后半窗口至少 4 单、净增加至少 3 单，且日均数量相对前半窗口上升至少 50%
- **THEN** 系统输出该分类突增异常，并附上前后窗口数量对比

#### Scenario: Small sample is not treated as a surge
- **WHEN** 某分类仅从 1 单增加到 3 单
- **THEN** 系统不把它标成分类突增

#### Scenario: Repeat issue cluster is flagged
- **WHEN** 至少 3 条工单命中同一延迟/体验主题词，且不是单纯询问运费
- **THEN** 系统输出重复问题异常，并列出相关 `ticket_id`

#### Scenario: High-priority backlog is flagged
- **WHEN** 存在 `priority` 为「高」且 `is_resolved` 为 false 的工单
- **THEN** 系统将这些工单列入待关注积压清单

#### Scenario: Fund-risk wording is flagged
- **WHEN** 工单描述包含多扣、重复扣款、扣款金额错误等资金风险表述
- **THEN** 系统输出资金风险异常，并列出相关工单

### Requirement: Supervisor Report Output
系统 SHALL 生成一份 Markdown 报告和一份可离线打开的 HTML Dashboard。报告 MUST 包含：3 条互不重复主题的今日关注、趋势摘要、异常表、行动建议。HTML MUST 包含至少两类离线图表：每日趋势、分类分布。图表 MUST 不依赖外部 CDN。

#### Scenario: Markdown report is generated
- **WHEN** 分析成功结束
- **THEN** 输出 `output/report.md`，其中包含关注摘要、维度统计、异常表和行动建议

#### Scenario: HTML dashboard is generated
- **WHEN** 分析成功结束
- **THEN** 输出 `output/report.html`，浏览器打开后可见数字卡片和图表，无需额外服务

### Requirement: Reproducible CLI
系统 SHALL 提供一条本地命令，读取默认或指定工单文件并写出报告。重复运行 MUST 覆盖同名输出，而不是追加脏数据。

#### Scenario: Default analysis command
- **WHEN** 用户在项目根目录执行默认分析命令
- **THEN** 系统读取 `data/tickets.json` 并刷新 `output/` 下的报告

#### Scenario: Rerun overwrites previous output
- **WHEN** 用户连续运行两次分析命令
- **THEN** `output/report.md` 与 `output/report.html` 反映最近一次运行结果
