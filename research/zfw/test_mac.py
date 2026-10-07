import unittest
from unittest.mock import Mock

from zfw.mac import binding_present


def response(total, rows):
    value = Mock()
    value.json.return_value = {"total": total, "rows": rows}
    value.summary.return_value = {"status": 200}
    return value


def row(index):
    return [0, f"{index:012x}", None, None, None]


class MACTests(unittest.TestCase):
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
