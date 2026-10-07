"""Read one explicitly selected account from the product configuration."""

import json
from pathlib import Path


def credential(config, alias):
    data = json.loads(Path(config).read_text(encoding="utf-8-sig"))
    value = data.get("accounts", {}).get(alias)
    if not isinstance(value, dict) or not value.get("account") or not value.get("password"):
        raise ValueError("selected account requires an account and password")
    return value["account"], value["password"]
