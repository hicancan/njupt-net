"""Inspect bill filters, raw table contracts and session-dependent XLS exports."""

from datetime import datetime, timedelta, timezone
import json
from pathlib import Path
from network import shape
from zfw.session import arguments, opened

TABLES = {
    "online": ("userOnlineLog", "getUserOnlineLog", "exportUserOnlineLog", "userOnlineList"),
    "monthly": ("monthPay", "getMonthPay", "exportMonthPay", "monthPay"),
    "operations": ("operatorLog", "getOperatorLog", "exportOperatorLog", "operatorLog"),
}


def parameters(page, kind, start=None, end=None, year=None, page_number=1, page_size=10, sort=None, order="DESC"):
    allowed_sizes = (10, 20, 50, 100) if kind == "online" else (10, 25, 50, 100)
    if page_number < 1 or page_size not in allowed_sizes or order not in {"ASC", "DESC"}:
        raise ValueError("invalid bill pagination or order")
    sort = sort if sort is not None else "loginTime" if kind == "online" else "0"
    allowed_sort = {"loginTime"} if kind == "online" else set("01234567") if kind == "monthly" else set("0124")
    if sort not in allowed_sort:
        raise ValueError("sort field is not offered by this table")
    values = {"pageNumber": page_number, "pageSize": page_size, "searchText": "", "sortName": sort, "sortOrder": order}
    if kind == "monthly":
        if start or end:
            raise ValueError("monthly bills take year, not date range")
        selector = page.root.find("select", id="year")
        if selector is None:
            raise ValueError("monthly year selector is absent")
        options = [node for node in selector.walk() if node.tag == "option"]
        offered = [node.attrs.get("value", node.text()) for node in options]
        selected = next((node for node in options if "selected" in node.attrs), options[0] if options else None)
        requested = str(year) if year else selected.attrs.get("value", selected.text()) if selected else ""
        if requested not in offered:
            raise ValueError("requested year is not offered by current page")
        values["year"] = requested
    else:
        if year:
            raise ValueError("this bill takes date range, not year")
        today = datetime.now(timezone(timedelta(hours=8))).date()
        if start is None and end is None:
            start = end = today.isoformat()
        if start is None or end is None:
            raise ValueError("start and end are required together")
        first = datetime.strptime(start, "%Y-%m-%d").date()
        last = datetime.strptime(end, "%Y-%m-%d").date()
        if first.isoformat() != start or last.isoformat() != end or last < first or (last - first).days > 60 or last > today:
            raise ValueError("date range must end by today and span at most 60 days")
        values.update({"startTime": start, "endTime": end})
    return values


def inspect(session, kind, export=None, export_file=None, **query):
    page_route, query_route, export_route, table_id = TABLES[kind]
    page = session.page("bill/" + page_route)
    if page.root.find("table", id=table_id) is None:
        raise ValueError("expected bill table is absent")
    params = parameters(page, kind, **query)
    value = session.query("bill/" + query_route, params)
    result = {"kind": kind, "parameters": params, "response_shape": shape(value)}
    if export:
        values = {"type": "1" if export == "page" else "2"}
        if export == "all":
            for key in ("startTime", "endTime", "year"):
                if key in params:
                    values[key] = params[key]
        if kind == "operations":
            for key in ("startTime", "endTime", "sortName", "sortOrder"):
                values[key] = params[key]
        response = session.request("GET", "bill/" + export_route, values)
        disposition = response.headers.get("Content-Disposition", "")
        valid = response.status == 200 and disposition.startswith("attachment; filename=") and disposition.endswith(".xls") and response.body.startswith(bytes.fromhex("d0cf11e0a1b11ae1"))
        result["export"] = {**response.summary(), "xls_container": valid}
        if not valid:
            raise ValueError("export did not return the expected XLS attachment")
        if export_file:
            with Path(export_file).open("xb") as stream:
                stream.write(response.body)
    return result


def main():
    parser = arguments(__doc__)
    parser.add_argument("--kind", choices=TABLES, required=True)
    parser.add_argument("--start")
    parser.add_argument("--end")
    parser.add_argument("--year", type=int)
    parser.add_argument("--page", type=int, default=1)
    parser.add_argument("--size", type=int, default=10)
    parser.add_argument("--sort")
    parser.add_argument("--order", choices=("ASC", "DESC"), default="DESC")
    parser.add_argument("--export", choices=("page", "all"))
    parser.add_argument("--export-file")
    args = parser.parse_args()
    if args.export_file and not args.export:
        parser.error("--export-file requires --export")
    with opened(args) as session:
        result = inspect(session, args.kind, args.export, args.export_file, start=args.start, end=args.end,
                         year=args.year, page_number=args.page, page_size=args.size, sort=args.sort, order=args.order)
        print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
