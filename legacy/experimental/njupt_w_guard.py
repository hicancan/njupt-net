# /// script
# requires-python = ">=3.12"
# dependencies = [
#     "beautifulsoup4",
#     "requests",
#     "rich",
#     "typer",
# ]
# ///

from __future__ import annotations

import json
import os
import signal
import socket
import subprocess
import time
from concurrent.futures import ThreadPoolExecutor, TimeoutError as FuturesTimeoutError, as_completed
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import requests
import typer
from bs4 import BeautifulSoup
from requests.packages.urllib3.exceptions import InsecureRequestWarning
from rich.console import Console

requests.packages.urllib3.disable_warnings(category=InsecureRequestWarning)

REPO_ROOT = Path(__file__).resolve().parents[2]
DEFAULT_CREDENTIALS_PATH = REPO_ROOT / "credentials.json"
DEFAULT_STATE_DIR = REPO_ROOT / "dist" / "w-guard"
DEFAULT_SELF_BASE_URL = "http://10.10.244.240:8080/Self"
DEFAULT_PORTAL_BASE_URL = "https://10.10.244.11:802/eportal/portal"
DEFAULT_PORTAL_FALLBACK_BASE_URLS = ("https://p.njupt.edu.cn:802/eportal/portal",)
DEFAULT_TARGET = "W"

app = typer.Typer(help="Guard W account broadband binding and portal login.")
console = Console()
SHOULD_STOP = False


@dataclass
class AccountCredentials:
    tag: str
    username: str
    password: str


@dataclass
class BroadbandCredentials:
    label: str
    account: str
    password: str


@dataclass
class BindingSnapshot:
    fldextra1: str = ""
    fldextra2: str = ""
    fldextra3: str = ""
    fldextra4: str = ""
    fldextra5: str = ""
    fldextra6: str = ""

    def matches_mobile(self, broadband: BroadbandCredentials) -> bool:
        return self.fldextra3 == broadband.account and self.fldextra4 == broadband.password

    def matches_mobile_account(self, broadband: BroadbandCredentials) -> bool:
        return self.fldextra3 == broadband.account


def log(message: str, style: str | None = None) -> None:
    timestamp = time.strftime("%Y-%m-%d %H:%M:%S")
    prefix = f"[{timestamp}] "
    if style:
        console.print(prefix + message, style=style)
    else:
        console.print(prefix + message)


def install_signal_handlers() -> None:
    def _handle_signal(signum: int, _frame: Any) -> None:
        global SHOULD_STOP
        SHOULD_STOP = True
        log(f"Received signal {signum}, preparing to stop.", style="yellow")

    for sig in (signal.SIGINT, signal.SIGTERM):
        signal.signal(sig, _handle_signal)


def ensure_state_dir(state_dir: Path) -> None:
    state_dir.mkdir(parents=True, exist_ok=True)


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.write_text(json.dumps(payload, ensure_ascii=False, indent=2), encoding="utf-8")


def load_json(path: Path) -> dict[str, Any]:
    return json.loads(path.read_text(encoding="utf-8"))


def normalize_accounts(data: dict[str, Any]) -> dict[str, dict[str, Any]]:
    if "accounts" in data and isinstance(data["accounts"], dict):
        return data["accounts"]
    if "users" in data and isinstance(data["users"], dict):
        return data["users"]
    raise KeyError("configuration is missing accounts/users")


def resolve_target_key(accounts: dict[str, dict[str, Any]], target: str) -> str | None:
    if target in accounts:
        return target
    lowered = target.lower()
    for key in accounts:
        if key.lower() == lowered:
            return key
    return None


def normalize_broadband(
    data: dict[str, Any],
    target_key: str,
    accounts: dict[str, dict[str, Any]],
) -> BroadbandCredentials:
    if "cmcc" in data and isinstance(data["cmcc"], dict):
        cmcc = data["cmcc"]
        return BroadbandCredentials(
            label="cmcc",
            account=str(cmcc["account"]),
            password=str(cmcc["password"]),
        )

    if "broadbands" in data and isinstance(data["broadbands"], dict):
        bind_name = str(accounts[target_key].get("bind_broadband", "")).strip()
        if not bind_name:
            raise KeyError(f"account {target_key!r} has no bind_broadband configured")
        broadband = data["broadbands"].get(bind_name)
        if not isinstance(broadband, dict):
            raise KeyError(f"broadband {bind_name!r} not found in configuration")
        return BroadbandCredentials(
            label=bind_name,
            account=str(broadband["account"]),
            password=str(broadband["password"]),
        )

    raise KeyError("configuration is missing cmcc/broadbands")


def load_guard_config(credentials_path: Path, target: str) -> tuple[AccountCredentials, dict[str, AccountCredentials], BroadbandCredentials]:
    data = load_json(credentials_path)
    raw_accounts = normalize_accounts(data)
    target_key = resolve_target_key(raw_accounts, target)
    if not target_key:
        raise KeyError(f"target account {target!r} not found")

    accounts: dict[str, AccountCredentials] = {}
    for tag, raw in raw_accounts.items():
        accounts[tag] = AccountCredentials(
            tag=tag,
            username=str(raw["username"]),
            password=str(raw["password"]),
        )

    broadband = normalize_broadband(data, target_key, raw_accounts)
    return accounts[target_key], accounts, broadband


def format_request_exception(exc: requests.RequestException) -> str:
    parts = [type(exc).__name__]
    current: BaseException | None = exc
    visited: set[int] = set()

    while current is not None:
        visited.add(id(current))
        next_exc = getattr(current, "__cause__", None) or getattr(current, "__context__", None)
        if next_exc is None or id(next_exc) in visited:
            break
        label = type(next_exc).__name__
        errno = getattr(next_exc, "errno", None)
        if errno is not None:
            label = f"{label}(errno={errno})"
        parts.append(label)
        current = next_exc

    text = str(exc).strip()
    if text and text.isascii():
        parts.append(text)

    return " -> ".join(parts)


def looks_like_login_page(html: str) -> bool:
    soup = BeautifulSoup(html, "html.parser")
    if soup.find("input", {"name": "checkcode"}):
        return True
    if soup.find("input", {"name": "account"}) and soup.find("input", {"name": "password"}):
        return True
    form = soup.find("form")
    if form:
        action = str(form.get("action", "")).lower()
        if "/self/login" in action or "login/verify" in action:
            return True
    return False


def is_candidate_local_ip(ip: str) -> bool:
    if not ip:
        return False
    if ip.startswith("127.") or ip.startswith("169.254."):
        return False
    return "." in ip


def detect_local_ip_by_udp() -> str | None:
    for host in ("223.5.5.5", "1.1.1.1", "8.8.8.8"):
        try:
            with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
                sock.connect((host, 80))
                candidate = sock.getsockname()[0]
            if is_candidate_local_ip(candidate):
                return candidate
        except OSError:
            continue
    return None


def detect_local_ip_by_powershell() -> str | None:
    powershell = [
        "Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' |",
        "Sort-Object RouteMetric, InterfaceMetric |",
        "ForEach-Object {",
        "  Get-NetIPAddress -AddressFamily IPv4 -InterfaceIndex $_.InterfaceIndex -ErrorAction SilentlyContinue |",
        "  Where-Object {",
        "    $_.AddressState -eq 'Preferred' -and",
        "    $_.IPAddress -notlike '127.*' -and",
        "    $_.IPAddress -notlike '169.254.*' -and",
        "    $_.IPAddress -ne '192.168.137.1'",
        "  } |",
        "  Select-Object -ExpandProperty IPAddress",
        "} | Select-Object -First 1",
    ]
    try:
        result = subprocess.run(
            ["powershell.exe", "-NoProfile", "-Command", " ".join(powershell)],
            capture_output=True,
            check=False,
            text=True,
            timeout=8,
        )
    except (OSError, subprocess.TimeoutExpired):
        return None

    candidate = result.stdout.strip().splitlines()
    if not candidate:
        return None
    ip = candidate[0].strip()
    return ip if is_candidate_local_ip(ip) else None


def detect_local_ip() -> str | None:
    return detect_local_ip_by_udp() or detect_local_ip_by_powershell()


def portal_user_account(username: str, isp: str) -> str:
    suffix_map = {
        "telecom": "@dx",
        "unicom": "@lt",
        "mobile": "@cmcc",
    }
    suffix = suffix_map.get(isp.strip().lower(), "")
    return f",0,{username}{suffix}"


class NJUPTSelfService:
    def __init__(self, credentials: AccountCredentials, base_url: str = DEFAULT_SELF_BASE_URL, timeout: float = 4.0) -> None:
        self.credentials = credentials
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.session = requests.Session()
        self.session.headers.update(
            {
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36",
                "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
                "Upgrade-Insecure-Requests": "1",
            }
        )

    def login(self) -> tuple[bool, str]:
        login_url = f"{self.base_url}/login/?302=LI"
        try:
            response = self.session.get(login_url, timeout=self.timeout)
            response.raise_for_status()
            soup = BeautifulSoup(response.text, "html.parser")
            checkcode_node = soup.find("input", {"name": "checkcode"})
            checkcode = str(checkcode_node.get("value", "")).strip() if checkcode_node else ""
            if not checkcode:
                return False, "missing checkcode token"

            self.session.get(f"{self.base_url}/login/randomCode", params={"t": str(time.time())}, timeout=self.timeout)
            form = {
                "foo": "",
                "bar": "",
                "checkcode": checkcode,
                "account": self.credentials.username,
                "password": self.credentials.password,
                "code": "",
            }
            verify = self.session.post(
                f"{self.base_url}/login/verify",
                data=form,
                headers={
                    "Referer": login_url,
                    "Origin": "http://10.10.244.240:8080",
                    "Content-Type": "application/x-www-form-urlencoded",
                },
                allow_redirects=False,
                timeout=self.timeout,
            )

            location = verify.headers.get("Location", "")
            if "dashboard" not in location:
                return False, f"verify redirected to {location or 'unknown location'}"

            dashboard = self.session.get(f"{self.base_url}/dashboard", timeout=self.timeout)
            if dashboard.status_code != 200:
                return False, f"dashboard readback failed with status={dashboard.status_code}"
            return True, "login ok"
        except requests.RequestException as exc:
            return False, f"self login request failed: {format_request_exception(exc)}"

    def get_binding(self) -> tuple[BindingSnapshot | None, str]:
        try:
            response = self.session.get(f"{self.base_url}/service/operatorId", timeout=self.timeout)
            response.raise_for_status()
        except requests.RequestException as exc:
            return None, f"operatorId request failed: {format_request_exception(exc)}"

        if looks_like_login_page(response.text):
            return None, "operatorId returned login page"

        soup = BeautifulSoup(response.text, "html.parser")
        snapshot = BindingSnapshot(
            fldextra1=self._input_value(soup, "FLDEXTRA1"),
            fldextra2=self._input_value(soup, "FLDEXTRA2"),
            fldextra3=self._input_value(soup, "FLDEXTRA3"),
            fldextra4=self._input_value(soup, "FLDEXTRA4"),
            fldextra5=self._input_value(soup, "FLDEXTRA5"),
            fldextra6=self._input_value(soup, "FLDEXTRA6"),
        )
        return snapshot, "binding read ok"

    def set_binding(self, updates: dict[str, str]) -> tuple[bool, str]:
        try:
            response = self.session.get(f"{self.base_url}/service/operatorId", timeout=self.timeout)
            response.raise_for_status()
        except requests.RequestException as exc:
            return False, f"operatorId request failed: {format_request_exception(exc)}"

        if looks_like_login_page(response.text):
            return False, "operatorId returned login page"

        soup = BeautifulSoup(response.text, "html.parser")
        token_node = soup.find("input", {"name": "csrftoken"})
        token = str(token_node.get("value", "")).strip() if token_node else ""
        if not token:
            return False, "missing csrftoken"

        form = {
            "csrftoken": token,
            "FLDEXTRA1": self._input_value(soup, "FLDEXTRA1"),
            "FLDEXTRA2": self._input_value(soup, "FLDEXTRA2"),
            "FLDEXTRA3": self._input_value(soup, "FLDEXTRA3"),
            "FLDEXTRA4": self._input_value(soup, "FLDEXTRA4"),
            "FLDEXTRA5": self._input_value(soup, "FLDEXTRA5"),
            "FLDEXTRA6": self._input_value(soup, "FLDEXTRA6"),
        }
        for key, value in updates.items():
            if key in form:
                form[key] = value

        try:
            self.session.post(
                f"{self.base_url}/service/bind-operator",
                data=form,
                headers={"Referer": f"{self.base_url}/service/operatorId", "Origin": "http://10.10.244.240:8080"},
                timeout=self.timeout + 5,
            )
        except requests.RequestException as exc:
            return False, f"bind submit failed: {format_request_exception(exc)}"

        time.sleep(1.0)
        snapshot, message = self.get_binding()
        if not snapshot:
            return False, message
        for key, expected in updates.items():
            actual = getattr(snapshot, key.lower(), None)
            if actual != expected:
                return False, f"readback mismatch for {key}: expected={expected!r} actual={actual!r}"
        return True, "binding updated"

    @staticmethod
    def _input_value(soup: BeautifulSoup, name: str) -> str:
        node = soup.find("input", {"name": name})
        if not node:
            return ""
        return str(node.get("value", "")).strip()


class Portal802Client:
    def __init__(
        self,
        base_url: str = DEFAULT_PORTAL_BASE_URL,
        timeout: float = 3.0,
        fallback_base_urls: tuple[str, ...] = DEFAULT_PORTAL_FALLBACK_BASE_URLS,
    ) -> None:
        base_urls: list[str] = []
        for candidate in (base_url, *fallback_base_urls):
            normalized = candidate.rstrip("/")
            if normalized and normalized not in base_urls:
                base_urls.append(normalized)
        self.base_urls = base_urls
        self.timeout = timeout
        self.session = requests.Session()
        self.session.verify = False

    def login(self, credentials: AccountCredentials, ip: str, isp: str) -> tuple[bool, str, dict[str, Any]]:
        attempts: list[str] = []
        for base_url in self.base_urls:
            ok, message, payload = self._login_once(base_url, credentials, ip, isp)
            if payload:
                payload = {**payload, "_base_url": base_url}
            if ok:
                return True, f"portal login ok via {base_url}", payload
            if payload:
                return False, f"{message} via {base_url}", payload
            attempts.append(f"{base_url} -> {message}")
        return False, " | ".join(attempts), {}

    def _login_once(self, base_url: str, credentials: AccountCredentials, ip: str, isp: str) -> tuple[bool, str, dict[str, Any]]:
        try:
            response = self.session.get(
                f"{base_url}/login",
                params={
                    "callback": "dr1003",
                    "login_method": "1",
                    "user_account": portal_user_account(credentials.username, isp),
                    "user_password": credentials.password,
                    "wlan_user_ip": ip,
                },
                timeout=self.timeout,
            )
            response.raise_for_status()
        except requests.RequestException as exc:
            return False, f"portal login request failed: {format_request_exception(exc)}", {}

        payload = parse_jsonp_payload(
            decode_http_body(
                response.content,
                declared_encoding=response.encoding,
                apparent_encoding=response.apparent_encoding,
            )
        )
        if not payload:
            return False, "portal response is not valid JSONP", {}

        if str(payload.get("result", "")).strip() == "1":
            return True, "portal login ok", payload

        ret_code = payload.get("ret_code", "")
        msg = str(payload.get("msg", "")).strip() or "portal login failed"
        return False, f"portal login failed ret_code={ret_code} msg={msg}", payload


def parse_jsonp_payload(raw: str) -> dict[str, Any]:
    body = raw.strip()
    prefix = "dr1003("
    if not body.startswith(prefix):
        return {}
    body = body[len(prefix) :].strip()
    if body.endswith(");"):
        body = body[:-2].strip()
    elif body.endswith(")"):
        body = body[:-1].strip()
    if not body:
        return {}
    try:
        return json.loads(body)
    except json.JSONDecodeError:
        return {}


def decode_http_body(
    content: bytes,
    declared_encoding: str | None = None,
    apparent_encoding: str | None = None,
) -> str:
    encodings: list[str] = []

    for encoding in (declared_encoding, apparent_encoding):
        normalized = (encoding or "").strip()
        if not normalized:
            continue
        if normalized.lower() in {"iso-8859-1", "latin-1"}:
            continue
        if normalized not in encodings:
            encodings.append(normalized)

    for fallback in ("utf-8", "gb18030", "gbk"):
        if fallback not in encodings:
            encodings.append(fallback)

    for encoding in encodings:
        try:
            return content.decode(encoding)
        except UnicodeDecodeError:
            continue

    return content.decode("utf-8", errors="replace")


def _connectivity_probe(url: str, validator: Any, timeout: float) -> tuple[bool, str]:
    try:
        response = requests.get(url, timeout=timeout, allow_redirects=False)
        if validator(response):
            return True, f"connectivity ok via {url}"
        return False, f"unexpected status={response.status_code}"
    except requests.RequestException as exc:
        return False, format_request_exception(exc)


def check_connectivity(timeout: float = 1.2) -> tuple[bool, str]:
    probes = [
        ("http://www.msftconnecttest.com/connecttest.txt", lambda response: response.status_code == 200 and "Microsoft Connect Test" in response.text),
        ("http://connectivitycheck.gstatic.com/generate_204", lambda response: response.status_code == 204),
        ("http://captive.apple.com/hotspot-detect.html", lambda response: response.status_code == 200 and "Success" in response.text),
    ]
    failures: list[str] = []

    with ThreadPoolExecutor(max_workers=len(probes)) as executor:
        future_map = {
            executor.submit(_connectivity_probe, url, validator, timeout): url
            for url, validator in probes
        }
        try:
            for future in as_completed(future_map, timeout=timeout + 0.8):
                ok, message = future.result()
                if ok:
                    return True, message
                failures.append(f"{future_map[future]} -> {message}")
        except FuturesTimeoutError:
            failures.append(f"probe window exceeded {timeout + 0.8:.1f}s")

    if failures:
        return False, f"all connectivity probes failed within {timeout:.1f}s"
    return False, f"all connectivity probes failed within {timeout:.1f}s"


def ensure_target_binding(
    target: AccountCredentials,
    accounts: dict[str, AccountCredentials],
    broadband: BroadbandCredentials,
    self_base_url: str,
    self_timeout: float,
) -> tuple[bool, str]:
    target_client = NJUPTSelfService(target, base_url=self_base_url, timeout=self_timeout)
    ok, message = target_client.login()
    if not ok:
        return False, f"target self login failed: {message}"

    target_binding, message = target_client.get_binding()
    if not target_binding:
        return False, f"target binding read failed: {message}"
    if target_binding.matches_mobile_account(broadband):
        if target_binding.fldextra4 and target_binding.fldextra4 != broadband.password:
            log(
                f"Target {target.tag} already owns mobile account {broadband.account}, "
                f"but stored password readback differs ({target_binding.fldextra4!r} != configured).",
                style="yellow",
            )
        return True, "target binding already correct"

    holder: AccountCredentials | None = None
    for tag, credentials in accounts.items():
        if tag == target.tag:
            continue
        client = NJUPTSelfService(credentials, base_url=self_base_url, timeout=self_timeout)
        ok, message = client.login()
        if not ok:
            log(f"Skip holder probe for {tag}: {message}", style="yellow")
            continue
        binding, message = client.get_binding()
        if not binding:
            log(f"Skip binding read for {tag}: {message}", style="yellow")
            continue
        if binding.fldextra3 == broadband.account:
            holder = credentials
            clear_ok, clear_message = client.set_binding({"FLDEXTRA3": "", "FLDEXTRA4": ""})
            if not clear_ok:
                return False, f"failed to clear holder {tag}: {clear_message}"
            log(f"Released mobile binding from {tag}.", style="yellow")
            break

    bind_ok, bind_message = target_client.set_binding(
        {
            "FLDEXTRA3": broadband.account,
            "FLDEXTRA4": broadband.password,
        }
    )
    if not bind_ok:
        snapshot, snapshot_message = target_client.get_binding()
        if snapshot and snapshot.matches_mobile_account(broadband):
            if snapshot.fldextra4 and snapshot.fldextra4 != broadband.password:
                log(
                    f"Target {target.tag} now owns mobile account {broadband.account}, "
                    f"but password readback remains {snapshot.fldextra4!r}.",
                    style="yellow",
                )
            return True, f"target mobile account bound with password mismatch warning: {bind_message}"
        return False, f"failed to bind target {target.tag}: {bind_message}; readback={snapshot_message}"

    if holder:
        return True, f"binding moved from {holder.tag} to {target.tag}"
    return True, f"binding attached to {target.tag}"


def run_guard_cycle(
    target: AccountCredentials,
    accounts: dict[str, AccountCredentials],
    broadband: BroadbandCredentials,
    isp: str,
    self_base_url: str,
    portal_base_url: str,
    self_timeout: float,
    portal_timeout: float,
    force_binding_check: bool,
    cycle_label: str = "",
) -> dict[str, Any]:
    label_prefix = f"{cycle_label} " if cycle_label else ""
    cycle: dict[str, Any] = {
        "target": target.tag,
        "username": target.username,
        "broadband_label": broadband.label,
        "timestamp": time.strftime("%Y-%m-%d %H:%M:%S"),
        "force_binding_check": force_binding_check,
    }

    binding_ok = True
    binding_message = "binding check skipped"
    if force_binding_check:
        binding_ok, binding_message = ensure_target_binding(
            target=target,
            accounts=accounts,
            broadband=broadband,
            self_base_url=self_base_url,
            self_timeout=self_timeout,
        )
    cycle["binding_ok"] = binding_ok
    cycle["binding_message"] = binding_message
    if not binding_ok:
        cycle["portal_login_ok"] = False
        cycle["portal_login_message"] = "portal login skipped because binding repair failed"
        return cycle

    internet_ok, internet_message = check_connectivity()
    cycle["initial_internet_ok"] = internet_ok
    cycle["initial_internet_message"] = internet_message
    cycle["internet_ok"] = internet_ok
    cycle["internet_message"] = internet_message
    if internet_ok:
        cycle["portal_login_ok"] = True
        cycle["portal_login_message"] = "portal login not needed"
        cycle["recovery_step"] = "healthy"
        return cycle

    log(f"{label_prefix}offline detected: {internet_message}", style="yellow")
    ip = detect_local_ip()
    cycle["local_ip"] = ip or ""
    if not ip:
        cycle["portal_login_ok"] = False
        cycle["portal_login_message"] = "unable to detect local IPv4"
        cycle["recovery_step"] = "no-ip"
        return cycle

    log(f"{label_prefix}trying portal login with ip={ip}", style="yellow")
    portal_client = Portal802Client(base_url=portal_base_url, timeout=portal_timeout)
    first_portal_ok, first_portal_message, first_payload = portal_client.login(target, ip=ip, isp=isp)
    cycle["first_portal_login_ok"] = first_portal_ok
    cycle["first_portal_login_message"] = first_portal_message
    cycle["first_portal_payload"] = first_payload
    if first_portal_ok:
        post_first_internet_ok, post_first_internet_message = check_connectivity()
        cycle["post_first_login_internet_ok"] = post_first_internet_ok
        cycle["post_first_login_internet_message"] = post_first_internet_message
        cycle["internet_ok"] = post_first_internet_ok
        cycle["internet_message"] = post_first_internet_message
        if post_first_internet_ok:
            cycle["portal_login_ok"] = True
            cycle["portal_login_message"] = first_portal_message
            cycle["portal_payload"] = first_payload
            cycle["recovery_step"] = "portal-login"
            return cycle

        cycle["first_portal_login_message"] = (
            f"{first_portal_message}; post-login connectivity failed: {post_first_internet_message}"
        )

    log(
        f"{label_prefix}portal login did not restore connectivity: {cycle['first_portal_login_message']}; checking binding",
        style="yellow",
    )
    binding_repair_ok, binding_repair_message = ensure_target_binding(
        target=target,
        accounts=accounts,
        broadband=broadband,
        self_base_url=self_base_url,
        self_timeout=self_timeout,
    )
    cycle["binding_repair_ok"] = binding_repair_ok
    cycle["binding_repair_message"] = binding_repair_message
    if not binding_repair_ok:
        cycle["portal_login_ok"] = False
        cycle["portal_login_message"] = (
            f"portal login failed before binding repair: {first_portal_message}"
        )
        cycle["recovery_step"] = "binding-repair-failed"
        return cycle

    retry_ip = detect_local_ip() or ip
    cycle["retry_local_ip"] = retry_ip
    log(f"{label_prefix}binding repair result: {binding_repair_message}", style="yellow")
    log(f"{label_prefix}retrying portal login with ip={retry_ip}", style="yellow")
    second_portal_ok, second_portal_message, second_payload = portal_client.login(target, ip=retry_ip, isp=isp)
    cycle["second_portal_login_ok"] = second_portal_ok
    cycle["second_portal_login_message"] = second_portal_message
    cycle["second_portal_payload"] = second_payload
    post_second_internet_ok, post_second_internet_message = check_connectivity()
    cycle["post_second_login_internet_ok"] = post_second_internet_ok
    cycle["post_second_login_internet_message"] = post_second_internet_message
    cycle["internet_ok"] = post_second_internet_ok
    cycle["internet_message"] = post_second_internet_message
    cycle["portal_login_ok"] = post_second_internet_ok
    cycle["portal_login_message"] = (
        second_portal_message
        if post_second_internet_ok
        else f"{second_portal_message}; post-login connectivity failed: {post_second_internet_message}"
    )
    cycle["portal_payload"] = second_payload
    cycle["recovery_step"] = "binding-repair-then-portal-login"
    return cycle


@app.command()
def once(
    target: str = typer.Option(DEFAULT_TARGET, help="Target account tag to guard."),
    credentials: Path = typer.Option(DEFAULT_CREDENTIALS_PATH, help="Path to credentials.json."),
    isp: str = typer.Option("mobile", help="Portal ISP route: telecom|unicom|mobile."),
    self_base_url: str = typer.Option(DEFAULT_SELF_BASE_URL, help="Self service base URL."),
    portal_base_url: str = typer.Option(DEFAULT_PORTAL_BASE_URL, help="Portal 802 base URL."),
    self_timeout: float = typer.Option(4.0, help="Self request timeout seconds."),
    portal_timeout: float = typer.Option(3.0, help="Portal request timeout seconds."),
) -> None:
    target_account, accounts, broadband = load_guard_config(credentials, target)
    cycle = run_guard_cycle(
        target=target_account,
        accounts=accounts,
        broadband=broadband,
        isp=isp,
        self_base_url=self_base_url,
        portal_base_url=portal_base_url,
        self_timeout=self_timeout,
        portal_timeout=portal_timeout,
        force_binding_check=True,
        cycle_label="cycle=once",
    )
    console.print_json(json.dumps(cycle, ensure_ascii=False))
    if not cycle.get("portal_login_ok", False):
        raise typer.Exit(code=1)


@app.command()
def daemon(
    target: str = typer.Option(DEFAULT_TARGET, help="Target account tag to guard."),
    credentials: Path = typer.Option(DEFAULT_CREDENTIALS_PATH, help="Path to credentials.json."),
    interval_seconds: int = typer.Option(3, min=3, help="Connectivity probe loop interval in seconds."),
    binding_check_interval: int = typer.Option(180, min=30, help="How often to perform a full binding repair check."),
    isp: str = typer.Option("mobile", help="Portal ISP route: telecom|unicom|mobile."),
    self_base_url: str = typer.Option(DEFAULT_SELF_BASE_URL, help="Self service base URL."),
    portal_base_url: str = typer.Option(DEFAULT_PORTAL_BASE_URL, help="Portal 802 base URL."),
    self_timeout: float = typer.Option(4.0, help="Self request timeout seconds."),
    portal_timeout: float = typer.Option(3.0, help="Portal request timeout seconds."),
    state_dir: Path = typer.Option(DEFAULT_STATE_DIR, help="Directory used for pid/status files."),
) -> None:
    install_signal_handlers()
    ensure_state_dir(state_dir)

    pid_file = state_dir / "worker.pid"
    status_file = state_dir / "status.json"
    pid_file.write_text(str(os.getpid()), encoding="utf-8")

    try:
        target_account, accounts, broadband = load_guard_config(credentials, target)
    except Exception as exc:  # noqa: BLE001
        log(f"Failed to load configuration: {exc}", style="red")
        raise typer.Exit(code=1) from exc

    log(f"Starting guard for target={target_account.tag} user={target_account.username} broadband={broadband.label}.", style="cyan")
    log(f"State directory: {state_dir}", style="cyan")

    last_binding_check = 0.0
    cycle_index = 0
    try:
        while not SHOULD_STOP:
            cycle_started_at = time.monotonic()
            cycle_index += 1
            now = time.time()
            force_binding_check = (now - last_binding_check) >= binding_check_interval
            if cycle_index == 1:
                force_binding_check = True

            cycle = run_guard_cycle(
                target=target_account,
                accounts=accounts,
                broadband=broadband,
                isp=isp,
                self_base_url=self_base_url,
                portal_base_url=portal_base_url,
                self_timeout=self_timeout,
                portal_timeout=portal_timeout,
                force_binding_check=force_binding_check,
                cycle_label=f"cycle={cycle_index}",
            )
            cycle["cycle_index"] = cycle_index
            cycle["probe_interval_seconds"] = interval_seconds
            cycle["binding_check_interval"] = binding_check_interval
            cycle["cycle_elapsed_seconds"] = round(time.monotonic() - cycle_started_at, 2)
            write_json(status_file, cycle)

            if force_binding_check:
                last_binding_check = now

            binding_ok = cycle.get("binding_ok", False)
            portal_ok = cycle.get("portal_login_ok", False)
            internet_ok = cycle.get("internet_ok", False)
            summary = (
                f"cycle={cycle_index} binding_ok={binding_ok} "
                f"internet_ok={internet_ok} portal_ok={portal_ok} "
                f"elapsed={cycle.get('cycle_elapsed_seconds', 0)}s "
                f"step={cycle.get('recovery_step', '')} "
                f"binding_msg={cycle.get('binding_message', '')} "
                f"portal_msg={cycle.get('portal_login_message', '')}"
            )
            log(summary, style="green" if portal_ok else "yellow")

            remaining = max(0.0, interval_seconds - (time.monotonic() - cycle_started_at))
            while remaining > 0 and not SHOULD_STOP:
                nap = min(0.5, remaining)
                time.sleep(nap)
                remaining -= nap
    finally:
        if pid_file.exists():
            pid_file.unlink()
        log("Guard stopped.", style="yellow")


@app.command()
def status(
    state_dir: Path = typer.Option(DEFAULT_STATE_DIR, help="Directory used for pid/status files."),
) -> None:
    status_file = state_dir / "status.json"
    if not status_file.exists():
        log("No local guard status file found.", style="yellow")
        raise typer.Exit(code=1)
    payload = load_json(status_file)
    console.print_json(json.dumps(payload, ensure_ascii=False))


if __name__ == "__main__":
    app()
