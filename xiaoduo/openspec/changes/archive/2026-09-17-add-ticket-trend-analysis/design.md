## Context

仓库是新建的测评交付目录，只有 OpenSpec 规划文件。输入是 50 条静态工单 JSON，时间跨度约 11 天。约束是：一次命令可复现、异常规则可解释、主管能直接看报告。不接线上工单系统，不调用付费 LLM。

## Goals / Non-Goals

**Goals:**
- 用标准库完成加载、聚合、异常检测和报告生成，降低复现成本。
- 把规则阈值写进代码常量和 README，保证评委能核对依据。
- HTML 单文件交付，方便截图和离线打开。

**Non-Goals:**
- 不搭建 Web 服务、数据库或定时任务。
- 不把工单描述交给外部大模型做分类。
- 不追求复杂时序模型；50 条样本以规则和对比窗口为主。

## Decisions

- Decision: Python 3 + 标准库，报告内嵌 Chart.js CDN。
  - Why: 无额外依赖，评委只要有 Python 就能跑；图表足够主管阅读。
  - Alternatives considered: pandas + matplotlib 需要安装；纯 Markdown 表格不利于截图展示趋势。

- Decision: 将观察期对半切分为前期/后期窗口做分类突增对比。
  - Why: 11 天样本不足以做稳健 z-score，对半对比足够暴露「支付问题变多」。
  - Alternatives considered: 滑动 3 日均值；样本太少，阈值难解释。

- Decision: 用关键词组做重复问题聚类，而不是编辑距离全量两两比较。
  - Why: 题面异常已经按业务主题埋好（重复扣款、退款慢、机器人无效），关键词更可解释。
  - Alternatives considered: 文本 embedding / LLM 归类；超出离线、零依赖约束。

- Decision: 同时输出 Markdown 和 HTML。
  - Why: Markdown 方便 GitHub 阅读和 README 引用；HTML 满足可视化截图。

## Risks / Trade-offs

- 关键词漏检 → 在 README 列出主题词表，并允许后续增补而不改统计口径。
- 对半窗口对更短数据集失效 → 窗口逻辑按唯一日期数切分，少于 4 天则只报绝对数量异常。
- Chart.js CDN 离线不可用 → HTML 仍保留表格和数字卡片，图表失败不影响正文。

## Migration Plan

首次交付即可使用。无存量系统需要迁移。回滚方式是删除 `output/` 并还原脚本。
