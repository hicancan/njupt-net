"""Query the deployed Portal API directly, independently of browser resources."""

import argparse
import json
import secrets

from network import Link, shape
from p.access import selected_session
from p.portal import decode_jsonp


def query(link, port, route):
    if port not in (801, 802, 803, 804):
        raise ValueError("Portal port must be 801, 802, 803 or 804")
    if route not in ("online_list", "page/loadConfig"):
        raise ValueError("expected a Portal status or configuration query")
    scheme = "http" if port in (801, 803) else "https"
    callback = "dr" + str(secrets.randbelow(100000) + 1000)
    response = link.request("GET", f"{scheme}://p.njupt.edu.cn:{port}/eportal/portal/{route}",
                            {"callback": callback})
    if response.status != 200:
        raise ValueError(f"Portal query returned HTTP {response.status}")
    value = decode_jsonp(response.body, callback)
    if route == "online_list":
        selected_session(value, link.source)
    elif str(value.get("code")) != "1" or not isinstance(value.get("data"), dict):
        raise ValueError("Portal configuration query was rejected")
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--source", required=True)
    parser.add_argument("--port", type=int, choices=(801, 802, 803, 804), default=804)
    parser.add_argument("--route", choices=("online_list", "page/loadConfig"), default="online_list")
    parser.add_argument("--timeout", type=float, default=10)
    parser.add_argument("--output")
    args = parser.parse_args()
    link = Link(args.source, args.timeout, args.output)
    value = query(link, args.port, args.route)
    result = {"port": args.port, "route": args.route, "response_shape": shape(value)}
    if args.route == "online_list":
        result["selected_source_online"] = selected_session(value, link.source) is not None
    print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
