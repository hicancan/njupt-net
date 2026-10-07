"""Observe account/profile page contracts and optional account refresh."""

import json
import re
from network import shape
from pages import fields
from zfw.session import arguments, opened, user_model


def inspect(session, refresh=False, submit_profile=False):
    dashboard = session.page("dashboard")
    profile = session.page("setting/personList")
    result = {"dashboard": dashboard.inventory("http://zfw.njupt.edu.cn:8080/Self/dashboard"),
              "user_model_shape": shape(user_model(dashboard)),
              "profile": profile.inventory("http://zfw.njupt.edu.cn:8080/Self/setting/personList")}
    form = profile.form("/Self/setting/updateUserSecurity")
    result["editable_profile_fields"] = [name for name in fields(form) if name != "csrftoken"] if form else []
    if submit_profile:
        values = fields(form)
        if set(values) != {"csrftoken"} or not values["csrftoken"]:
            raise ValueError("profile submission experiment requires the observed token-only form")
        response = session.request("POST", form.attrs["action"], values)
        result["profile_submission"] = response.summary()
        result["profile_update_verified"] = False
    if refresh:
        match = re.search(r'\$\.get\("/Self/dashboard/refreshaccount",\s*\{\s*csrftoken:\s*\x27([^\x27]+)\x27', dashboard.script)
        if not match:
            raise ValueError("account refresh token is absent")
        response = session.request("GET", "dashboard/refreshaccount", {"csrftoken": match[1]})
        result["refresh_response"] = response.summary()
        result["after_user_model_shape"] = shape(user_model(session.page("dashboard")))
    return result


def main():
    parser = arguments(__doc__)
    parser.add_argument("--refresh", action="store_true")
    parser.add_argument("--submit-profile", action="store_true", help="explicitly test the current token-only profile action")
    args = parser.parse_args()
    with opened(args) as session:
        print(json.dumps(inspect(session, args.refresh, args.submit_profile), ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
