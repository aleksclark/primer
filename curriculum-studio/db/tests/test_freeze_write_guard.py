"""Adversarial guards: freeze_inventory.py --write must refuse live envs/markers."""

from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

DB_ROOT = Path(__file__).resolve().parents[1]
SCRIPT = DB_ROOT / "scripts" / "freeze_inventory.py"
BASELINE = [
    "00001_identity_and_catalogs.sql",
    "00002_plan_domain.sql",
    "00003_materialization_and_integration.sql",
    "00004_invariants.sql",
]


def _seed_tree(tmp: Path) -> Path:
    mig = tmp / "migrations"
    mig.mkdir(parents=True)
    src = DB_ROOT / "migrations"
    for name in BASELINE:
        shutil.copy2(src / name, mig / name)
    # seed manifest from real committed inventory
    shutil.copy2(DB_ROOT / "baseline_manifest.json", tmp / "baseline_manifest.json")
    return mig


def _run_write(cwd: Path, env: dict[str, str] | None = None) -> subprocess.CompletedProcess[str]:
    # Invoke script with ROOT overridden via cwd layout: script parents[1] is db root.
    # Copy script-local tree so Path(__file__).parents[1] == cwd when we exec a copy.
    # Instead, run the real script but monkey via env that the script must honor:
    # STUDIO_MIGRATIONS_LIVE / marker under the real db root would be dangerous.
    # Prefer running a temp copy of the script with ROOT patched — exercise the
    # installed script against a temp DB tree by setting cwd and using
    # PYTHONPATH + running module after chdir into temp that mirrors layout.
    script_copy = cwd / "scripts" / "freeze_inventory.py"
    script_copy.parent.mkdir(parents=True, exist_ok=True)
    text = SCRIPT.read_text(encoding="utf-8")
    # Point ROOT at the temp db root (parent of migrations/).
    text = text.replace(
        'ROOT = Path(__file__).resolve().parents[1]',
        f'ROOT = Path({str(cwd)!r})',
    )
    script_copy.write_text(text, encoding="utf-8")
    run_env = os.environ.copy()
    if env:
        run_env.update(env)
    return subprocess.run(
        [sys.executable, str(script_copy), "--write"],
        cwd=str(cwd),
        env=run_env,
        text=True,
        capture_output=True,
        check=False,
    )


def test_write_refuses_live_env(tmp_path: Path) -> None:
    mig = _seed_tree(tmp_path)
    # Mutate SQL — write would otherwise normalize drift into the manifest.
    target = mig / "00002_plan_domain.sql"
    target.write_bytes(target.read_bytes() + b"\n-- py adversarial drift\n")
    before = (tmp_path / "baseline_manifest.json").read_bytes()

    proc = _run_write(tmp_path, {"STUDIO_MIGRATIONS_LIVE": "true"})
    assert proc.returncode != 0, proc.stdout + proc.stderr
    assert "live" in (proc.stderr + proc.stdout).lower()
    after = (tmp_path / "baseline_manifest.json").read_bytes()
    assert after == before, "manifest must not be rewritten under live env"


def test_write_refuses_marker(tmp_path: Path) -> None:
    mig = _seed_tree(tmp_path)
    (tmp_path / "STUDIO_MIGRATIONS_LIVE").write_text("1\n", encoding="utf-8")
    target = mig / "00001_identity_and_catalogs.sql"
    target.write_bytes(target.read_bytes() + b"\n-- marker drift\n")
    before = (tmp_path / "baseline_manifest.json").read_bytes()

    proc = _run_write(tmp_path, {"STUDIO_MIGRATIONS_LIVE": ""})
    assert proc.returncode != 0, proc.stdout + proc.stderr
    after = (tmp_path / "baseline_manifest.json").read_bytes()
    assert after == before


def test_check_still_available_when_live(tmp_path: Path) -> None:
    _seed_tree(tmp_path)
    (tmp_path / "STUDIO_MIGRATIONS_LIVE").write_text("1\n", encoding="utf-8")

    script_copy = tmp_path / "scripts" / "freeze_inventory.py"
    script_copy.parent.mkdir(parents=True, exist_ok=True)
    text = SCRIPT.read_text(encoding="utf-8")
    text = text.replace(
        'ROOT = Path(__file__).resolve().parents[1]',
        f'ROOT = Path({str(tmp_path)!r})',
    )
    script_copy.write_text(text, encoding="utf-8")
    env = os.environ.copy()
    env["STUDIO_MIGRATIONS_LIVE"] = "true"
    proc = subprocess.run(
        [sys.executable, str(script_copy), "--check"],
        cwd=str(tmp_path),
        env=env,
        text=True,
        capture_output=True,
        check=False,
    )
    assert proc.returncode == 0, proc.stdout + proc.stderr
    assert "ok" in proc.stdout.lower()


def test_write_allowed_non_live(tmp_path: Path) -> None:
    mig = _seed_tree(tmp_path)
    target = mig / "00003_materialization_and_integration.sql"
    target.write_bytes(target.read_bytes() + b"\n-- pre-live rewrite ok\n")

    proc = _run_write(tmp_path, {"STUDIO_MIGRATIONS_LIVE": ""})
    assert proc.returncode == 0, proc.stdout + proc.stderr
    data = json.loads((tmp_path / "baseline_manifest.json").read_text(encoding="utf-8"))
    assert data["migrations"], "manifest should be regenerated"


LIVE_TRUTHY_SPELLINGS = [
    "1",
    "true",
    "TRUE",
    "True",
    "yes",
    "YES",
    "on",
    "ON",
    "Yes",
    " true",
    "true ",
    " true ",
]

NON_LIVE_SPELLINGS = ["", "0", "false", "FALSE", "no", "off", "t", "y"]


@pytest.mark.parametrize("spelling", LIVE_TRUTHY_SPELLINGS)
def test_write_refuses_truthy_spellings(tmp_path: Path, spelling: str) -> None:
    mig = _seed_tree(tmp_path)
    target = mig / "00002_plan_domain.sql"
    target.write_bytes(target.read_bytes() + b"\n-- py spelling drift\n")
    before = (tmp_path / "baseline_manifest.json").read_bytes()

    proc = _run_write(tmp_path, {"STUDIO_MIGRATIONS_LIVE": spelling})
    assert proc.returncode != 0, f"spelling {spelling!r} must refuse: {proc.stdout}{proc.stderr}"
    assert "live" in (proc.stderr + proc.stdout).lower()
    after = (tmp_path / "baseline_manifest.json").read_bytes()
    assert after == before, f"manifest must stay byte-identical for {spelling!r}"


@pytest.mark.parametrize("spelling", NON_LIVE_SPELLINGS)
def test_truthy_helper_non_live(spelling: str) -> None:
    # Import helpers from a temp-patched copy is heavy; assert via script module
    # semantics by reading the installed helper after import patch.
    import importlib.util

    spec = importlib.util.spec_from_file_location("freeze_inventory", SCRIPT)
    assert spec and spec.loader
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    assert mod._truthy_env_value(spelling) is False


@pytest.mark.parametrize("spelling", LIVE_TRUTHY_SPELLINGS)
def test_truthy_helper_live(spelling: str) -> None:
    import importlib.util

    spec = importlib.util.spec_from_file_location("freeze_inventory", SCRIPT)
    assert spec and spec.loader
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    assert mod._truthy_env_value(spelling) is True


@pytest.mark.parametrize(
    "shape",
    ["regular-file", "directory", "symlink-to-file", "symlink-to-dir"],
)
def test_write_refuses_marker_shapes(tmp_path: Path, shape: str) -> None:
    mig = _seed_tree(tmp_path)
    marker = tmp_path / "STUDIO_MIGRATIONS_LIVE"
    if shape == "regular-file":
        marker.write_text("1\n", encoding="utf-8")
    elif shape == "directory":
        marker.mkdir()
    elif shape == "symlink-to-file":
        tgt = tmp_path / "marker-target"
        tgt.write_text("1\n", encoding="utf-8")
        marker.symlink_to(tgt)
    elif shape == "symlink-to-dir":
        tgt = tmp_path / "marker-target-dir"
        tgt.mkdir()
        marker.symlink_to(tgt)

    target = mig / "00001_identity_and_catalogs.sql"
    target.write_bytes(target.read_bytes() + b"\n-- py shape drift\n")
    before = (tmp_path / "baseline_manifest.json").read_bytes()

    proc = _run_write(tmp_path, {"STUDIO_MIGRATIONS_LIVE": ""})
    assert proc.returncode != 0, f"shape {shape} must refuse: {proc.stdout}{proc.stderr}"
    after = (tmp_path / "baseline_manifest.json").read_bytes()
    assert after == before, f"manifest unchanged for marker shape {shape}"


def test_marker_exists_broken_symlink_not_live(tmp_path: Path) -> None:
    import importlib.util

    marker = tmp_path / "STUDIO_MIGRATIONS_LIVE"
    marker.symlink_to(tmp_path / "missing-target")
    # Patch ROOT for helper
    text = SCRIPT.read_text(encoding="utf-8")
    script_copy = tmp_path / "scripts" / "freeze_inventory.py"
    script_copy.parent.mkdir(parents=True, exist_ok=True)
    text = text.replace(
        "ROOT = Path(__file__).resolve().parents[1]",
        f"ROOT = Path({str(tmp_path)!r})",
    )
    script_copy.write_text(text, encoding="utf-8")
    spec = importlib.util.spec_from_file_location("freeze_inventory_broken", script_copy)
    assert spec and spec.loader
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    assert mod.live_marker_exists(marker) is False
    assert mod.is_live_env() is False
