from pathlib import Path
import unittest
from unittest.mock import Mock
from pages import Page
from zfw.bills import parameters
from zfw.session import Session, user_model


class SessionTests(unittest.TestCase):
    def test_model_is_parsed_as_json_without_execution(self):
        page = Page('<script>(function(user){} )({"userName":"example","installmentFlag":999999});</script>')
        self.assertEqual(user_model(page)["installmentFlag"], 999999)

    def test_outside_origin_rejected_before_request(self):
        link = Mock()
        with self.assertRaises(ValueError):
            Session(link).request("GET", "http://example.test/Self/dashboard")
        link.request.assert_not_called()

    def test_bridge_rejects_unknown_host_or_incomplete_signature(self):
        link = Mock()
        for url in ("http://example.test:8080/Self/login/eportalLogin?params=a&timestamp=b&sign=c",
                    "http://zfw.njupt.edu.cn:8080/Self/login/eportalLogin?params=a"):
            with self.assertRaises(ValueError):
                Session(link).login_bridge(url, "example")
        link.request.assert_not_called()

    def test_monthly_year_comes_from_current_page(self):
        page = Page('<select id="year"><option>2025</option><option selected>2026</option></select>')
        self.assertEqual(parameters(page, "monthly")["year"], "2026")
        with self.assertRaises(ValueError):
            parameters(page, "monthly", year=2024)

    def test_bill_range_and_sort(self):
        with self.assertRaises(ValueError):
            parameters(Page(""), "online", start="2026-01-01", end="2026-04-01")
        with self.assertRaises(ValueError):
            parameters(Page(""), "operations", sort="3")

    def test_synthetic_login_form(self):
        page = Page((Path(__file__).parent / "samples" / "login-form.html").read_text(encoding="utf-8"))
        self.assertIsNotNone(page.form("/Self/login/verify"))
        self.assertIn("hide", page.root.find(id="randomDiv").attrs["class"])
