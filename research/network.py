"""HTTP experiments through one explicitly selected IPv4, without a proxy."""

from dataclasses import dataclass
from email.message import Message
import hashlib
import gzip
import http.client
import http.cookiejar
import io
import ipaddress
import json
from pathlib import Path
import re
import ssl
import zlib
from urllib.parse import urlencode, urljoin, urlsplit, urlunsplit
from urllib.request import Request


@dataclass
class Response:
    status: int
    url: str
    headers: Message
    body: bytes

    def info(self):
        return self.headers

    def text(self, encoding="utf-8"):
        return self.body.decode(encoding)

    def json(self):
        if not self.body.strip():
            raise ValueError("empty response is not JSON")
        return json.loads(self.body)

    def summary(self):
        return {"status": self.status, "path": safe_path(self.url),
                "content_type": self.headers.get("Content-Type", ""),
                "content_encoding": self.headers.get("Content-Encoding", ""),
                "bytes": len(self.body), "sha256": hashlib.sha256(self.body).hexdigest()}


def _split_url(url):
    try:
        value = urlsplit(url)
        value.port
        return value
    except ValueError:
        raise ValueError("invalid HTTP endpoint") from None


def safe_path(url):
    return re.sub(r"(?i);jsessionid=[^/;?]*", ";jsessionid=<session>", _split_url(url).path)


def origin(url):
    u = _split_url(url)
    return u.scheme, u.hostname, u.port or (443 if u.scheme == "https" else 80)


class Link:
    """One Cookie session. Each request binds its socket to source."""

    def __init__(self, source, timeout=10, output=None):
        self.source = str(ipaddress.IPv4Address(source))
        if self.source == "0.0.0.0" or timeout <= 0:
            raise ValueError("a concrete source IPv4 and positive timeout are required")
        self.timeout = timeout
        self.cookies = http.cookiejar.CookieJar()
        self.output = Path(output) if output else None
        self.responses = []
        if self.output:
            self.output.mkdir(parents=True, exist_ok=True)

    def request(self, method, url, values=None, redirects=True):
        method = method.upper()
        if method not in {"GET", "POST"}:
            raise ValueError("experiments support GET and form POST")
        initial_origin = origin(url)
        for count in range(11):
            u = _split_url(url)
            if u.scheme not in {"http", "https"} or not u.hostname or u.username or u.password:
                raise ValueError("expected an HTTP endpoint without URL credentials")
            query, body = u.query, None
            if values is not None:
                encoded = urlencode(values, doseq=True)
                if method == "GET":
                    query = "&".join(part for part in (query, encoded) if part)
                else:
                    body = encoded.encode("utf-8")
            target = urlunsplit(("", "", u.path or "/", query, ""))
            actual = urlunsplit((u.scheme, u.netloc, u.path or "/", query, ""))
            req = Request(actual, method=method)
            self.cookies.add_cookie_header(req)
            headers = {"User-Agent": "njupt-net-research/1.0", "Connection": "close"}
            if req.has_header("Cookie"):
                headers["Cookie"] = req.get_header("Cookie")
            if body is not None:
                headers["Content-Type"] = "application/x-www-form-urlencoded"
            options = {"timeout": self.timeout, "source_address": (self.source, 0)}
            if u.scheme == "https":
                conn = http.client.HTTPSConnection(u.hostname, u.port, context=ssl.create_default_context(), **options)
            else:
                conn = http.client.HTTPConnection(u.hostname, u.port, **options)
            try:
                conn.request(method, target, body=body, headers=headers)
                incoming = conn.getresponse()
                data = incoming.read(32 * 1024 * 1024 + 1)
                if len(data) > 32 * 1024 * 1024:
                    raise ValueError("response exceeds 32 MiB")
                encoding = incoming.headers.get("Content-Encoding", "").strip().lower()
                if encoding == "gzip":
                    with gzip.GzipFile(fileobj=io.BytesIO(data)) as archive:
                        data = archive.read(32 * 1024 * 1024 + 1)
                elif encoding == "deflate":
                    decoder = zlib.decompressobj()
                    data = decoder.decompress(data, 32 * 1024 * 1024 + 1)
                    if not decoder.eof and len(data) <= 32 * 1024 * 1024:
                        raise ValueError("incomplete deflate response")
                elif encoding not in {"", "identity"}:
                    raise ValueError("unsupported HTTP content encoding")
                if len(data) > 32 * 1024 * 1024:
                    raise ValueError("decoded response exceeds 32 MiB")
                response = Response(incoming.status, actual, incoming.headers, data)
                self.cookies.extract_cookies(response, req)
            except (OSError, http.client.HTTPException) as exc:
                raise RuntimeError(f"{method} {safe_path(url)}: {type(exc).__name__}") from None
            finally:
                conn.close()
            self.responses.append(response.summary())
            if self.output:
                name = f"{len(self.responses):03d}-{re.sub(r'[^a-zA-Z0-9.-]+', '-', safe_path(url)).strip('-') or 'root'}"
                with (self.output / (name + ".body")).open("xb") as stream:
                    stream.write(data)
                with (self.output / (name + ".json")).open("x", encoding="utf-8") as stream:
                    json.dump(response.summary(), stream, ensure_ascii=False, indent=2)
            if response.status in {301, 302, 303, 307, 308} and redirects:
                if response.status in {307, 308}:
                    raise ValueError("redirect would replay a request")
                if count == 10:
                    raise ValueError("too many redirects")
                try:
                    destination = urljoin(actual, response.headers.get("Location", ""))
                except ValueError:
                    raise ValueError("invalid redirect destination") from None
                if origin(destination) != initial_origin or destination == actual:
                    raise ValueError("unexpected redirect destination")
                url, values = destination, None
                if response.status == 303 or method == "POST":
                    method = "GET"
                continue
            return response
        raise AssertionError("unreachable redirect state")


def shape(value):
    """Describe observed JSON structure without printing account data."""
    if isinstance(value, dict):
        return {key: shape(item) for key, item in value.items()}
    if isinstance(value, list):
        return {"type": "array", "count": len(value), "first": shape(value[0]) if value else None}
    return type(value).__name__
