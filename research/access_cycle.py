"""Explicitly test Self offline -> Portal login -> same-source Internet reachability."""

import json
from credentials import credential
from network import Link, shape
from p.portal import Portal, TERMINALS
from p.access import identity_candidates, login, login_parameters, observe_state, selected_session, status
from zfw.session import arguments, opened
from zfw.connections import online, offline


def validate_preconditions(portal, account, password, operator):
    if not account or not password or any(c in account for c in ",@"):
        raise ValueError("base account and password are required before disconnection")
    for key in ("login_method", "check_online_method"):
        if str(portal.settings.get(key)) != "1":
            raise ValueError(f"unsupported {key} before disconnection")
    for key in ("account_prefix", "ipad_terminal_identity", "no_filter_accandpwd"):
        if str(portal.settings.get(key)) not in {"0", "1"}:
            raise ValueError(f"invalid {key} before disconnection")
    if not portal.settings.get("rcn"):
        raise ValueError("missing authentication nonce before disconnection")
    expected = identity_candidates(portal, account, operator)
    before = status(portal)
    selected = selected_session(before, portal.link.source)
    if selected is None or selected.get("user_account") not in expected:
        raise ValueError("current source account/operator differs from requested restoration identity")
    login_parameters(portal, account, password, operator)
    return before, expected


def main():
    parser = arguments(__doc__)
    parser.add_argument("--execute", action="store_true", required=True, help="authorize this connection-changing experiment")
    parser.add_argument("--terminal", choices=TERMINALS, default="pc")
    parser.add_argument("--operator", choices=("campus", "njxy", "cmcc"), required=True)
    parser.add_argument("--restore-on-failure", action="store_true", help="explicitly allow one cleanup login if failure leaves source offline")
    args = parser.parse_args()
    account, password = credential(args.config, args.account)
    link = Link(args.source, args.timeout)
    portal = Portal(link, args.terminal)
    portal.configure()
    before, expected = validate_preconditions(portal, account, password, args.operator)
    result = {"before": shape(before), "verified": False, "recovery": "not_required"}
    disconnected = False
    try:
        with opened(args) as session:
            rows = online(session)
            matches = [row for row in rows if row.get("ip") == args.source]
            if len(matches) != 1:
                raise ValueError("expected one online session matching selected source")
            disconnected = True
            removed = offline(session, str(matches[0]["sessionId"]))
            result["offline"] = removed
            if not removed["session_removed"]:
                raise ValueError("selected session did not disappear; login was not submitted")
        observe_state(portal, False)
        reply = login(portal, account, password, args.operator)
        result["login"] = shape(reply)
        result["login_accepted"] = reply.get("result") in (1, "1", "ok")
        if not result["login_accepted"]:
            raise ValueError("Portal rejected experiment login")
        after = observe_state(portal, True, expected_account=expected)
        result["after"] = shape(after)
        probe = link.request("GET", "http://www.msftconnecttest.com/connecttest.txt", redirects=False)
        result["internet"] = probe.status == 200 and probe.body.strip() == b"Microsoft Connect Test"
        result["verified"] = result["internet"]
        if not result["verified"]:
            raise ValueError("same-source Internet probe failed")
    except (OSError, RuntimeError, ValueError) as exc:
        result["error"] = str(exc)
        if disconnected:
            result["recovery"] = "not_requested"
            try:
                current = status(portal)
                result["current"] = shape(current)
                current_rows = [row for row in current.get("list", []) if row.get("online_ip") == args.source]
                if len(current_rows) == 1 and current_rows[0].get("user_account") in expected:
                    result["recovery"] = "original_identity_online"
                elif not current_rows and current.get("msg") == "获取用户在线信息数据为空！" and args.restore_on_failure:
                    recovery = login(portal, account, password, args.operator)
                    result["recovery_reply"] = shape(recovery)
                    if recovery.get("result") in (1, "1", "ok"):
                        observe_state(portal, True, expected_account=expected)
                        result["recovery"] = "original_identity_restored"
                    else:
                        result["recovery"] = "rejected"
                elif current_rows:
                    result["recovery"] = "different_identity_online_no_submission"
            except (OSError, RuntimeError, ValueError) as cleanup:
                result["recovery"] = "unknown"
                result["recovery_error"] = str(cleanup)
    print(json.dumps(result, ensure_ascii=False, indent=2))
    if not result["verified"]:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
