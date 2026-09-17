import json
import tempfile
import unittest
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from analyze import TicketLoadError, analyze, load_tickets, main  # noqa: E402


class LoadTicketsTest(unittest.TestCase):
    def test_load_real_dataset(self):
        tickets = load_tickets(ROOT / "data" / "tickets.json")
        self.assertEqual(len(tickets), 50)
        self.assertEqual(tickets[0]["ticket_id"], "T001")

    def test_missing_file(self):
        with self.assertRaises(TicketLoadError):
            load_tickets(ROOT / "data" / "not-exists.json")

    def test_invalid_json_exits_without_report(self):
        with tempfile.TemporaryDirectory() as tmp:
            bad = Path(tmp) / "bad.json"
            bad.write_text("{not-json", encoding="utf-8")
            out = Path(tmp) / "out"
            code = main(["--input", str(bad), "--output-dir", str(out)])
            self.assertEqual(code, 1)
            self.assertFalse((out / "report.md").exists())


class AnomalyTest(unittest.TestCase):
    def _ticket(self, **kwargs):
        base = {
            "ticket_id": "T000",
            "created_at": "2024-06-01 09:00",
            "category": "支付问题",
            "description": "普通咨询",
            "priority": "中",
            "resolution_time_hours": 1,
            "satisfaction": 4,
            "channel": "在线",
            "is_resolved": True,
        }
        base.update(kwargs)
        return base

    def test_category_surge(self):
        tickets = []
        for day in range(1, 5):
            tickets.append(self._ticket(ticket_id=f"A{day}", created_at=f"2024-06-0{day} 09:00", category="商品咨询"))
        for day in range(5, 9):
            tickets.append(
                self._ticket(
                    ticket_id=f"B{day}",
                    created_at=f"2024-06-0{day} 09:00",
                    category="支付问题",
                    description="重复扣款请退回",
                )
            )
            tickets.append(
                self._ticket(
                    ticket_id=f"C{day}",
                    created_at=f"2024-06-0{day} 10:00",
                    category="支付问题",
                    description="银行卡已经扣款了但订单没成功",
                )
            )
        result = analyze(self._reload(tickets))
        types = {item["type"] for item in result["anomalies"]}
        titles = " ".join(item["title"] for item in result["anomalies"])
        self.assertIn("分类突增", types)
        self.assertIn("支付问题", titles)

    def test_small_sample_not_surge(self):
        tickets = [
            self._ticket(ticket_id="C1", created_at="2024-06-01 09:00", category="投诉", description="态度差"),
            self._ticket(ticket_id="P1", created_at="2024-06-02 09:00", category="商品咨询", description="问尺寸"),
            self._ticket(ticket_id="C2", created_at="2024-06-05 09:00", category="投诉", description="等太久"),
            self._ticket(ticket_id="C3", created_at="2024-06-06 09:00", category="投诉", description="还是投诉"),
            self._ticket(ticket_id="P2", created_at="2024-06-07 09:00", category="商品咨询", description="问颜色"),
            self._ticket(ticket_id="P3", created_at="2024-06-08 09:00", category="商品咨询", description="问材质"),
        ]
        result = analyze(self._reload(tickets))
        surge = [item for item in result["anomalies"] if item["type"] == "分类突增" and "投诉" in item["title"]]
        self.assertEqual(surge, [])

    def test_repeat_issue_cluster(self):
        tickets = [
            self._ticket(ticket_id="T1", description="重复扣款 请帮我退回来", created_at="2024-06-01 09:00"),
            self._ticket(ticket_id="T2", description="微信支付扣了两次钱", created_at="2024-06-02 09:00"),
            self._ticket(ticket_id="T3", description="两个都扣钱了", created_at="2024-06-03 09:00"),
            self._ticket(ticket_id="T4", description="咨询尺码", category="商品咨询", created_at="2024-06-04 09:00"),
        ]
        result = analyze(self._reload(tickets))
        money = [item for item in result["anomalies"] if item["type"] == "资金风险"]
        self.assertTrue(money)
        self.assertGreaterEqual(len(money[0]["ticket_ids"]), 3)

    def test_fund_risk(self):
        tickets = [
            self._ticket(ticket_id="T9", description="订单金额是199 扣了我299 多扣了100块"),
            self._ticket(ticket_id="T8", description="咨询发货时间", category="物流查询"),
        ]
        result = analyze(self._reload(tickets))
        fund = [item for item in result["anomalies"] if item["type"] == "资金风险"]
        self.assertEqual(len(fund), 1)
        self.assertIn("T9", fund[0]["ticket_ids"])

    def test_freight_question_is_not_slow_refund(self):
        tickets = [
            self._ticket(ticket_id="T011x", category="退款退货", description="质量问题退货运费应该你们出吧"),
            self._ticket(ticket_id="T021x", category="退款退货", description="退货运费太贵了 能不能便宜点"),
            self._ticket(ticket_id="T001x", category="退款退货", description="退货申请提交一周了还在审核中"),
            self._ticket(ticket_id="T007x", category="退款退货", description="退货寄回去三天了钱还没退"),
            self._ticket(ticket_id="T031x", category="退款退货", description="垫付了运费说好了退给我 到现在没退"),
        ]
        result = analyze(self._reload(tickets))
        refund = [item for item in result["anomalies"] if item["id"] == "theme-slow_refund"]
        self.assertTrue(refund)
        self.assertIn("T031x", refund[0]["ticket_ids"])
        self.assertNotIn("T011x", refund[0]["ticket_ids"])
        self.assertNotIn("T021x", refund[0]["ticket_ids"])

    def _reload(self, tickets):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "tickets.json"
            path.write_text(json.dumps(tickets, ensure_ascii=False), encoding="utf-8")
            return load_tickets(path)


class ReportTest(unittest.TestCase):
    def test_real_report_contains_required_sections(self):
        tickets = load_tickets(ROOT / "data" / "tickets.json")
        result = analyze(tickets)
        self.assertEqual(len(result["focus"]), 3)
        groups_in_titles = " ".join(item["title"] for item in result["focus"])
        self.assertIn("支付", groups_in_titles)
        self.assertTrue(result["stats"]["channel_category"])
        from analyze import render_markdown, render_html

        md = render_markdown(result)
        html = render_html(result)
        self.assertIn("今日关注", md)
        self.assertIn("渠道 × 分类", md)
        self.assertIn("异常信号", md)
        self.assertIn("行动建议", md)
        self.assertIn("dailyChart", html)
        self.assertIn("catChart", html)
        self.assertIn("<svg", html)
        self.assertNotIn("chart.js", html.lower())
        self.assertLessEqual(len(result["actions"]), 3)
        csat_alerts = [item for item in result["anomalies"] if item["type"] == "低满意度"]
        self.assertLessEqual(len(csat_alerts), 2)
        complaint_surge = [
            item for item in result["anomalies"] if item["type"] == "分类突增" and "投诉" in item["title"]
        ]
        self.assertEqual(complaint_surge, [])


if __name__ == "__main__":
    unittest.main()
