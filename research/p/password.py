"""Inspect conditional password-change resources; submit only with explicit action."""

import getpass
import json
import secrets
from credentials import credential
from network import Link, shape
from p.portal import Portal, arguments, b64, API


def main():
    parser = arguments(__doc__)
    parser.add_argument("--action", choices=("captcha", "change"), default="captcha")
    parser.add_argument("--config")
    parser.add_argument("--account")
    parser.add_argument("--captcha")
    args = parser.parse_args()
    if args.action == "change" and not (args.config and args.account and args.captcha):
        parser.error("change requires --config, --account and --captcha")
    portal = Portal(Link(args.source, args.timeout, args.output), args.terminal)
    portal.configure()
    if args.action == "captcha":
        response = portal.link.request("GET", API + "captcha", {"randomNum": str(secrets.randbelow(10000) + 500)})
        print(json.dumps(response.summary(), indent=2))
        return
    account, password = credential(args.config, args.account)
    new_password = getpass.getpass("New password: ")
    if not new_password or new_password == password:
        raise ValueError("new password must be nonempty and different")
    reply = portal.call("change_pass", {"user_account": b64(account), "user_old_password": b64(password),
        "user_new_password": b64(new_password), "registerMode": portal.settings["register_mode"], "captcha": args.captcha})
    print(json.dumps({"reply": shape(reply), "password_change_verified": False}, indent=2))


if __name__ == "__main__":
    main()
