import json
import unittest
from unittest.mock import Mock, call, patch

from pages import Page
from zfw.operator import inspect, main


def operator_page(njxy_account="telecom-account", njxy_password="telecom-secret",
                  cmcc_account="mobile-account", cmcc_password="mobile-secret", token="fresh-token"):
    return Page(f'''<form action="/Self/service/bind-operator" method="post">
        <input type="hidden" name="csrftoken" value="{token}">
        <input name="FLDEXTRA1" value="{njxy_account}">
        <input type="password" name="FLDEXTRA2" value="{njxy_password}">
        <input name="FLDEXTRA3" value="{cmcc_account}">
        <input type="password" name="FLDEXTRA4" value="{cmcc_password}">
        </form>''')


class OperatorTests(unittest.TestCase):
    def test_unbind_submits_one_fresh_form_and_verifies_independent_readback(self):
        for selected, cleared in (
            ("cmcc", {"cmcc_account": "", "cmcc_password": ""}),
            ("njxy", {"njxy_account": "", "njxy_password": ""}),
        ):
            with self.subTest(operator=selected):
                session = Mock()
                session.page.side_effect = [operator_page(), operator_page(**cleared)]
                session.request.return_value.summary.return_value = {"status": 200}
                with patch("zfw.operator.getpass.getpass") as prompt:
                    result = inspect(session, unbind=selected)
                prompt.assert_not_called()
                expected = {"csrftoken": "fresh-token", "FLDEXTRA1": "telecom-account",
                            "FLDEXTRA2": "telecom-secret", "FLDEXTRA3": "mobile-account",
                            "FLDEXTRA4": "mobile-secret"}
                for key in (("FLDEXTRA3", "FLDEXTRA4") if selected == "cmcc" else ("FLDEXTRA1", "FLDEXTRA2")):
                    expected[key] = ""
                self.assertEqual(session.mock_calls[:4], [
                    call.page("service/operatorId"),
                    call.request("POST", "/Self/service/bind-operator", expected),
                    call.request().summary(),
                    call.page("service/operatorId"),
                ])
                session.request.assert_called_once()
                self.assertTrue(result["account_cleared"])
                self.assertTrue(result["password_cleared"])
                self.assertTrue(result["other_operator_unchanged"])
                serialized = json.dumps(result)
                for secret in ("telecom-secret", "mobile-secret", "fresh-token"):
                    self.assertNotIn(secret, serialized)

    def test_successful_http_status_does_not_prove_fields_were_cleared(self):
        session = Mock()
        session.page.side_effect = [operator_page(), operator_page()]
        session.request.return_value.summary.return_value = {"status": 200}
        result = inspect(session, unbind="cmcc")
        self.assertFalse(result["account_cleared"])
        self.assertFalse(result["password_cleared"])
        session.request.assert_called_once()

    def test_unbind_detects_partial_clear_and_other_operator_change(self):
        session = Mock()
        session.page.side_effect = [operator_page(), operator_page(cmcc_account="", njxy_password="changed")]
        session.request.return_value.summary.return_value = {"status": 200}
        result = inspect(session, unbind="cmcc")
        self.assertTrue(result["account_cleared"])
        self.assertFalse(result["password_cleared"])
        self.assertFalse(result["other_operator_unchanged"])

    def test_conflicting_actions_fail_before_any_request(self):
        session = Mock()
        with self.assertRaises(ValueError):
            inspect(session, operator="cmcc", unbind="cmcc")
        self.assertEqual(session.mock_calls, [])

    def test_cli_rejects_conflicting_actions_before_login(self):
        args = ["operator", "--source", "192.0.2.1", "--config", "unused.json", "--account", "test",
                "--bind", "cmcc", "--unbind", "cmcc"]
        with patch("sys.argv", args), patch("zfw.operator.opened") as opened, patch("sys.stderr"):
            with self.assertRaises(SystemExit) as error:
                main()
        self.assertEqual(error.exception.code, 2)
        opened.assert_not_called()
