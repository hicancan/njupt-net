import unittest
from unittest.mock import Mock

from zfw.mac import binding_present, bindings, initialization


def response(total, rows):
    value = Mock()
    value.json.return_value = {"total": total, "rows": rows}
    value.summary.return_value = {"status": 200}
    return value


def row(index):
    return ["0", f"{index:012x}", None, None, None]


class MACTests(unittest.TestCase):
    def test_observed_string_online_state(self):
        value = {"total": 1, "rows": [["1", "aabbccddeeff", "#PC", "2026-10-08 17:34:00", "192.0.2.10"]]}
        self.assertEqual(bindings(value, 10), (1, value["rows"]))
        for online in (0, 1, True, None, "2"):
            with self.subTest(online=online):
                value["rows"][0][0] = online
                with self.assertRaises(ValueError):
                    bindings(value, 10)

    def test_direct_query_precedes_page_initialization(self):
        session = Mock()
        value = {"total": 1, "rows": [row(1)]}
        session.query.side_effect = [value, value]
        result = initialization(session)
        self.assertTrue(result["direct_query_valid"])
        self.assertTrue(result["same_after_page"])
        self.assertEqual([call[0] for call in session.mock_calls], ["query", "page", "query"])

    def test_binding_moved_to_later_page_remains_present(self):
        session = Mock()
        session.request.side_effect = [response(101, [row(index) for index in range(100)]), response(101, [row(100)])]
        observations = []
        self.assertTrue(binding_present(session, row(100)[1], observations))
        self.assertEqual(len(observations), 2)
        self.assertEqual(session.request.call_args.args[2]["pageNumber"], 2)

    def test_absence_requires_complete_and_consistent_pages(self):
        for pages in (
            [response(101, [row(index) for index in range(100)]), response(100, [])],
            [response(101, [row(index) for index in range(100)]), response(101, [])],
            [response(101, [row(index) for index in range(100)]), response(101, [row(0)])],
        ):
            with self.subTest(pages=pages):
                session = Mock()
                session.request.side_effect = pages
                with self.assertRaises(ValueError):
                    binding_present(session, row(200)[1], [])

    def test_valid_empty_list_proves_absence(self):
        session = Mock()
        session.request.return_value = response(0, [])
        self.assertFalse(binding_present(session, row(200)[1], []))
