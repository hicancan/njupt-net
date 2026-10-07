"""Inspect operator binding fields; submit only the explicitly selected operator."""

import getpass
import json
from pages import fields
from zfw.session import arguments, opened


def inspect(session, operator=None, account=None):
    page = session.page("service/operatorId")
    form = page.form("/Self/service/bind-operator")
    values = fields(form)
    required = {"csrftoken", "FLDEXTRA1", "FLDEXTRA2", "FLDEXTRA3", "FLDEXTRA4"}
    if not required.issubset(values) or not values["csrftoken"]:
        raise ValueError("operator form fields or token differ")
    result = {"fields": sorted(values), "njxy_account_set": bool(values["FLDEXTRA1"]),
              "njxy_password_set": bool(values["FLDEXTRA2"]), "cmcc_account_set": bool(values["FLDEXTRA3"]),
              "cmcc_password_set": bool(values["FLDEXTRA4"])}
    if operator:
        if not account:
            raise ValueError("binding requires an operator account")
        password = getpass.getpass("Operator password: ")
        if not password:
            raise ValueError("operator password is required")
        account_field, password_field = {"njxy": ("FLDEXTRA1", "FLDEXTRA2"), "cmcc": ("FLDEXTRA3", "FLDEXTRA4")}[operator]
        values.update({account_field: account, password_field: password})
        response = session.request("POST", form.attrs["action"], values)
        result["submission"] = response.summary()
        actual = fields(session.page("service/operatorId").form("/Self/service/bind-operator"))
        result["account_matches"] = actual.get(account_field) == account
        result["other_operator_unchanged"] = all(actual.get(key) == value for key, value in values.items()
                                                    if key in required - {"csrftoken", account_field, password_field})
    return result


def main():
    parser = arguments(__doc__)
    parser.add_argument("--bind", choices=("njxy", "cmcc"))
    parser.add_argument("--operator-account")
    args = parser.parse_args()
    with opened(args) as session:
        print(json.dumps(inspect(session, args.bind, args.operator_account), indent=2))


if __name__ == "__main__":
    main()
