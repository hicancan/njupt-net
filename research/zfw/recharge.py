"""Inspect whether the current account is actually offered a payment flow."""

import json
from zfw.session import arguments, opened
from pages import Page


def inspect(session):
    response = session.request("GET", "service/userRecharge")
    page = Page(response.text())
    inventory = page.inventory(response.url)
    return {"response": response.summary(), "inventory": inventory,
            "payment_form_present": bool(inventory["forms"]),
            "payment_contract_verified": False}


def main():
    args = arguments(__doc__).parse_args()
    with opened(args) as session:
        print(json.dumps(inspect(session), ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
