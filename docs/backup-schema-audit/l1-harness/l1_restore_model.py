"""L1 pure synthetic restore model for HRT cold-backup design.

This is NOT production code. It is a standalone model used to exercise the
T1-T15 restore semantics with synthetic data. It does not import or call any
production module, browser storage, Worker, database, or network API.
"""

from __future__ import annotations

import copy
import math
from dataclasses import dataclass
from typing import Any

ROUTES = {"sublingual", "injection", "patchApply", "patchRemove", "gel", "oral"}
ESTERS = {
    "E2", "EB", "EV", "EC", "EN", "EU", "CPA", "SPIRO", "BICA", "MPA",
    "LEUP", "TRIP", "RLX", "T", "TC", "TE", "TU",
}
PACK_KINDS = {"vial", "pill", "pump", "sachet", "patch", "bulk"}
LOCAL_CATEGORIES = ("quickDoses", "supplies", "plan")
CREDENTIAL_KEYS = {
    "auth_token", "auth_user", "weak_password", "login_device_passes",
    "enc_key", "totp_secret", "totp_backup_codes", "passkey", "passkeys",
    "session", "sessions", "private_key", "privateKey", "passphrase",
}
PREFERENCE_KEYS = {
    "hrt-lang", "app-theme", "hrt-supply-lead-days", "app-show-prediction",
    "app-hide-site-label", "hrt-mode", "app-pixel-cats", "app-pixel-cat-style",
}


class RestoreError(Exception):
    pass


class UnsupportedVersion(RestoreError):
    pass


class InvalidData(RestoreError):
    pass


class Conflict(RestoreError):
    pass


def _num(value: Any, name: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise InvalidData(f"{name} must be a finite number")
    value = float(value)
    if not math.isfinite(value):
        raise InvalidData(f"{name} must be finite")
    return value


def _id(value: Any, name: str) -> str:
    if not isinstance(value, str) or not value:
        raise InvalidData(f"{name} must be a non-empty string")
    return value


def _no_credentials(value: Any, path: str = "backup") -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            if key in CREDENTIAL_KEYS:
                raise InvalidData(f"credential field is not allowed: {path}.{key}")
            _no_credentials(child, f"{path}.{key}")
    elif isinstance(value, list):
        for index, child in enumerate(value):
            _no_credentials(child, f"{path}[{index}]")


def validate_pack(pack: Any) -> None:
    if not isinstance(pack, dict):
        raise InvalidData("Supply.pack must be an object")
    kind = pack.get("kind")
    if kind not in PACK_KINDS:
        raise InvalidData(f"unknown Supply.pack kind: {kind!r}")
    if kind == "vial":
        if _num(pack.get("ml"), "pack.ml") <= 0 or _num(pack.get("mgPerMl"), "pack.mgPerMl") <= 0:
            raise InvalidData("vial ml and mgPerMl must be > 0")
    elif kind in {"pill", "pump", "sachet"}:
        if _num(pack.get("mg"), "pack.mg") <= 0:
            raise InvalidData(f"{kind}.mg must be > 0")
    elif set(pack) - {"kind"}:
        raise InvalidData(f"{kind} pack must not carry branch fields")


def validate_quick_dose(item: Any) -> None:
    if not isinstance(item, dict):
        raise InvalidData("quickDose must be an object")
    _id(item.get("id"), "quickDose.id")
    if item.get("route") not in ROUTES or item.get("ester") not in ESTERS:
        raise InvalidData("quickDose route/ester invalid")
    if _num(item.get("value"), "quickDose.value") < 0:
        raise InvalidData("quickDose.value must be >= 0")
    _num(item.get("createdAt"), "quickDose.createdAt")


def validate_supply(item: Any) -> None:
    if not isinstance(item, dict):
        raise InvalidData("supply must be an object")
    _id(item.get("id"), "supply.id")
    if item.get("route") not in ROUTES or item.get("ester") not in ESTERS:
        raise InvalidData("supply route/ester invalid")
    validate_pack(item.get("pack"))
    if _num(item.get("count"), "supply.count") < 0:
        raise InvalidData("supply.count must be >= 0")
    _num(item.get("startedAt"), "supply.startedAt")


def validate_plan_item(item: Any) -> None:
    if not isinstance(item, dict):
        raise InvalidData("plan item must be an object")
    _id(item.get("id"), "plan.id")
    if item.get("route") not in ROUTES or item.get("ester") not in ESTERS:
        raise InvalidData("plan route/ester invalid")
    if _num(item.get("doseMG"), "plan.doseMG") < 0:
        raise InvalidData("plan.doseMG must be >= 0")
    if _num(item.get("everyDays"), "plan.everyDays") < 1:
        raise InvalidData("plan.everyDays must be >= 1")
    if _num(item.get("timesPerDay"), "plan.timesPerDay") < 1:
        raise InvalidData("plan.timesPerDay must be >= 1")
    extras = item.get("extras", {})
    if not isinstance(extras, dict):
        raise InvalidData("plan.extras must be an object")
    for key, value in extras.items():
        _num(value, f"plan.extras.{key}")


VALIDATORS = {
    "quickDoses": validate_quick_dose,
    "supplies": validate_supply,
    "plan": validate_plan_item,
}


def validate_backup(backup: Any) -> None:
    if not isinstance(backup, dict):
        raise InvalidData("backup must be an object")
    if backup.get("formatVersion") != 1:
        raise UnsupportedVersion(f"unsupported formatVersion: {backup.get('formatVersion')!r}")
    _no_credentials(backup)
    modes = backup.get("modes")
    if not isinstance(modes, dict) or not modes:
        raise InvalidData("backup.modes must be a non-empty object")
    for mode, block in modes.items():
        if mode not in {"transfem", "transmasc"} or not isinstance(block, dict):
            raise InvalidData(f"invalid mode: {mode!r}")
        for category in LOCAL_CATEGORIES:
            if category not in block:
                continue
            items = block[category]
            if not isinstance(items, list):
                raise InvalidData(f"{mode}.{category} must be an array")
            seen: set[str] = set()
            for item in items:
                VALIDATORS[category](item)
                if item["id"] in seen:
                    raise InvalidData(f"duplicate id in {mode}.{category}: {item['id']}")
                seen.add(item["id"])
    calibration = backup.get("calibration")
    if calibration is not None:
        if not isinstance(calibration, dict):
            raise InvalidData("calibration must be an object")
        if calibration.get("method") not in {"off", "ekf", "ou_kalman", "mipd"}:
            raise InvalidData("invalid calibration method")
        if calibration.get("historyMode") not in {"forward", "retrospective"}:
            raise InvalidData("invalid calibration history mode")
    preferences = backup.get("preferences", {})
    if not isinstance(preferences, dict):
        raise InvalidData("preferences must be an object")
    for key in preferences:
        if key not in PREFERENCE_KEYS:
            raise InvalidData(f"preference not in approved L1 set: {key}")


def validate_state(state: Any) -> None:
    if not isinstance(state, dict):
        raise InvalidData("state must be an object")
    _no_credentials(state)
    modes = state.get("modes")
    if not isinstance(modes, dict):
        raise InvalidData("state.modes must be an object")
    for mode, block in modes.items():
        if mode not in {"transfem", "transmasc"} or not isinstance(block, dict):
            raise InvalidData(f"invalid state mode: {mode!r}")
        for category in LOCAL_CATEGORIES:
            for item in block.get(category, []):
                VALIDATORS[category](item)


def _by_id(items: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    return {item["id"]: item for item in items}


def merge_category(
    local: list[dict[str, Any]],
    incoming: list[dict[str, Any]],
    policy: str,
) -> list[dict[str, Any]]:
    if policy not in {"keep-local", "prefer-backup", "skip", "require-user-choice"}:
        raise InvalidData(f"unknown conflict policy: {policy}")
    out = copy.deepcopy(local)
    index = _by_id(out)
    for item in copy.deepcopy(incoming):
        existing = index.get(item["id"])
        if existing is None:
            out.append(item)
            index[item["id"]] = item
        elif policy == "prefer-backup":
            existing.clear()
            existing.update(item)
        elif policy == "require-user-choice":
            raise Conflict(f"conflict for id {item['id']!r}")
        # keep-local and skip both retain the existing item
    return out


def merge_state(
    state: dict[str, Any],
    backup: dict[str, Any],
    policy: str = "keep-local",
) -> dict[str, Any]:
    validate_state(state)
    validate_backup(backup)
    out = copy.deepcopy(state)
    for mode, block in backup["modes"].items():
        target = out["modes"].setdefault(mode, {category: [] for category in LOCAL_CATEGORIES})
        for category in LOCAL_CATEGORIES:
            if category in block:
                target[category] = merge_category(target.get(category, []), block[category], policy)
    if "calibration" in backup:
        out["calibration"] = copy.deepcopy(backup["calibration"])
    if "preferences" in backup:
        out.setdefault("preferences", {}).update(copy.deepcopy(backup["preferences"]))
    return out


def replace_state(
    state: dict[str, Any],
    backup: dict[str, Any],
) -> dict[str, Any]:
    validate_state(state)
    validate_backup(backup)
    out = copy.deepcopy(state)
    for mode, block in backup["modes"].items():
        target = out["modes"].setdefault(mode, {category: [] for category in LOCAL_CATEGORIES})
        for category in LOCAL_CATEGORIES:
            if category in block:
                target[category] = copy.deepcopy(block[category])
    if "calibration" in backup:
        out["calibration"] = copy.deepcopy(backup["calibration"])
    if "preferences" in backup:
        out.setdefault("preferences", {}).update(copy.deepcopy(backup["preferences"]))
    return out


def apply_transaction(
    state: dict[str, Any],
    new_state: dict[str, Any],
    fail_during_commit: bool = False,
) -> dict[str, Any]:
    """Apply all-or-nothing; on simulated failure restore the original state."""
    validate_state(new_state)
    snapshot = copy.deepcopy(state)
    working = copy.deepcopy(state)
    try:
        if fail_during_commit:
            raise RuntimeError("simulated commit failure")
        working.clear()
        working.update(copy.deepcopy(new_state))
        return working
    except Exception:
        working.clear()
        working.update(snapshot)
        raise


def restore_warnings(
    backup_mode: str,
    target_mode: str,
    source_account: str | None = None,
    target_account: str | None = None,
) -> list[str]:
    warnings: list[str] = []
    if backup_mode != target_mode:
        warnings.append(f"mode mismatch: backup={backup_mode}, target={target_mode}")
    if source_account is not None and target_account is not None and source_account != target_account:
        warnings.append("account mismatch: restore targets a different account namespace")
    return warnings


def fixture_state() -> dict[str, Any]:
    return {
        "modes": {
            "transfem": {
                "quickDoses": [{"id": "q-local", "route": "sublingual", "ester": "E2", "value": 1, "createdAt": 1000}],
                "supplies": [{"id": "s-local", "route": "injection", "ester": "EV", "pack": {"kind": "vial", "ml": 5, "mgPerMl": 40}, "count": 2, "startedAt": 1000}],
                "plan": [{"id": "p-local", "route": "injection", "ester": "EV", "doseMG": 2, "extras": {}, "everyDays": 7, "timesPerDay": 1}],
            },
            "transmasc": {"quickDoses": [], "supplies": [], "plan": []},
        },
        "calibration": {"method": "mipd", "historyMode": "forward"},
        "preferences": {"hrt-lang": "zh"},
    }


def fixture_backup() -> dict[str, Any]:
    return {
        "formatVersion": 1,
        "modes": {
            "transfem": {
                "quickDoses": [{"id": "q-new", "route": "oral", "ester": "E2", "value": 2, "createdAt": 2000}],
                "supplies": [{"id": "s-new", "route": "oral", "ester": "E2", "pack": {"kind": "pill", "mg": 2}, "count": 10, "startedAt": 2000}],
                "plan": [{"id": "p-new", "route": "oral", "ester": "E2", "doseMG": 2, "extras": {}, "everyDays": 1, "timesPerDay": 2}],
            },
            "transmasc": {"quickDoses": [], "supplies": [], "plan": []},
        },
        "calibration": {"method": "ekf", "historyMode": "retrospective"},
        "preferences": {"hrt-lang": "en", "app-theme": "dark"},
    }
