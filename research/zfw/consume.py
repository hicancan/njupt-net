"""Observe consumption limits; submit and reread only an explicit amount."""

from decimal import Decimal
import json
import re
from pages import fields
from zfw.session import arguments, opened, user_model


def inspect(session, limit=None):
    page = session.page("service/consumeProtect")
    form = page.form("/Self/service/changeConsumeProtect")
    values = fields(form)
    if not values.get("csrftoken"):
        raise ValueError("consumption form token is absent")
    current = user_model(page).get("installmentFlag")
    result = {"limit": current, "unlimited": Decimal(str(current)) == Decimal("999999")}
    if limit is not None:
        if not re.fullmatch(r"(0|[1-9]\d*)(\.\d{1,3})?", limit):
            raise ValueError("limit must be a nonnegative amount with at most three decimal places")
        values["consumeLimit"] = limit
        response = session.request("POST", form.attrs["action"], values)
        result["submission"] = response.summary()
        after = user_model(session.page("service/consumeProtect")).get("installmentFlag")
        result["after"] = after
        result["matches"] = Decimal(str(after)) == Decimal(limit)
    return result


def main():
    parser = arguments(__doc__)
    parser.add_argument("--set-limit")
    args = parser.parse_args()
    with opened(args) as session:
        print(json.dumps(inspect(session, args.set_limit), indent=2))


if __name__ == "__main__":
    main()
