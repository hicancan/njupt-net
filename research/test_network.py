import gzip
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import threading
import unittest
from unittest.mock import patch

from network import Link, shape


class Handler(BaseHTTPRequestHandler):
    requests = []

    def do_GET(self):
        type(self).requests.append((self.client_address[0], self.path, self.headers.get("Cookie")))
        if self.path == "/cookie":
            self.send_response(200)
            self.send_header("Set-Cookie", "session=example; Path=/")
            data = b"cookie"
        elif self.path == "/gzip":
            self.send_response(200)
            self.send_header("Content-Encoding", "gzip")
            data = gzip.compress(b"decoded page")
        elif self.path == "/redirect":
            self.send_response(302)
            self.send_header("Location", "/done")
            data = b""
        elif self.path == "/outside":
            self.send_response(302)
            self.send_header("Location", "http://localhost:1/")
            data = b""
        elif self.path == "/invalid-port":
            self.send_response(302)
            self.send_header("Location", "http://127.0.0.1:synthetic-sensitive-value/Self/?sign=synthetic-sensitive-value")
            data = b""
        elif self.path == "/invalid-address":
            self.send_response(302)
            self.send_header("Location", "http://[synthetic-sensitive-value/?sign=synthetic-sensitive-value")
            data = b""
        else:
            self.send_response(200)
            data = b"done"
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        self.rfile.read(int(self.headers.get("Content-Length", "0")))
        self.send_response(307)
        self.send_header("Location", "/done")
        self.send_header("Content-Length", "0")
        self.end_headers()

    def log_message(self, *args):
        pass


class NetworkTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.base = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join()

    def test_explicit_source_and_cookie_isolation(self):
        first, second = Link("127.0.0.1"), Link("127.0.0.1")
        first.request("GET", self.base + "/cookie")
        first.request("GET", self.base + "/done")
        second.request("GET", self.base + "/done")
        self.assertEqual(Handler.requests[-2], ("127.0.0.1", "/done", "session=example"))
        self.assertIsNone(Handler.requests[-1][2])

    def test_proxy_environment_does_not_change_direct_request(self):
        with patch.dict("os.environ", {"HTTP_PROXY": "http://127.0.0.1:1", "HTTPS_PROXY": "http://127.0.0.1:1"}):
            self.assertEqual(Link("127.0.0.1").request("GET", self.base).status, 200)

    def test_gzip_is_decoded(self):
        response = Link("127.0.0.1").request("GET", self.base + "/gzip")
        self.assertEqual(response.body, b"decoded page")
        self.assertEqual(response.summary()["content_encoding"], "gzip")

    def test_same_origin_redirect(self):
        response = Link("127.0.0.1").request("GET", self.base + "/redirect")
        self.assertTrue(response.url.endswith("/done"))

    def test_cross_origin_redirect_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "destination"):
            Link("127.0.0.1").request("GET", self.base + "/outside")

    def test_replay_redirect_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "replay"):
            Link("127.0.0.1").request("POST", self.base, {"password": "synthetic"})

    def test_malformed_location_does_not_expose_authorization_values(self):
        for path in ("/invalid-port", "/invalid-address"):
            with self.subTest(path=path), self.assertRaises(ValueError) as raised:
                Link("127.0.0.1").request("GET", self.base + path)
            self.assertNotIn("synthetic-sensitive-value", str(raised.exception))

    def test_summary_omits_values(self):
        self.assertEqual(shape({"account": "synthetic", "rows": [[1, "secret"]]}),
                         {"account": "str", "rows": {"type": "array", "count": 1,
                                                     "first": {"type": "array", "count": 2, "first": "int"}}})

    def test_unspecified_source_is_rejected(self):
        with self.assertRaises(ValueError):
            Link("0.0.0.0")
