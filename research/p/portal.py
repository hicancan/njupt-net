"""Fetch the current Portal bootstrap, configuration, scripts and templates."""

import argparse
import base64
import json
import re
import secrets
from urllib.parse import parse_qs, urljoin, urlsplit

from network import Link, shape
from pages import Page, assignment, routes

BASE = "https://p.njupt.edu.cn"
API = BASE + ":804/eportal/portal/"
TERMINALS = {"pc": 1, "mobile": 2, "hipad": 3, "vipad": 4}


def b64(value):
    return base64.b64encode(value.encode("utf-8")).decode("ascii")


def decode_jsonp(data, callback):
    text = data.decode("utf-8").strip()
    match = re.fullmatch(re.escape(callback) + r"\(\s*(.*?)\s*\)\s*;?", text, re.S)
    if not match:
        raise ValueError("JSONP callback or framing differs from request")
    result = json.loads(match[1])
    if not isinstance(result, dict):
        raise ValueError("Portal JSONP must contain an object")
    return result


def object_source(source, name):
    """Read one literal object's body for static dispatch inspection."""
    matches = list(re.finditer(r"\b(?:var\s+)?" + re.escape(name) + r"\s*=\s*\{", source))
    if not matches:
        return ""
    # The combined bundle first defines disabled stubs, then the full modules.
    match = matches[-1]
    start, depth, index = match.end() - 1, 1, match.end()
    while index < len(source):
        char = source[index]
        if char in "'\"`":
            quote = char
            index += 1
            while index < len(source):
                if source[index] == "\\":
                    index += 2
                elif source[index] == quote:
                    break
                else:
                    index += 1
        elif source.startswith("//", index):
            end = source.find("\n", index)
            index = len(source) if end < 0 else end
        elif source.startswith("/*", index):
            end = source.find("*/", index + 2)
            index = len(source) if end < 0 else end + 1
        elif char == "{":
            depth += 1
        elif char == "}":
            depth -= 1
            if depth == 0:
                return source[start:index + 1]
        index += 1
    raise ValueError(f"unterminated literal object {name}")


def portal_routes(source):
    """Normalize literal Portal prefixes and expand observed dispatch actions."""
    paths = set()
    expression = r'''\bportal_api\s*\)?((?:\s*\+\s*(?:'[^'\\]*'|"[^"\\]*"))+)'''
    for match in re.finditer(expression, source):
        literals = re.findall(r"['\"]([^'\"]*)['\"]", match[1])
        path = "".join(literals).split("?", 1)[0]
        if path and not path.endswith("/"):
            paths.add(path)
    for match in re.finditer(r"\beportal\s*\+\s*['\"]portal/([^'\"?]+)", source):
        paths.add(match[1])
    if re.search(r"\bportal_api\s*\+\s*['\"]page/['\"]", source):
        paths.update("page/" + action for action in re.findall(r"['\"](load(?:Online|Logon|Recharge)Record)['\"]", source))
    for object_name, prefix in (("visitor", "visitor"), ("general", "educae_join")):
        section = object_source(source, object_name)
        if section and re.search(r"portal_api\s*\+\s*['\"]" + prefix + r"/['\"]\s*\+\s*action", section):
            actions = re.findall(r"\.jsonp\s*\(\s*['\"]([^'\"]+)['\"]", section)
            actions += re.findall(r"\.doload\s*\(\s*['\"]([^'\"]+)['\"]", section)
            paths.update(prefix + "/" + action for action in actions)
        paths.update(prefix + "/" + action for action in re.findall(r"\b" + object_name + r"\.jsonp\s*\(\s*['\"]([^'\"]+)['\"]", source))
    if "/eportal/portal/index" in source:
        paths.add("index")
    return sorted(paths)


class Portal:
    def __init__(self, link, terminal="pc"):
        self.link = link
        self.terminal = TERMINALS[terminal]
        self.terminal_name = terminal
        self.program = self.page = ""
        self.version = "4.X"
        self.settings = {}
        self.context = {}
        self.scripts = {}
        self.file_version = ""

    def call(self, path, values=None):
        if path.startswith("/") or ".." in path or "?" in path:
            raise ValueError("expected a relative Portal route")
        callback = "dr" + str(secrets.randbelow(100000) + 1000)
        params = list((values or {}).items())
        params += [("program_index", self.program), ("page_index", self.page),
                   ("callback", callback), ("jsVersion", self.version),
                   ("v", str(secrets.randbelow(10000) + 500)), ("lang", "zh")]
        response = self.link.request("GET", API + path, params)
        if response.status != 200:
            raise ValueError(f"Portal {path}: HTTP {response.status}")
        return decode_jsonp(response.body, callback)

    def configure(self):
        response = self.link.request("GET", BASE + "/a79.htm")
        html = response.text("gb18030")
        self.file_version = assignment(html, "fileVersion")
        self.context = {"wlan_user_ip": assignment(html, "v46ip"),
                        "wlan_user_mac": re.sub(r"[:-]", "", assignment(html, "ss4")),
                        "wlan_vlan_id": assignment(html, "vlanid"),
                        "wlan_user_ipv6": "", "wlan_ac_ip": "", "wlan_ac_name": ""}
        if self.context["wlan_user_ip"] != self.link.source:
            raise ValueError("bootstrap terminal IPv4 differs from selected source")
        query = parse_qs(urlsplit(response.url).query)
        for url_key, field in {"wlanacip": "wlan_ac_ip", "wlanacname": "wlan_ac_name",
                               "UserV6IP": "wlan_user_ipv6", "vlanid": "wlan_vlan_id"}.items():
            if url_key in query:
                self.context[field] = query[url_key][0]
        if "wlanuserip" in query and query["wlanuserip"][0] != self.link.source:
            raise ValueError("redirect terminal IPv4 differs from selected source")
        if "wlanusermac" in query:
            self.context["wlan_user_mac"] = re.sub(r"[:-]", "", query["wlanusermac"][0])
        script = self.link.request("GET", BASE + "/a41.js", {"version": self.file_version})
        self.scripts["a41.js"] = script.text("gb18030")
        values = {key: b64(self.context[key]) for key in ("wlan_user_ip", "wlan_user_ipv6", "wlan_ac_ip")}
        values.update({"wlan_vlan_id": self.context["wlan_vlan_id"],
                       "wlan_user_ssid": query.get("ssid", [""])[0],
                       "wlan_user_areaid": query.get("areaID", [""])[0],
                       "wlan_ap_mac": re.sub(r"[:-]", "", query.get("apmac", ["000000000000"])[0]),
                       "gw_id": re.sub(r"[:-]", "", query.get("gw_id", ["000000000000"])[0])})
        result = self.call("page/loadConfig", values)
        if str(result.get("code")) != "1" or not isinstance(result.get("data"), dict):
            raise ValueError("configuration request was rejected")
        self.settings = result["data"]
        self.program, self.page = str(self.settings["program_index"]), str(self.settings["page_index"])
        version_script = self.link.request("GET", BASE + "/a40.js", {"v": "_" + self.file_version})
        self.scripts["a40.js"] = version_script.text("gb18030")
        self.version = assignment(self.scripts["a40.js"], "jsVersion")
        return self.settings

    def investigate(self, templates=True):
        if not self.settings:
            self.configure()
        references = lambda source: set(re.findall(r"['\"](a\d+\.js)(?:\?[^'\"]*)?['\"]", source))
        pending = references(self.scripts["a41.js"])
        scripts, visited = {}, set()
        while pending:
            name = min(pending)
            pending.remove(name)
            visited.add(name)
            if name not in self.scripts:
                response = self.link.request("GET", BASE + "/" + name, {"v": "_" + self.file_version})
                scripts[name] = response.summary()
                if response.status == 200:
                    self.scripts[name] = response.text("gb18030")
            else:
                scripts[name] = {"status": 200, "loaded": True}
            if name in self.scripts:
                pending.update(references(self.scripts[name]) - visited)
        candidates, directory = {}, {}
        for name, source in self.scripts.items():
            for route in routes(source):
                candidates.setdefault(route, []).append(name)
            for route in portal_routes(source):
                directory.setdefault(route, []).append(name)
        template_results = []
        if templates:
            prefix = f"{BASE}:804/eportal/extern/{self.program}/{self.page}/"
            for kind in TERMINALS:
                suffixes = ("", "_1", "_3", "_2", "_29") if kind == "pc" else ("", "_31", "_33", "_32", "_29")
                for suffix in suffixes:
                    response = self.link.request("GET", prefix + kind + suffix + ".js", {"v": "_" + self.file_version})
                    item = {"terminal": kind, "template": kind + suffix, **response.summary()}
                    if response.status == 200:
                        source = response.text("gb18030")
                        item["route_candidates"] = routes(source)
                        item["calls"] = sorted(set(re.findall(r"\b(?:ee|dd|portal_login|portal_logout|loadConfig|online_list|change_pass|self)\s*\([^\r\n;]*", source)))
                    template_results.append(item)
        return {"bootstrap": {"https_enabled": assignment(self.scripts["a41.js"], "enableHttps"),
                               "https_port": assignment(self.scripts["a41.js"], "enHTTPSPort"), "js_version": self.version},
                "configuration_shape": shape(self.settings),
                "feature_settings": {key: self.settings.get(key) for key in (
                    "login_method", "check_online_method", "enable_new_drcom_srv", "enable_r3", "en_md5",
                    "account_prefix", "ipad_terminal_identity", "no_filter_accandpwd", "un_bind_mac",
                    "user_info", "online_info", "logon_info", "recharge_info", "enable_login_verify")},
                "scripts": scripts, "literal_route_candidates": candidates,
                "normalized_portal_routes": directory, "templates": template_results}


def arguments(description, authenticated=False):
    parser = argparse.ArgumentParser(description=description)
    parser.add_argument("--source", required=True)
    parser.add_argument("--timeout", type=float, default=10)
    parser.add_argument("--output", help="new directory for raw observations; may contain account data")
    parser.add_argument("--terminal", choices=TERMINALS, default="pc")
    if authenticated:
        parser.add_argument("--config", required=True)
        parser.add_argument("--account", required=True, help="configured account alias")
    return parser


def main():
    parser = arguments(__doc__)
    parser.add_argument("--skip-templates", action="store_true")
    args = parser.parse_args()
    portal = Portal(Link(args.source, args.timeout, args.output), args.terminal)
    print(json.dumps(portal.investigate(not args.skip_templates), ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
