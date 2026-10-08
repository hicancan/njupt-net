"""Create and inspect one independent Self management session."""

import argparse
from contextlib import contextmanager
import json
import re
import secrets
from pathlib import Path
from urllib.parse import urljoin, urlsplit

from credentials import credential
from network import Link, origin, shape
from pages import Page, fields, routes

BASE = "http://zfw.njupt.edu.cn:8080/Self/"
READ_PAGES = ("dashboard", "bill", "bill/userOnlineLog", "bill/monthPay", "bill/operatorLog",
              "service", "service/consumeProtect", "service/myMac", "service/operatorId",
              "service/userRecharge", "setting", "setting/personList")


def user_model(page):
    for match in re.finditer(r"\}\s*\)\s*\(\s*(\{[^\r\n]+\})\s*\)\s*;", page.script):
        value = json.loads(match[1])
        if value.get("userName"):
            return value
    raise ValueError("page has no server-rendered user model")


class Session:
    def __init__(self, link):
        self.link = link
        self.authenticated = False
        self.base = BASE

    def request(self, method, path, values=None):
        url = urljoin(self.base, path)
        if origin(url) != origin(self.base) or not urlsplit(url).path.startswith("/Self/"):
            raise ValueError("request leaves Self management origin")
        return self.link.request(method, url, values)

    def page(self, path):
        response = self.request("GET", path)
        if response.status != 200:
            raise ValueError(f"{path}: HTTP {response.status}")
        page = Page(response.text())
        if self.authenticated and page.form("/Self/login/verify") is not None:
            self.authenticated = False
            raise ValueError("management session expired")
        return page

    def query(self, path, values=None):
        if not self.authenticated:
            raise ValueError("management login is required")
        params = dict(values or {})
        params["t"] = str(secrets.randbelow(10**15) / 10**15)
        response = self.request("GET", path, params)
        if response.status != 200:
            raise ValueError(f"{path}: HTTP {response.status}")
        return response.json()

    def login(self, account, password, initialize_image=True):
        if self.authenticated:
            raise ValueError("session is already authenticated")
        response = self.request("GET", "login/")
        page = Page(response.text())
        form = page.form("/Self/login/verify")
        values = fields(form)
        required = {"foo", "bar", "checkcode", "account", "password", "code"}
        if not required.issubset(values) or not values["checkcode"]:
            raise ValueError("unexpected login form fields")
        if not re.search(r"\bvar\s+md5\s*=\s*false\s*;", page.script):
            raise ValueError("login password contract changed")
        captcha = page.root.find(id="randomDiv")
        if captcha is None or "hide" not in captcha.attrs.get("class", "").split():
            raise ValueError("normal account/password login is not available")
        if initialize_image:
            image = self.request("GET", "login/randomCode", {"t": str(secrets.randbelow(10**12))})
            if image.status != 200 or not image.body.startswith(b"\x89PNG\r\n\x1a\n"):
                raise ValueError("login session image initialization did not return PNG")
        values.update({"account": account, "password": password})
        response = self.request("POST", urljoin(response.url, form.attrs["action"]), values)
        final_path = re.sub(r";jsessionid=[^/;?]*", "", urlsplit(response.url).path)
        if final_path != "/Self/dashboard":
            raise ValueError("management login was rejected")
        self.authenticated = True
        if user_model(Page(response.text())).get("userName") != account:
            raise ValueError("authenticated account differs from requested account")
        return response

    def login_bridge(self, bridge_url, expected_account):
        from urllib.parse import parse_qs
        u = urlsplit(bridge_url)
        query = parse_qs(u.query)
        if self.authenticated or u.scheme != "http" or u.hostname not in {"zfw.njupt.edu.cn", "10.10.244.240"} or \
                u.port != 8080 or u.path != "/Self/login/eportalLogin" or u.username or u.password or u.fragment or \
                set(query) != {"params", "timestamp", "sign"} or any(len(values) != 1 or not values[0] for values in query.values()):
            raise ValueError("unexpected deployed Self bridge endpoint")
        self.base = f"http://{u.netloc}/Self/"
        response = self.request("GET", bridge_url)
        path = re.sub(r";jsessionid=[^/;?]*", "", urlsplit(response.url).path)
        if response.status != 200 or path != "/Self/dashboard":
            raise ValueError("bridge did not reach an authenticated dashboard")
        self.authenticated = True
        if user_model(Page(response.text())).get("userName") != expected_account:
            raise ValueError("bridge authenticated an unexpected account")
        return response

    def logout(self):
        if not self.authenticated:
            return
        response = self.request("GET", "login/logout")
        if Page(response.text()).form("/Self/login/verify") is None:
            raise ValueError("logout did not return login form")
        self.authenticated = False

    def language(self, language):
        if language not in {"English", "zh_cn"}:
            raise ValueError("unknown deployed language value")
        response = self.request("GET", "login/changeLanguage", {"language": language})
        page_response = self.request("GET", "login/")
        button = Page(page_response.text()).root.find(id="language")
        expected = "中文" if language == "English" else "English"
        if button is None or button.text().strip() != expected:
            raise ValueError("server did not apply requested Cookie-session language")
        return {"response": response.summary(), "button": expected, "verified": True}

    def investigate(self):
        result, script_urls = {}, set()
        for path in READ_PAGES:
            response = self.request("GET", path)
            record = response.summary()
            if response.status == 200:
                page = Page(response.text())
                record["inventory"] = page.inventory(response.url)
                script_urls.update(record["inventory"]["scripts"])
            result[path] = record
        script_results = {}
        for url in sorted(script_urls):
            if origin(url) != origin(self.base):
                continue
            response = self.link.request("GET", url)
            record = response.summary()
            if response.status == 200:
                record["literal_route_candidates"] = routes(response.text())
            script_results[urlsplit(url).path] = record
        return {"pages": result, "scripts": script_results}


def arguments(description, authenticated=True):
    parser = argparse.ArgumentParser(description=description)
    parser.add_argument("--source", required=True)
    parser.add_argument("--timeout", type=float, default=10)
    parser.add_argument("--output", help="new directory for raw observations; may contain account data")
    if authenticated:
        parser.add_argument("--config", required=True)
        parser.add_argument("--account", required=True, help="configured account alias")
        parser.add_argument("--self-url-file", help="explicit file containing a deployed Self bridge URL")
    return parser


@contextmanager
def opened(args):
    session = Session(Link(args.source, args.timeout, args.output))
    try:
        account, password = credential(args.config, args.account)
        if args.self_url_file:
            session.login_bridge(Path(args.self_url_file).read_text(encoding="utf-8-sig").strip(), account)
        else:
            session.login(account, password)
        yield session
    finally:
        session.logout()


def main():
    parser = arguments(__doc__)
    parser.add_argument("--skip-image", action="store_true", help="explicitly test login without image initialization")
    parser.add_argument("--language", choices=("English", "zh_cn"))
    args = parser.parse_args()
    session = Session(Link(args.source, args.timeout, args.output))
    try:
        account, password = credential(args.config, args.account)
        if args.self_url_file:
            login_response = session.login_bridge(Path(args.self_url_file).read_text(encoding="utf-8-sig").strip(), account)
        else:
            login_response = session.login(account, password, initialize_image=not args.skip_image)
        result = {"login": login_response.summary(), "inventory": session.investigate()}
        if args.language:
            result["language"] = session.language(args.language)
        print(json.dumps(result, ensure_ascii=False, indent=2))
    finally:
        session.logout()


if __name__ == "__main__":
    main()
