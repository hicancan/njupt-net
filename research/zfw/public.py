"""Inspect public notices, help frames, agreement and their same-origin frames."""

import json
from urllib.parse import urljoin
from network import Link, origin
from pages import Page
from zfw.session import Session, arguments, BASE


def inspect(session):
    # Without the /Self Cookie, this deployment URL-rewrites helpinfo/0 as
    # helpinfo/;jsessionid=<id>0. Establish the public-page session first.
    if not any(cookie.name == "JSESSIONID" and cookie.path == "/Self" for cookie in session.link.cookies):
        session.page("login/")
    results = {}
    for kind in ("notice", "help", "agreement"):
        response = session.request("GET", "unlogin/" + kind)
        record = response.summary()
        if response.status == 200:
            page = Page(response.text())
            record["inventory"] = page.inventory(response.url)
            record["frames"] = []
            for node in page.root.walk():
                if node.tag == "iframe" and node.attrs.get("src"):
                    url = urljoin(response.url, node.attrs["src"])
                    if origin(url) == origin(BASE):
                        record["frames"].append(session.link.request("GET", url).summary())
        results[kind] = record
    return results


def main():
    args = arguments(__doc__, authenticated=False).parse_args()
    print(json.dumps(inspect(Session(Link(args.source, args.timeout, args.output))), ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
