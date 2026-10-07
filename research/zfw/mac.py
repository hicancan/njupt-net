"""Inspect actual MAC list responses; unbinding requires an explicit observed MAC."""

import json
import re
from network import shape
from pages import Page
from zfw.session import arguments, opened


def bindings(value, size):
    if not isinstance(value, dict) or type(value.get("total")) is not int or not isinstance(value.get("rows"), list):
        raise ValueError("MAC response requires total and rows")
    total, rows = value["total"], value["rows"]
    if total < len(rows) or total < 0 or len(rows) > size:
        raise ValueError("MAC response has inconsistent row counts")
    for row in rows:
        if (not isinstance(row, list) or len(row) != 5 or type(row[0]) is not int
                or not isinstance(row[1], str) or not re.fullmatch(r"[0-9a-fA-F]{12}", row[1])
                or any(item is not None and not isinstance(item, str) for item in row[2:])):
            raise ValueError("MAC response has an invalid five-column row")
    return total, rows


def binding_present(session, mac, observations):
    size, total, read, page_number, seen = 100, None, 0, 1, set()
    present = False
    while True:
        response = session.request("GET", "service/getMacList", {
            "pageNumber": page_number, "pageSize": size, "searchText": "", "sortName": "2", "sortOrder": "DESC"})
        observations.append(response.summary())
        current_total, rows = bindings(response.json(), size)
        if total is None:
            total = current_total
        if current_total != total or len(rows) != min(size, total - read):
            raise ValueError("MAC pagination changed or is incomplete")
        for row in rows:
            key = row[1].lower()
            if key in seen:
                raise ValueError("MAC pagination repeats a binding")
            seen.add(key)
            present = present or key == mac.lower()
        read += len(rows)
        if read == total:
            return present
        page_number += 1


def inspect(session, page_number=1, page_size=10, unbind=None):
    page = session.page("service/myMac")
    params = {"pageNumber": page_number, "pageSize": page_size, "searchText": "", "sortName": "2", "sortOrder": "DESC"}
    response = session.request("GET", "service/getMacList", params)
    result = {"page": page.inventory("http://zfw.njupt.edu.cn:8080/Self/service/myMac"), "list_response": response.summary(),
              "rendered_columns": [{"index": int(index), "title": title}
                                   for index, title in re.findall(r"field:\s*(\d+)\s*,\s*title:\s*['\"]([^'\"]+)['\"]", page.script)]}
    value = None
    if response.body.strip():
        value = response.json()
        result["list_shape"] = shape(value)
    else:
        result["list_contract"] = "empty response; no JSON list contract established"
    if unbind:
        if not re.fullmatch(r"[0-9a-fA-F]{12}", unbind):
            raise ValueError("unbind MAC must contain twelve hexadecimal digits")
        if value is None:
            raise ValueError("cannot select an observed binding from an empty list response")
        _, rows = bindings(value, page_size)
        if not any(row[1].lower() == unbind.lower() for row in rows):
            raise ValueError("selected MAC is not present in observed binding list")
        token = re.search(r'"&ajaxCsrfToken="\s*\+\s*\x27([^\x27]+)\x27', page.script)
        if not token:
            raise ValueError("unbind operation token is absent")
        reply = session.request("GET", "service/unbindmac", {"mac": unbind, "ajaxCsrfToken": token[1]})
        result["unbind_response"] = reply.summary()
        result["after_responses"] = []
        result["removal_verified"] = False
        try:
            result["removal_verified"] = not binding_present(session, unbind, result["after_responses"])
        except (ValueError, RuntimeError):
            result["removal_contract"] = "complete and consistent JSON list observation is required"
    return result


def main():
    parser = arguments(__doc__)
    parser.add_argument("--page", type=int, default=1)
    parser.add_argument("--size", type=int, choices=(10, 25, 50, 100), default=10)
    parser.add_argument("--unbind")
    args = parser.parse_args()
    if args.page < 1:
        parser.error("page must be positive")
    with opened(args) as session:
        print(json.dumps(inspect(session, args.page, args.size, args.unbind), ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
