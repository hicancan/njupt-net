import unittest
from p.access import selected_session


class StateTests(unittest.TestCase):
    def test_only_matching_source_is_selected(self):
        value = {"result": 1, "total": 1, "list": [{"online_ip": "192.0.2.1", "user_account": "example"}]}
        self.assertIsNone(selected_session(value, "192.0.2.2"))
        self.assertEqual(selected_session(value, "192.0.2.1")["user_account"], "example")

    def test_rejection_is_not_offline(self):
        with self.assertRaises(ValueError):
            selected_session({"result": 0, "msg": "authentication rejected"}, "192.0.2.1")

    def test_duplicate_identity_or_bad_total_is_rejected(self):
        row = {"online_ip": "192.0.2.1", "user_account": "example"}
        for total, rows in ((2, [row]), (2, [row, row])):
            with self.assertRaises(ValueError):
                selected_session({"result": 1, "total": total, "list": rows}, "192.0.2.1")
