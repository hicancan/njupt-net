"""Observe online sessions/history, or explicitly take one selected session offline."""

import json
import time
from network import shape
from zfw.session import arguments, opened


def online(session):
    return session.query("dashboard/getOnlineList")


def offline(session, session_id):
    before = online(session)
    if not isinstance(before, list) or not any(str(row.get("sessionId")) == session_id for row in before):
        raise ValueError("selected session is not present in online list")
    reply = session.query("dashboard/tooffline", {"sessionid": session_id})
    deadline = time.monotonic() + 10
    while True:
        after = online(session)
        if not any(str(row.get("sessionId")) == session_id for row in after):
            return {"reply": shape(reply), "session_removed": True}
        if time.monotonic() >= deadline:
            return {"reply": shape(reply), "session_removed": False}
        time.sleep(0.5)


def main():
    parser = arguments(__doc__)
    parser.add_argument("--offline-session", help="explicit sessionId to disconnect")
    args = parser.parse_args()
    with opened(args) as session:
        result = {"online": shape(online(session)), "history": shape(session.query("dashboard/getLoginHistory"))}
        if args.offline_session:
            result["offline"] = offline(session, args.offline_session)
        print(json.dumps(result, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
