import copy
import unittest

from l1_restore_model import (
    Conflict,
    InvalidData,
    UnsupportedVersion,
    apply_transaction,
    fixture_backup,
    fixture_state,
    merge_category,
    merge_state,
    replace_state,
    restore_warnings,
    validate_backup,
    validate_pack,
)


def clone(value):
    return copy.deepcopy(value)


def with_quick(backup, items):
    b = clone(backup)
    b["modes"]["transfem"]["quickDoses"] = items
    return b


class L1RestoreTests(unittest.TestCase):
    def setUp(self):
        self.state = fixture_state()
        self.backup = fixture_backup()

    def test_t01_round_trip_both_modes(self):
        validate_backup(self.backup)
        merged = merge_state(self.state, self.backup, policy="prefer-backup")
        self.assertEqual(len(merged["modes"]["transfem"]["quickDoses"]), 2)
        self.assertEqual(len(merged["modes"]["transfem"]["supplies"]), 2)
        self.assertEqual(len(merged["modes"]["transfem"]["plan"]), 2)

    def test_t02_merge_adds_and_keeps_local_on_conflict(self):
        local_only = merge_state(self.state, self.backup, policy="keep-local")
        quick = local_only["modes"]["transfem"]["quickDoses"]
        self.assertEqual({x["id"] for x in quick}, {"q-local", "q-new"})

        conflict = with_quick(
            self.backup,
            [{"id": "q-local", "route": "oral", "ester": "E2", "value": 9, "createdAt": 2000}],
        )
        kept = merge_state(self.state, conflict, policy="keep-local")
        self.assertEqual(kept["modes"]["transfem"]["quickDoses"][0]["value"], 1)
        replaced = merge_state(self.state, conflict, policy="prefer-backup")
        self.assertEqual(replaced["modes"]["transfem"]["quickDoses"][0]["value"], 9)
        with self.assertRaises(Conflict):
            merge_state(self.state, conflict, policy="require-user-choice")

    def test_t03_replace_only_selected_categories(self):
        backup = {"formatVersion": 1, "modes": {"transfem": {"quickDoses": []}}}
        replaced = replace_state(self.state, backup)
        self.assertEqual(replaced["modes"]["transfem"]["quickDoses"], [])
        self.assertEqual(len(replaced["modes"]["transfem"]["supplies"]), 1)
        self.assertEqual(len(replaced["modes"]["transfem"]["plan"]), 1)

    def test_t04_duplicate_ids_in_backup_rejected(self):
        item = {"id": "dup", "route": "oral", "ester": "E2", "value": 1, "createdAt": 1}
        with self.assertRaises(InvalidData):
            validate_backup(with_quick(self.backup, [item, clone(item)]))

    def test_t05_supply_pack_object_union_valid_and_invalid(self):
        valid = [
            {"kind": "vial", "ml": 5, "mgPerMl": 40},
            {"kind": "pill", "mg": 2},
            {"kind": "pump", "mg": 1.25},
            {"kind": "sachet", "mg": 1.5},
            {"kind": "patch"},
            {"kind": "bulk"},
        ]
        for pack in valid:
            validate_pack(pack)
        for pack in [
            {"kind": "vial", "ml": 0, "mgPerMl": 40},
            {"kind": "pill", "mg": -1},
            {"kind": "patch", "mg": 2},
            {"kind": "unknown"},
        ]:
            with self.assertRaises(InvalidData):
                validate_pack(pack)

    def test_t06_string_supply_pack_rejected(self):
        b = clone(self.backup)
        b["modes"]["transfem"]["supplies"][0]["pack"] = "vial"
        with self.assertRaises(InvalidData):
            validate_backup(b)

    def test_t07_plan_bounds(self):
        b = clone(self.backup)
        plan = b["modes"]["transfem"]["plan"][0]
        plan["everyDays"] = 0
        with self.assertRaises(InvalidData):
            validate_backup(b)
        b = clone(self.backup)
        b["modes"]["transfem"]["plan"][0]["timesPerDay"] = 0
        with self.assertRaises(InvalidData):
            validate_backup(b)
        b = clone(self.backup)
        b["modes"]["transfem"]["plan"][0]["doseMG"] = -1
        with self.assertRaises(InvalidData):
            validate_backup(b)

    def test_t08_invalid_calibration_rejected(self):
        b = clone(self.backup)
        b["calibration"] = {"method": "legacy", "historyMode": "forward"}
        with self.assertRaises(InvalidData):
            validate_backup(b)

    def test_t09_preferences_and_credentials(self):
        merged = merge_state(self.state, self.backup, policy="keep-local")
        self.assertEqual(merged["preferences"]["hrt-lang"], "en")
        b = clone(self.backup)
        b["credentials"] = {"auth_token": "synthetic-not-a-secret"}
        with self.assertRaises(InvalidData):
            validate_backup(b)
        b = clone(self.backup)
        b["modes"]["transfem"]["quickDoses"][0]["auth_token"] = "synthetic-not-a-secret"
        with self.assertRaises(InvalidData):
            validate_backup(b)

    def test_t10_unknown_version_rejected(self):
        b = clone(self.backup)
        b["formatVersion"] = 2
        with self.assertRaises(UnsupportedVersion):
            validate_backup(b)

    def test_t11_transaction_rollback(self):
        before = clone(self.state)
        with self.assertRaises(RuntimeError):
            apply_transaction(self.state, merge_state(self.state, self.backup), fail_during_commit=True)
        self.assertEqual(self.state, before)
        applied = apply_transaction(self.state, merge_state(self.state, self.backup))
        self.assertNotEqual(applied, self.state)

    def test_t12_cross_mode_and_account_warnings(self):
        warnings = restore_warnings("transmasc", "transfem", "account-a", "account-b")
        self.assertTrue(any("mode" in w.lower() for w in warnings))
        self.assertTrue(any("account" in w.lower() for w in warnings))
        self.assertEqual(restore_warnings("transfem", "transfem"), [])

    def test_t13_credentials_never_persist(self):
        b = clone(self.backup)
        b["preferences"]["enc_key"] = "synthetic-not-a-secret"
        with self.assertRaises(InvalidData):
            validate_backup(b)
        merged = merge_state(self.state, self.backup)
        self.assertNotIn("enc_key", merged["preferences"])

    def test_t14_empty_vs_missing(self):
        backup = {"formatVersion": 1, "modes": {"transfem": {"quickDoses": []}}}
        replaced = replace_state(self.state, backup)
        self.assertEqual(replaced["modes"]["transfem"]["quickDoses"], [])
        self.assertEqual(len(replaced["modes"]["transfem"]["supplies"]), 1)

    def test_t15_repeated_merge_idempotent(self):
        once = merge_state(self.state, self.backup, policy="prefer-backup")
        twice = merge_state(once, self.backup, policy="prefer-backup")
        self.assertEqual(len(twice["modes"]["transfem"]["quickDoses"]), 2)
        self.assertEqual(len(twice["modes"]["transfem"]["supplies"]), 2)
        self.assertEqual(len(twice["modes"]["transfem"]["plan"]), 2)
        self.assertEqual(once, twice)


if __name__ == "__main__":
    unittest.main()
