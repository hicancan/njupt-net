"""Observe the server-rendered authentication preference; toggle only explicitly."""

import json
from pages import Page
from zfw.session import arguments, opened


def inspect(session, toggle=False):
    before = session.query("dashboard/refreshMauthType")
    if not isinstance(before, str):
        raise ValueError("mauth response is not an HTML JSON string")
    result = {"before": " ".join(Page(before).root.text().split())}
    if toggle:
        result["operation"] = session.request("GET", "dashboard/oprateMauthAction").summary()
        after = session.query("dashboard/refreshMauthType")
        result["after"] = " ".join(Page(after).root.text().split())
        result["changed"] = result["before"] != result["after"]
    return result


def main():
    parser = arguments(__doc__)
    parser.add_argument("--toggle", action="store_true")
    args = parser.parse_args()
    with opened(args) as session:
        print(json.dumps(inspect(session, args.toggle), ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
