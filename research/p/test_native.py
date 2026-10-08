import unittest
from email.message import Message

from network import Response
from p.native import query


class QueryTests(unittest.TestCase):
    def test_status_uses_only_callback_on_each_endpoint(self):
        class FakeLink:
            source = "192.0.2.10"

            def request(self, method, url, values):
                self.observed = method, url, values
                body = (values["callback"] + '( {"result":0,"msg":"获取用户在线信息数据为空！"} );').encode()
                return Response(200, url, Message(), body)

        for port in (801, 802, 803, 804):
            link = FakeLink()
            self.assertEqual(query(link, port, "online_list")["result"], 0)
            method, url, values = link.observed
            self.assertEqual(method, "GET")
            scheme = "http" if port in (801, 803) else "https"
            self.assertEqual(url, f"{scheme}://p.njupt.edu.cn:{port}/eportal/portal/online_list")
            self.assertEqual(set(values), {"callback"})
            self.assertRegex(values["callback"], r"^dr\d+$")

    def test_invalid_query_does_not_make_a_request(self):
        for port, route in ((80, "online_list"), (804, "login")):
            with self.assertRaises(ValueError):
                query(object(), port, route)
