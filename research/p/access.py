"""Observe Portal state, or explicitly submit one login or logout experiment."""

import json
import time
import ipaddress
from credentials import credential
from network import Link, shape
from p.portal import Portal, arguments, b64


def selected_session(value, source):
    if str(value.get("result")) == "0":
        if value.get("msg") == "获取用户在线信息数据为空！" and "list" not in value and "total" not in value:
            return None
        raise ValueError("online query was rejected; offline state is not established")
    rows = value.get("list")
    if str(value.get("result")) != "1" or not isinstance(rows, list) or int(value.get("total", -1)) != len(rows):
        raise ValueError("online response has an unconfirmed list contract")
    matches = []
    for row in rows:
        if not isinstance(row, dict):
            raise ValueError("online list row is not an object")
        ipaddress.IPv4Address(row.get("online_ip", ""))
        if row["online_ip"] == source:
            matches.append(row)
    if len(matches) > 1 or matches and not matches[0].get("user_account"):
        raise ValueError("online source identity is ambiguous or absent")
    return matches[0] if matches else None


def identity_candidates(portal, account, operator):
    suffix = {"campus": "", "njxy": "@njxy", "cmcc": "@cmcc"}[operator]
    mobile = portal.terminal == 2 or portal.terminal >= 3 and str(portal.settings["ipad_terminal_identity"]) == "1"
    return {account + suffix, f",{1 if mobile else 0},{account}{suffix}"}


def status(portal):
    context = portal.context
    result = portal.call("online_list", {"user_account": "", "user_password": "",
        "wlan_user_mac": context["wlan_user_mac"].upper(),
        "wlan_user_ip": b64(context["wlan_user_ip"]), "wlan_user_ipv6": b64(context["wlan_user_ipv6"])})
    for row in result.get("list", []):
        if row.get("online_ip") == portal.link.source and isinstance(row.get("online_mac"), str):
            portal.context["wlan_user_mac"] = row["online_mac"].replace(":", "").replace("-", "")
    return result


def login_parameters(portal, account, password, operator):
    if str(portal.settings.get("login_method")) != "1":
        raise ValueError("this experiment requires deployed login_method=1")
    if not account or not password or any(c in account for c in ",@"):
        raise ValueError("base account and password are required")
    if any(str(portal.settings.get(key)) != "0" for key in ("enable_r3", "en_md5")):
        raise ValueError("this experiment requires deployed unencrypted method1 credentials")
    for key in ("account_prefix", "ipad_terminal_identity", "no_filter_accandpwd"):
        if str(portal.settings.get(key)) not in {"0", "1"}:
            raise ValueError(f"unexpected authentication setting {key}")
    suffix = {"campus": "", "njxy": "@njxy", "cmcc": "@cmcc"}[operator]
    mobile = portal.terminal == 2 or portal.terminal >= 3 and str(portal.settings["ipad_terminal_identity"]) == "1"
    mac_type = "1" if mobile else "0"
    wire_account = account + suffix
    if str(portal.settings["account_prefix"]) == "1":
        wire_account = f",{mac_type},{wire_account}"
    encoded = str(portal.settings["no_filter_accandpwd"])
    if encoded == "1":
        wire_account, password = b64(wire_account), b64(password)
    values = dict(portal.context)
    values.update({"login_method": "1", "is_base64encode": encoded, "user_account": wire_account,
        "user_password": password, "authex_enable": "", "terminal_type": str(portal.terminal),
        "lang": "zh-cn", "user_agent": "njupt-net-research/1.0", "enable_r3": "0", "mac_type": mac_type,
        "rcn": portal.settings["rcn"], "operate": "portal_login", "business_type": "1"})
    return values


def login(portal, account, password, operator):
    return portal.call("login", login_parameters(portal, account, password, operator))


def logout(portal):
    values = dict(portal.context)
    values.update({"login_method": "1", "user_account": "drcom", "user_password": "123",
                   "ac_logout": portal.settings["ac_logout"], "register_mode": portal.settings["register_mode"]})
    return portal.call("logout", values)


def observe_state(portal, expected_online, seconds=10, expected_account=None):
    deadline = time.monotonic() + seconds
    while True:
        value = status(portal)
        selected = selected_session(value, portal.link.source)
        online = selected is not None
        account_matches = not expected_account or online and selected["user_account"] in expected_account
        if online == expected_online and (str(value.get("result")) == "1" or
                value.get("msg") == "获取用户在线信息数据为空！") and account_matches:
            return value
        if time.monotonic() >= deadline:
            raise ValueError("submitted operation did not reach expected observed state")
        time.sleep(0.5)


def main():
    parser = arguments(__doc__)
    parser.add_argument("--action", choices=("status", "login", "logout", "error"), default="status")
    parser.add_argument("--config")
    parser.add_argument("--account")
    parser.add_argument("--operator", choices=("campus", "njxy", "cmcc"), default="campus")
    args = parser.parse_args()
    if args.action == "login" and not (args.config and args.account):
        parser.error("login requires --config and --account")
    portal = Portal(Link(args.source, args.timeout, args.output), args.terminal)
    portal.configure()
    before = status(portal)
    result = {"before": shape(before)}
    if args.action == "login":
        if any(row.get("online_ip") == args.source for row in before.get("list", [])):
            raise ValueError("terminal is already online; authentication was not submitted")
        account, password = credential(args.config, args.account)
        reply = login(portal, account, password, args.operator)
        result["reply"] = shape(reply)
        if reply.get("result") in (1, "1", "ok"):
            result["after"] = shape(observe_state(portal, True, expected_account=identity_candidates(portal, account, args.operator)))
            result["identity_verified"] = True
    elif args.action == "logout":
        result["reply"] = shape(logout(portal))
        result["after"] = shape(status(portal))
    elif args.action == "error":
        result["reply"] = shape(portal.call("err_code", {key: portal.context[key]
            for key in ("wlan_user_ip", "wlan_user_ipv6", "wlan_user_mac")}))
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
