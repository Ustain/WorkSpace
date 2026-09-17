#!/usr/bin/env python3
"""客服工单趋势分析：加载、聚合、异常检测、主管报告。"""

from __future__ import annotations

import argparse
import html
import json
import statistics
import sys
from collections import Counter, defaultdict
from datetime import datetime
from pathlib import Path
from typing import Any

REQUIRED_FIELDS = (
    "ticket_id",
    "created_at",
    "category",
    "description",
    "priority",
    "resolution_time_hours",
    "satisfaction",
    "channel",
    "is_resolved",
)

SURGE_RATIO = 1.5
SURGE_MIN_LATE = 4
SURGE_MIN_INCREASE = 3
REPEAT_MIN_HITS = 3
LOW_CSAT_MAX = 2.0
LOW_CSAT_MIN_COUNT = 3
LOW_CSAT_MAX_ALERTS = 2
LONG_HANDLE_HOURS = 48
CHANNEL_CSAT_GAP = 0.4
FOCUS_LIMIT = 3
PALETTE = ("#38bdf8", "#f97316", "#a78bfa", "#34d399", "#fb7185", "#facc15")

ISSUE_THEMES = (
    {
        "id": "slow_refund",
        "group": "refund",
        "name": "退款或垫付费用迟迟不到账",
        "keywords": ("还没退", "没退", "退款还在", "还在处理", "审核中", "到现在没退", "垫付了", "什么时候给"),
    },
    {
        "id": "bot_useless",
        "group": "experience",
        "name": "机器人/在线客服无法解决",
        "keywords": ("机器人", "重新描述", "等了40分钟", "人手不够"),
    },
)

FUND_KEYWORDS = ("重复扣", "扣了两次", "两个都扣", "多扣", "扣款金额不对", "扣了我")

DIMENSION_VALUE = {
    "时间趋势": "判断最近是不是变忙，便于排班。",
    "类型分布": "看哪类问题占人力最多，决定专项小组。",
    "类型随时间变化": "识别正在恶化的品类，而不是只看总量。",
    "优先级与未解决": "把高优积压置顶，避免资金和账号风险过夜。",
    "处理时长": "暴露退款等长尾积压，衡量 SLA。",
    "满意度": "定位体验崩塌的分类，指导质检。",
    "渠道": "电话进线通常更紧急，对照在线分流效果。",
    "关联关系": "看渠道、分类、未解决是否叠在同一批单上，避免各看各的表。",
}


class TicketLoadError(ValueError):
    """工单文件无法使用。"""


def parse_created_at(value: str) -> datetime:
    return datetime.strptime(value, "%Y-%m-%d %H:%M")


def load_tickets(path: str | Path) -> list[dict[str, Any]]:
    file_path = Path(path)
    if not file_path.exists():
        raise TicketLoadError(f"工单文件不存在: {file_path}")
    try:
        raw = json.loads(file_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise TicketLoadError(f"工单文件不是合法 JSON: {exc}") from exc
    if not isinstance(raw, list):
        raise TicketLoadError("工单文件必须是 JSON 数组")
    tickets: list[dict[str, Any]] = []
    for index, item in enumerate(raw, start=1):
        if not isinstance(item, dict):
            raise TicketLoadError(f"第 {index} 条不是对象")
        missing = [field for field in REQUIRED_FIELDS if field not in item]
        if missing:
            raise TicketLoadError(f"第 {index} 条缺少字段: {', '.join(missing)}")
        ticket = dict(item)
        try:
            ticket["_dt"] = parse_created_at(str(ticket["created_at"]))
        except ValueError as exc:
            raise TicketLoadError(f"第 {index} 条时间格式错误: {exc}") from exc
        ticket["_day"] = ticket["_dt"].strftime("%Y-%m-%d")
        tickets.append(ticket)
    return tickets


def _avg(values: list[float]) -> float:
    return round(statistics.mean(values), 2) if values else 0.0


def aggregate(tickets: list[dict[str, Any]]) -> dict[str, Any]:
    days = sorted({t["_day"] for t in tickets})
    daily = []
    for day in days:
        items = [t for t in tickets if t["_day"] == day]
        daily.append(
            {
                "day": day,
                "count": len(items),
                "unresolved": sum(1 for t in items if not t["is_resolved"]),
                "by_category": dict(Counter(t["category"] for t in items)),
            }
        )

    categories = [_group_stats(name, items, len(tickets)) for name, items in _group(tickets, "category")]
    categories.sort(key=lambda row: row["count"], reverse=True)
    channels = [_group_stats(name, items, len(tickets)) for name, items in _group(tickets, "channel")]
    priorities = [_group_stats(name, items, len(tickets)) for name, items in _group(tickets, "priority")]

    unresolved = [t for t in tickets if not t["is_resolved"]]
    high_unresolved = [t for t in unresolved if t["priority"] == "高"]
    long_handle = [t for t in tickets if float(t["resolution_time_hours"]) >= LONG_HANDLE_HOURS]
    low_score = [t for t in tickets if int(t["satisfaction"]) <= 2]

    channel_category: list[dict[str, Any]] = []
    for channel, channel_items in _group(tickets, "channel"):
        for category, cat_items in _group(channel_items, "category"):
            channel_category.append(
                {
                    "channel": channel,
                    "category": category,
                    "count": len(cat_items),
                    "unresolved": sum(1 for t in cat_items if not t["is_resolved"]),
                    "avg_satisfaction": _avg([float(t["satisfaction"]) for t in cat_items]),
                }
            )

    return {
        "total": len(tickets),
        "days": days,
        "daily": daily,
        "categories": categories,
        "channels": channels,
        "priorities": priorities,
        "channel_category": channel_category,
        "unresolved_count": len(unresolved),
        "unresolved_rate": round(len(unresolved) / len(tickets), 3) if tickets else 0,
        "high_unresolved": [_brief(t) for t in high_unresolved],
        "long_handle": [_brief(t) for t in long_handle],
        "low_score_count": len(low_score),
        "avg_handle_hours": _avg([float(t["resolution_time_hours"]) for t in tickets]),
        "avg_satisfaction": _avg([float(t["satisfaction"]) for t in tickets]),
        "dimension_value": DIMENSION_VALUE,
    }


def _group(tickets: list[dict[str, Any]], key: str) -> list[tuple[str, list[dict[str, Any]]]]:
    buckets: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for ticket in tickets:
        buckets[str(ticket[key])].append(ticket)
    return sorted(buckets.items(), key=lambda item: len(item[1]), reverse=True)


def _group_stats(name: str, items: list[dict[str, Any]], total: int) -> dict[str, Any]:
    return {
        "name": name,
        "count": len(items),
        "share": round(len(items) / total, 3) if total else 0,
        "avg_handle_hours": _avg([float(t["resolution_time_hours"]) for t in items]),
        "avg_satisfaction": _avg([float(t["satisfaction"]) for t in items]),
        "unresolved": sum(1 for t in items if not t["is_resolved"]),
        "low_score": sum(1 for t in items if int(t["satisfaction"]) <= 2),
    }


def _brief(ticket: dict[str, Any]) -> dict[str, Any]:
    return {
        "ticket_id": ticket["ticket_id"],
        "created_at": ticket["created_at"],
        "category": ticket["category"],
        "priority": ticket["priority"],
        "description": ticket["description"],
        "resolution_time_hours": ticket["resolution_time_hours"],
        "satisfaction": ticket["satisfaction"],
        "channel": ticket["channel"],
        "is_resolved": ticket["is_resolved"],
    }


def _contains_any(text: str, keywords: tuple[str, ...]) -> bool:
    return any(keyword in text for keyword in keywords)


def _anomaly(
    *,
    anomaly_id: str,
    group: str,
    atype: str,
    severity: str,
    title: str,
    evidence: str,
    ticket_ids: list[str],
    action: str,
) -> dict[str, Any]:
    return {
        "id": anomaly_id,
        "group": group,
        "type": atype,
        "severity": severity,
        "title": title,
        "evidence": evidence,
        "ticket_ids": ticket_ids,
        "action": action,
    }


def detect_anomalies(tickets: list[dict[str, Any]], stats: dict[str, Any]) -> list[dict[str, Any]]:
    anomalies: list[dict[str, Any]] = []
    anomalies.extend(_detect_surges(tickets, stats))
    anomalies.extend(_detect_fund_risk(tickets))
    anomalies.extend(_detect_themes(tickets))
    anomalies.extend(_detect_high_backlog(tickets))
    anomalies.extend(_detect_long_handle(stats))
    anomalies.extend(_detect_low_csat(stats))
    anomalies.extend(_detect_channel_gap(stats))
    order = {"P0": 0, "P1": 1, "P2": 2}
    anomalies.sort(key=lambda item: (order.get(item["severity"], 9), item["title"]))
    return _dedupe_anomalies(anomalies)


def _split_windows(days: list[str]) -> tuple[set[str], set[str]]:
    if len(days) < 4:
        return set(), set(days)
    mid = len(days) // 2
    return set(days[:mid]), set(days[mid:])


def _detect_surges(tickets: list[dict[str, Any]], stats: dict[str, Any]) -> list[dict[str, Any]]:
    early_days, late_days = _split_windows(stats["days"])
    if not early_days or not late_days:
        return []
    anomalies = []
    for row in stats["categories"]:
        name = row["name"]
        early_items = [t for t in tickets if t["category"] == name and t["_day"] in early_days]
        late_items = [t for t in tickets if t["category"] == name and t["_day"] in late_days]
        early_count = len(early_items)
        late_count = len(late_items)
        if late_count < SURGE_MIN_LATE or (late_count - early_count) < SURGE_MIN_INCREASE:
            continue
        early_avg = early_count / len(early_days)
        late_avg = late_count / len(late_days)
        ratio = float("inf") if early_avg == 0 else late_avg / early_avg
        if ratio < SURGE_RATIO:
            continue
        group = "payment" if name == "支付问题" else "refund" if name == "退款退货" else "watch"
        severity = "P0" if name in {"支付问题", "退款退货"} else "P1"
        growth = "新出现" if early_avg == 0 else f"{(ratio - 1) * 100:.0f}%"
        anomalies.append(
            _anomaly(
                anomaly_id=f"surge-{name}",
                group=group,
                atype="分类突增",
                severity=severity,
                title=f"「{name}」后期明显变多",
                evidence=(
                    f"前半窗口 {sorted(early_days)[0]}~{sorted(early_days)[-1]} 共 {early_count} 单"
                    f"（日均 {early_avg:.2f}），后半窗口 {sorted(late_days)[0]}~{sorted(late_days)[-1]} "
                    f"共 {late_count} 单（日均 {late_avg:.2f}），日均增幅 {growth}。"
                    f"规则：后期≥{SURGE_MIN_LATE} 单、净增≥{SURGE_MIN_INCREASE}、日均增幅≥50%。"
                ),
                ticket_ids=[t["ticket_id"] for t in late_items],
                action=f"安排{name}专项排查，优先核对近期支付/退款规则和系统变更。",
            )
        )
    return anomalies


def _detect_themes(tickets: list[dict[str, Any]]) -> list[dict[str, Any]]:
    anomalies = []
    for theme in ISSUE_THEMES:
        hits = [t for t in tickets if _contains_any(str(t["description"]), theme["keywords"])]
        if len(hits) < REPEAT_MIN_HITS:
            continue
        anomalies.append(
            _anomaly(
                anomaly_id=f"theme-{theme['id']}",
                group=theme["group"],
                atype="重复问题",
                severity="P0" if theme["id"] == "slow_refund" else "P1",
                title=theme["name"],
                evidence=f"命中延迟/体验主题词后共 {len(hits)} 单，阈值 {REPEAT_MIN_HITS}。不含单纯询价运费。",
                ticket_ids=[t["ticket_id"] for t in hits],
                action="把同一问题收成标准处理路径，避免用户反复进线。",
            )
        )
    return anomalies


def _detect_high_backlog(tickets: list[dict[str, Any]]) -> list[dict[str, Any]]:
    hits = [t for t in tickets if t["priority"] == "高" and not t["is_resolved"]]
    if not hits:
        return []
    refundish = sum(1 for t in hits if t["category"] == "退款退货")
    group = "refund" if refundish >= (len(hits) / 2) else "backlog"
    return [
        _anomaly(
            anomaly_id="high-unresolved",
            group=group,
            atype="高优未解决",
            severity="P0",
            title=f"{len(hits)} 张高优先级工单仍未关闭",
            evidence=f"规则：priority=高 且 is_resolved=false。其中退款退货 {refundish} 张。",
            ticket_ids=[t["ticket_id"] for t in hits],
            action="今日站会按资金/退款/物流顺序逐单闭环，未关单不过夜。",
        )
    ]


def _detect_long_handle(stats: dict[str, Any]) -> list[dict[str, Any]]:
    hits = stats["long_handle"]
    if not hits:
        return []
    refundish = sum(1 for row in hits if row["category"] == "退款退货")
    return [
        _anomaly(
            anomaly_id="long-handle",
            group="refund" if refundish else "backlog",
            atype="处理积压",
            severity="P1",
            title=f"{len(hits)} 张工单处理时长 ≥ {LONG_HANDLE_HOURS} 小时",
            evidence=f"规则：resolution_time_hours ≥ {LONG_HANDLE_HOURS}。退款类 {refundish} 张。",
            ticket_ids=[row["ticket_id"] for row in hits],
            action="核对退款审核是否卡在财务或仓储，补 48 小时升级机制。",
        )
    ]


def _detect_low_csat(stats: dict[str, Any]) -> list[dict[str, Any]]:
    candidates = [
        row
        for row in stats["categories"]
        if row["count"] >= LOW_CSAT_MIN_COUNT and row["avg_satisfaction"] <= LOW_CSAT_MAX
    ]
    candidates.sort(key=lambda row: (row["avg_satisfaction"], -row["count"]))
    anomalies = []
    for row in candidates[:LOW_CSAT_MAX_ALERTS]:
        anomalies.append(
            _anomaly(
                anomaly_id=f"csat-{row['name']}",
                group="experience",
                atype="低满意度",
                severity="P1",
                title=f"「{row['name']}」平均满意度仅 {row['avg_satisfaction']}",
                evidence=(
                    f"{row['name']} 共 {row['count']} 单，均分 {row['avg_satisfaction']}，"
                    f"低分(≤2) {row['low_score']} 单。只取均分≤{LOW_CSAT_MAX} 的最差 {LOW_CSAT_MAX_ALERTS} 类，"
                    f"避免整体分偏低时全面亮红灯。"
                ),
                ticket_ids=[],
                action=f"对{row['name']}做质检抽样，先看处理时限和话术，而不是再开一张大盘。",
            )
        )
    return anomalies


def _detect_fund_risk(tickets: list[dict[str, Any]]) -> list[dict[str, Any]]:
    hits = [t for t in tickets if _contains_any(str(t["description"]), FUND_KEYWORDS)]
    if not hits:
        return []
    return [
        _anomaly(
            anomaly_id="fund-risk",
            group="payment",
            atype="资金风险",
            severity="P0",
            title="出现重复扣款或多扣款",
            evidence=f"描述命中资金风险词，共 {len(hits)} 单；与支付突增合并解读，不再单列重复扣款主题。",
            ticket_ids=[t["ticket_id"] for t in hits],
            action="立刻核对支付对账，优先退回多扣金额并排查支付回调。",
        )
    ]


def _detect_channel_gap(stats: dict[str, Any]) -> list[dict[str, Any]]:
    by_name = {row["name"]: row for row in stats["channels"]}
    phone = by_name.get("电话")
    online = by_name.get("在线")
    if not phone or not online:
        return []
    if phone["count"] < 5 or online["count"] < 5:
        return []
    gap = round(online["avg_satisfaction"] - phone["avg_satisfaction"], 2)
    if gap < CHANNEL_CSAT_GAP:
        return []
    phone_unresolved = next((row["unresolved"] for row in stats["channel_category"] if row["channel"] == "电话"), 0)
    return [
        _anomaly(
            anomaly_id="channel-gap",
            group="channel",
            atype="渠道关联",
            severity="P1",
            title=f"电话渠道满意度比在线低 {gap} 分",
            evidence=(
                f"电话 {phone['count']} 单均分 {phone['avg_satisfaction']}，"
                f"在线 {online['count']} 单均分 {online['avg_satisfaction']}，差距 {gap} ≥ {CHANNEL_CSAT_GAP}。"
                f"电话未解决 {phone['unresolved']} 单。"
            ),
            ticket_ids=[],
            action="电话进线按退款/物流优先人工接入，不要再先走机器人。",
        )
    ]


def _overlap_ratio(left: list[str], right: list[str]) -> float:
    if not left or not right:
        return 0.0
    a, b = set(left), set(right)
    return len(a & b) / len(a | b)


def _dedupe_anomalies(anomalies: list[dict[str, Any]]) -> list[dict[str, Any]]:
    kept: list[dict[str, Any]] = []
    for item in anomalies:
        duplicate = False
        for existing in kept:
            same_group = item["group"] == existing["group"]
            high_overlap = _overlap_ratio(item["ticket_ids"], existing["ticket_ids"]) >= 0.5
            if same_group and (high_overlap or item["type"] == existing["type"]):
                duplicate = True
                break
        if not duplicate:
            kept.append(item)
    return kept


def build_focus(anomalies: list[dict[str, Any]]) -> list[dict[str, Any]]:
    grouped: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for item in anomalies:
        grouped[item["group"]].append(item)
    rank = {"payment": 0, "refund": 1, "backlog": 2, "channel": 3, "experience": 4, "watch": 5}
    stories: list[dict[str, Any]] = []
    for group, _ in sorted(grouped.items(), key=lambda kv: rank.get(kv[0], 9)):
        items = grouped[group]
        ids: list[str] = []
        for item in items:
            for ticket_id in item["ticket_ids"]:
                if ticket_id not in ids:
                    ids.append(ticket_id)
        lead = items[0]
        extra = f"（{', '.join(ids[:5])}）" if ids else ""
        preferred_action = next((i["action"] for i in items if i["type"] == "资金风险"), None)
        if preferred_action is None:
            preferred_action = next((i["action"] for i in items if i["severity"] == "P0"), lead["action"])
        stories.append(
            {
                "severity": lead["severity"],
                "title": lead["title"] + extra,
                "why": "；".join(i["evidence"].rstrip("。") for i in items[:2]),
                "action": preferred_action,
            }
        )
        if len(stories) >= FOCUS_LIMIT:
            break
    return stories


def build_actions(focus: list[dict[str, Any]], anomalies: list[dict[str, Any]]) -> list[str]:
    actions: list[str] = []
    for item in focus:
        if item["action"] not in actions:
            actions.append(item["action"])
    for item in anomalies:
        if item["action"] not in actions:
            actions.append(item["action"])
        if len(actions) >= FOCUS_LIMIT:
            break
    return actions[:FOCUS_LIMIT]


def analyze(tickets: list[dict[str, Any]]) -> dict[str, Any]:
    stats = aggregate(tickets)
    anomalies = detect_anomalies(tickets, stats)
    focus = build_focus(anomalies)
    return {
        "stats": stats,
        "anomalies": anomalies,
        "focus": focus,
        "actions": build_actions(focus, anomalies),
    }


def render_markdown(result: dict[str, Any]) -> str:
    stats = result["stats"]
    lines = [
        "# 客服工单趋势分析报告",
        "",
        f"样本：{stats['total']} 条工单，观察期 {stats['days'][0]} ~ {stats['days'][-1]}。",
        "",
        "## 今日关注",
        "",
    ]
    for item in result["focus"]:
        lines.append(f"- **{item['severity']} {item['title']}**")
        lines.append(f"  - 依据：{item['why']}")
        lines.append(f"  - 动作：{item['action']}")
    lines.extend(["", "## 分析维度与决策价值", ""])
    for name, value in stats["dimension_value"].items():
        lines.append(f"- **{name}**：{value}")

    lines.extend(["", "## 趋势摘要", "", "### 每日工单量", "", "| 日期 | 工单量 | 未解决 |", "|---|---:|---:|"])
    for row in stats["daily"]:
        lines.append(f"| {row['day']} | {row['count']} | {row['unresolved']} |")

    lines.extend(
        [
            "",
            "### 分类分布",
            "",
            "| 分类 | 数量 | 占比 | 平均处理时长(h) | 平均满意度 | 未解决 |",
            "|---|---:|---:|---:|---:|---:|",
        ]
    )
    for row in stats["categories"]:
        lines.append(
            f"| {row['name']} | {row['count']} | {row['share']:.1%} | "
            f"{row['avg_handle_hours']} | {row['avg_satisfaction']} | {row['unresolved']} |"
        )

    lines.extend(
        [
            "",
            "### 渠道 × 分类",
            "",
            "| 渠道 | 分类 | 数量 | 未解决 | 平均满意度 |",
            "|---|---|---:|---:|---:|",
        ]
    )
    for row in stats["channel_category"]:
        lines.append(
            f"| {row['channel']} | {row['category']} | {row['count']} | "
            f"{row['unresolved']} | {row['avg_satisfaction']} |"
        )

    lines.extend(
        [
            "",
            "### 渠道与优先级",
            "",
            f"- 平均处理时长 **{stats['avg_handle_hours']}** 小时，平均满意度 **{stats['avg_satisfaction']}**，未解决率 **{stats['unresolved_rate']:.1%}**。",
            "",
        ]
    )
    for row in stats["channels"]:
        lines.append(f"- 渠道 {row['name']}：{row['count']} 单，满意度 {row['avg_satisfaction']}，未解决 {row['unresolved']}")
    for row in stats["priorities"]:
        lines.append(f"- 优先级 {row['name']}：{row['count']} 单，未解决 {row['unresolved']}")

    if stats["high_unresolved"]:
        lines.extend(["", "### 高优先级未解决清单", ""])
        for row in stats["high_unresolved"]:
            lines.append(f"- {row['ticket_id']} / {row['category']} / {row['channel']}：{row['description']}")

    lines.extend(
        [
            "",
            "## 异常信号",
            "",
            "| 级别 | 类型 | 发现 | 判断依据 | 相关工单 | 建议动作 |",
            "|---|---|---|---|---|---|",
        ]
    )
    for item in result["anomalies"]:
        ids = ", ".join(item["ticket_ids"]) if item["ticket_ids"] else "—"
        lines.append(
            f"| {item['severity']} | {item['type']} | {item['title']} | {item['evidence']} | {ids} | {item['action']} |"
        )

    lines.extend(["", "## 行动建议", ""])
    for action in result["actions"]:
        lines.append(f"- {action}")
    lines.append("")
    return "\n".join(lines)


def _svg_stacked_bars(stats: dict[str, Any]) -> str:
    daily = stats["daily"]
    categories = [row["name"] for row in stats["categories"]]
    width, height = 640, 260
    left, bottom, top = 36, 36, 16
    chart_w, chart_h = width - left - 12, height - bottom - top
    max_count = max((row["count"] for row in daily), default=1) or 1
    bar_gap = 8
    bar_w = max(12, (chart_w / max(len(daily), 1)) - bar_gap)
    parts = [
        f'<svg class="chart" viewBox="0 0 {width} {height}" role="img" aria-label="每日工单量与分类构成">'
    ]
    for index, row in enumerate(daily):
        x = left + index * (bar_w + bar_gap)
        y = top + chart_h
        for cat_index, name in enumerate(categories):
            value = int(row["by_category"].get(name, 0))
            if not value:
                continue
            h = chart_h * value / max_count
            y -= h
            color = PALETTE[cat_index % len(PALETTE)]
            parts.append(
                f'<rect x="{x:.1f}" y="{y:.1f}" width="{bar_w:.1f}" height="{h:.1f}" fill="{color}">'
                f"<title>{html.escape(row['day'])} {html.escape(name)} {value}</title></rect>"
            )
        parts.append(
            f'<text x="{x + bar_w / 2:.1f}" y="{height - 14}" text-anchor="middle" fill="#94a3b8" font-size="11">{html.escape(row["day"][5:])}</text>'
        )
    parts.append("</svg>")
    legend = "".join(
        f'<span class="legend"><i style="background:{PALETTE[i % len(PALETTE)]}"></i>{html.escape(name)}</span>'
        for i, name in enumerate(categories)
    )
    return f'<div id="dailyChart">{"".join(parts)}<div class="legends">{legend}</div></div>'


def _svg_category_bars(stats: dict[str, Any]) -> str:
    rows = stats["categories"]
    width, row_h = 420, 28
    height = 24 + row_h * max(len(rows), 1)
    max_count = max((row["count"] for row in rows), default=1) or 1
    parts = [f'<svg class="chart" viewBox="0 0 {width} {height}" role="img" aria-label="分类分布">']
    for index, row in enumerate(rows):
        y = 8 + index * row_h
        bar = 220 * row["count"] / max_count
        color = PALETTE[index % len(PALETTE)]
        parts.append(f'<text x="0" y="{y + 14}" fill="#cbd5e1" font-size="12">{html.escape(row["name"])}</text>')
        parts.append(f'<rect x="84" y="{y}" width="{bar:.1f}" height="16" rx="4" fill="{color}"></rect>')
        parts.append(f'<text x="{92 + bar:.1f}" y="{y + 13}" fill="#94a3b8" font-size="12">{row["count"]}</text>')
    parts.append("</svg>")
    return f'<div id="catChart">{"".join(parts)}</div>'


def render_html(result: dict[str, Any]) -> str:
    stats = result["stats"]
    focus_html = "".join(
        (
            "<li>"
            f"<strong>{html.escape(item['severity'])} {html.escape(item['title'])}</strong>"
            f"<div class='sub'>{html.escape(item['why'])}</div>"
            f"<div>动作：{html.escape(item['action'])}</div>"
            "</li>"
        )
        for item in result["focus"]
    )
    action_html = "".join(f"<li>{html.escape(item)}</li>" for item in result["actions"])
    cat_rows = "".join(
        (
            "<tr>"
            f"<td>{html.escape(row['name'])}</td>"
            f"<td>{row['count']}</td>"
            f"<td>{row['share']:.1%}</td>"
            f"<td>{row['avg_handle_hours']}</td>"
            f"<td>{row['avg_satisfaction']}</td>"
            f"<td>{row['unresolved']}</td>"
            "</tr>"
        )
        for row in stats["categories"]
    )
    channel_rows = "".join(
        (
            "<tr>"
            f"<td>{html.escape(row['channel'])}</td>"
            f"<td>{html.escape(row['category'])}</td>"
            f"<td>{row['count']}</td>"
            f"<td>{row['unresolved']}</td>"
            f"<td>{row['avg_satisfaction']}</td>"
            "</tr>"
        )
        for row in stats["channel_category"]
    )
    anomaly_rows = "".join(
        (
            "<tr>"
            f"<td><span class='tag {item['severity'].lower()}'>{item['severity']}</span></td>"
            f"<td>{html.escape(item['type'])}</td>"
            f"<td>{html.escape(item['title'])}</td>"
            f"<td>{html.escape(item['evidence'])}</td>"
            f"<td>{html.escape(', '.join(item['ticket_ids']) if item['ticket_ids'] else '—')}</td>"
            f"<td>{html.escape(item['action'])}</td>"
            "</tr>"
        )
        for item in result["anomalies"]
    )
    backlog_rows = "".join(
        (
            "<tr>"
            f"<td>{html.escape(row['ticket_id'])}</td>"
            f"<td>{html.escape(row['category'])}</td>"
            f"<td>{html.escape(row['channel'])}</td>"
            f"<td>{html.escape(row['description'])}</td>"
            f"<td>{row['satisfaction']}</td>"
            "</tr>"
        )
        for row in stats["high_unresolved"]
    ) or "<tr><td colspan='5'>无</td></tr>"

    return f"""<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>客服工单趋势分析</title>
  <style>
    :root {{
      --card: #1e293b; --line: #334155; --text: #e2e8f0; --muted: #94a3b8; --accent: #38bdf8;
    }}
    * {{ box-sizing: border-box; }}
    body {{
      margin: 0; font-family: "Segoe UI", "PingFang SC", sans-serif;
      background: linear-gradient(180deg, #0b1220, #0f172a); color: var(--text);
    }}
    header {{ padding: 32px 40px 12px; }}
    h1 {{ margin: 0 0 8px; font-size: 28px; }}
    .sub {{ color: var(--muted); margin: 4px 0 8px; }}
    .grid {{ display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; padding: 0 40px 24px; }}
    .card {{ background: var(--card); border: 1px solid var(--line); border-radius: 16px; padding: 18px 20px; }}
    .kpi {{ font-size: 28px; font-weight: 700; color: var(--accent); }}
    .charts {{ display: grid; grid-template-columns: 1.4fr 1fr; gap: 16px; padding: 0 40px 24px; }}
    section {{ padding: 0 40px 28px; }}
    table {{ width: 100%; border-collapse: collapse; font-size: 14px; }}
    th, td {{ border-bottom: 1px solid var(--line); padding: 10px 8px; text-align: left; vertical-align: top; }}
    th {{ color: var(--muted); font-weight: 600; }}
    .tag {{ padding: 2px 8px; border-radius: 999px; font-size: 12px; }}
    .p0 {{ background: #7f1d1d; color: #fecaca; }}
    .p1 {{ background: #7c2d12; color: #fed7aa; }}
    ul {{ line-height: 1.7; }}
    .chart {{ width: 100%; height: auto; }}
    .legends {{ display: flex; flex-wrap: wrap; gap: 10px; margin-top: 8px; color: var(--muted); font-size: 12px; }}
    .legend i {{ display: inline-block; width: 10px; height: 10px; border-radius: 2px; margin-right: 6px; }}
    @media (max-width: 960px) {{
      .grid, .charts {{ grid-template-columns: 1fr; }}
      header, .grid, .charts, section {{ padding-left: 16px; padding-right: 16px; }}
    }}
  </style>
</head>
<body>
  <header>
    <h1>客服工单趋势分析</h1>
    <div class="sub">观察期 {html.escape(stats['days'][0])} ~ {html.escape(stats['days'][-1])} ｜ 样本 {stats['total']} 单 ｜ 图表离线 SVG，无需外网</div>
  </header>
  <div class="grid">
    <div class="card"><div class="sub">工单总量</div><div class="kpi">{stats['total']}</div></div>
    <div class="card"><div class="sub">未解决率</div><div class="kpi">{stats['unresolved_rate']:.1%}</div></div>
    <div class="card"><div class="sub">平均处理时长</div><div class="kpi">{stats['avg_handle_hours']}h</div></div>
    <div class="card"><div class="sub">平均满意度</div><div class="kpi">{stats['avg_satisfaction']}</div></div>
  </div>
  <div class="charts">
    <div class="card"><b>每日工单量与分类构成</b>{_svg_stacked_bars(stats)}</div>
    <div class="card"><b>分类分布</b>{_svg_category_bars(stats)}</div>
  </div>
  <section class="card" style="margin: 0 40px 24px;">
    <h2>今日关注</h2>
    <ul>{focus_html}</ul>
  </section>
  <section>
    <h2>分类统计</h2>
    <table>
      <thead><tr><th>分类</th><th>数量</th><th>占比</th><th>平均时长</th><th>满意度</th><th>未解决</th></tr></thead>
      <tbody>{cat_rows}</tbody>
    </table>
  </section>
  <section>
    <h2>渠道 × 分类</h2>
    <table>
      <thead><tr><th>渠道</th><th>分类</th><th>数量</th><th>未解决</th><th>满意度</th></tr></thead>
      <tbody>{channel_rows}</tbody>
    </table>
  </section>
  <section>
    <h2>高优先级未解决</h2>
    <table>
      <thead><tr><th>工单</th><th>分类</th><th>渠道</th><th>描述</th><th>满意度</th></tr></thead>
      <tbody>{backlog_rows}</tbody>
    </table>
  </section>
  <section>
    <h2>异常信号</h2>
    <table>
      <thead><tr><th>级别</th><th>类型</th><th>发现</th><th>判断依据</th><th>工单</th><th>建议</th></tr></thead>
      <tbody>{anomaly_rows}</tbody>
    </table>
  </section>
  <section>
    <h2>行动建议</h2>
    <ul>{action_html}</ul>
  </section>
</body>
</html>
"""


def write_reports(result: dict[str, Any], output_dir: str | Path) -> tuple[Path, Path]:
    out = Path(output_dir)
    out.mkdir(parents=True, exist_ok=True)
    md_path = out / "report.md"
    html_path = out / "report.html"
    md_path.write_text(render_markdown(result), encoding="utf-8")
    html_path.write_text(render_html(result), encoding="utf-8")
    return md_path, html_path


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="客服工单趋势分析")
    parser.add_argument("--input", default="data/tickets.json", help="工单 JSON 路径")
    parser.add_argument("--output-dir", default="output", help="报告输出目录")
    return parser


def main(argv: list[str] | None = None) -> int:
    if hasattr(sys.stdout, "reconfigure"):
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8")
    args = build_parser().parse_args(argv)
    try:
        tickets = load_tickets(args.input)
    except TicketLoadError as exc:
        print(f"加载失败: {exc}", file=sys.stderr)
        return 1
    result = analyze(tickets)
    md_path, html_path = write_reports(result, args.output_dir)
    print(f"分析完成：{result['stats']['total']} 条工单")
    print(f"Markdown: {md_path}")
    print(f"HTML: {html_path}")
    print("今日关注:")
    for item in result["focus"]:
        print(f"- {item['severity']} {item['title']}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
