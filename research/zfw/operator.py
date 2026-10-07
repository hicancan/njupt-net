"""Inspect operator binding fields; submit only the explicitly selected operator."""

import getpass
import json
from pages import fields
from zfw.session import arguments, opened


def inspect(session, operator=None, account=None, unbind=None):
    if operator and unbind:
        raise ValueError("binding and unbinding are mutually exclusive")
    if unbind and account is not None:
        raise ValueError("an operator account is only used for binding")
    page = session.page("service/operatorId")
    form = page.form("/Self/service/bind-operator")
    values = fields(form)
    required = {"csrftoken", "FLDEXTRA1", "FLDEXTRA2", "FLDEXTRA3", "FLDEXTRA4"}
    if not required.issubset(values) or not values["csrftoken"]:
        raise ValueError("operator form fields or token differ")
    result = {"fields": sorted(values), "njxy_account_set": bool(values["FLDEXTRA1"]),
              "njxy_password_set": bool(values["FLDEXTRA2"]), "cmcc_account_set": bool(values["FLDEXTRA3"]),
              "cmcc_password_set": bool(values["FLDEXTRA4"])}
    selected = unbind or operator
    if selected:
        if selected not in {"njxy", "cmcc"}:
            raise ValueError("unknown operator")
        if unbind:
            account, password = "", ""
        else:
            if not account:
                raise ValueError("binding requires an operator account")
            password = getpass.getpass("Operator password: ")
            if not password:
                raise ValueError("operator password is required")
        account_field, password_field = {"njxy": ("FLDEXTRA1", "FLDEXTRA2"), "cmcc": ("FLDEXTRA3", "FLDEXTRA4")}[selected]
        values.update({account_field: account, password_field: password})
        response = session.request("POST", form.attrs["action"], values)
        result["submission"] = response.summary()
        actual = fields(session.page("service/operatorId").form("/Self/service/bind-operator"))
        result["account_cleared" if unbind else "account_matches"] = actual.get(account_field) == account
        result["password_cleared" if unbind else "password_matches"] = actual.get(password_field) == password
        result["other_operator_unchanged"] = all(actual.get(key) == value for key, value in values.items()
                                                    if key in required - {"csrftoken", account_field, password_field})
    return result


def main():
    parser = arguments(__doc__)
    action = parser.add_mutually_exclusive_group()
    action.add_argument("--bind", choices=("njxy", "cmcc"))
    action.add_argument("--unbind", choices=("njxy", "cmcc"), help="clear selected operator fields and verify readback")
    parser.add_argument("--operator-account")
    args = parser.parse_args()
    if args.operator_account is not None and not args.bind:
        parser.error("--operator-account requires --bind")
    with opened(args) as session:
        print(json.dumps(inspect(session, args.bind, args.operator_account, args.unbind), indent=2))


if __name__ == "__main__":
    main()
