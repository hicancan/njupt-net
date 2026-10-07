"""Obtain a Self bridge response without following its credential-bearing URL."""

import ipaddress
import json
from pathlib import Path
from credentials import credential
from network import Link, shape
from p.portal import Portal, arguments
from p.access import status


def main():
    parser = arguments(__doc__, authenticated=True)
    parser.add_argument("--type", type=int, choices=(0, 1, 2), default=1)
    parser.add_argument("--url-file", help="exclusively create a local bridge URL file; do not publish it")
    args = parser.parse_args()
    portal = Portal(Link(args.source, args.timeout, args.output), args.terminal)
    portal.configure()
    observed = status(portal)
    for row in observed.get("list", []):
        if row.get("online_ip") == args.source:
            portal.context["wlan_user_mac"] = row["online_mac"]
    account, password = credential(args.config, args.account)
    reply = portal.call("self", {"self_type": args.type, "user_account": account, "user_password": password,
        "wlan_user_mac": portal.context["wlan_user_mac"].upper(), "wlan_user_ip": str(int(ipaddress.IPv4Address(args.source)))})
    if args.url_file:
        target = reply.get("self_auth_url")
        if reply.get("result") not in (1, "1", "ok") or not isinstance(target, str) or not target:
            raise ValueError("successful bridge response with URL is required")
        with Path(args.url_file).open("x", encoding="utf-8") as stream:
            stream.write(target + "\n")
    print(json.dumps({"reply": shape(reply), "followed_bridge": False, "url_file_written": bool(args.url_file)}, indent=2))


if __name__ == "__main__":
    main()
