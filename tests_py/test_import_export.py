"""Tests for import/export CLI commands.

Migrated from tests/cli_import_export.bats.
"""

import json
from pathlib import Path

import pytest

from tests_py.helpers import json_stdout, run_grns, run_grns_fail


# ---------------------------------------------------------------------------
# Export
# ---------------------------------------------------------------------------


def test_export_produces_jsonl(running_server):
    env = running_server
    run_grns(env, "create", "Export task one", "--json")
    run_grns(env, "create", "Export task two", "-l", "label1", "--json")

    proc = run_grns(env, "export")
    lines = [l for l in proc.stdout.strip().split("\n") if l]
    assert len(lines) == 2

    records = [json.loads(line) for line in lines]
    assert all("id" in record for record in records)


def test_export_to_file(running_server, tmp_path):
    env = running_server
    run_grns(env, "create", "File export", "--json")

    outfile = tmp_path / "export.jsonl"
    run_grns(env, "export", "-o", str(outfile))
    assert outfile.exists()

    lines = [l for l in outfile.read_text().strip().split("\n") if l]
    assert len(lines) >= 1


# ---------------------------------------------------------------------------
# Import basics
# ---------------------------------------------------------------------------


def test_import_from_jsonl_file(running_server, make_server, tmp_path):
    env = running_server

    created = json_stdout(run_grns(env, "create", "Import me", "-l", "tag1", "--custom", "env=prod", "--json"))
    task_id = created["id"]

    outfile = tmp_path / "export.jsonl"
    run_grns(env, "export", "-o", str(outfile))

    with make_server("import_target") as env2:
        result = json_stdout(run_grns(env2, "import", "-i", str(outfile), "--json"))
        assert int(result["created"]) == 1

        shown = json_stdout(run_grns(env2, "show", task_id, "--json"))
        assert shown["title"] == "Import me"


def test_import_stream(running_server, make_server, tmp_path):
    env = running_server

    created = json_stdout(run_grns(env, "create", "Stream import me", "--json"))
    task_id = created["id"]

    outfile = tmp_path / "export_stream.jsonl"
    run_grns(env, "export", "-o", str(outfile))

    with make_server("import_stream") as env2:
        result = json_stdout(run_grns(env2, "import", "-i", str(outfile), "--stream", "--json"))
        assert int(result["created"]) == 1

        shown = json_stdout(run_grns(env2, "show", task_id, "--json"))
        assert shown["title"] == "Stream import me"


def test_import_dry_run(running_server, make_server, tmp_path):
    env = running_server

    created = json_stdout(run_grns(env, "create", "Dry run test", "--json"))
    task_id = created["id"]

    outfile = tmp_path / "dry.jsonl"
    run_grns(env, "export", "-o", str(outfile))

    with make_server("dry_target") as env2:
        result = json_stdout(run_grns(env2, "import", "-i", str(outfile), "--dry-run", "--json"))
        assert int(result["created"]) == 1
        assert result["dry_run"] is True

        # Task should NOT actually exist.
        proc = run_grns_fail(env2, "show", task_id, "--json")
        assert proc.returncode != 0


# ---------------------------------------------------------------------------
# Dedupe
# ---------------------------------------------------------------------------


def test_import_dedupe_skip(running_server, tmp_path):
    env = running_server

    run_grns(env, "create", "Dedupe test", "--json")

    outfile = tmp_path / "dedupe.jsonl"
    run_grns(env, "export", "-o", str(outfile))

    # Import into same DB — should skip.
    result = json_stdout(run_grns(env, "import", "-i", str(outfile), "--dedupe", "skip", "--json"))
    assert int(result["skipped"]) == 1
    assert int(result["created"]) == 0


# ---------------------------------------------------------------------------
# Round-trip preservation
# ---------------------------------------------------------------------------


def test_round_trip_preserves_data(running_server, make_server, tmp_path):
    env = running_server

    parent = json_stdout(run_grns(env, "create", "Parent task", "--json"))
    parent_id = parent["id"]

    child = json_stdout(run_grns(
        env, "create", "Child task", "-l", "important",
        "--custom", "env=staging", "--deps", parent_id, "--json",
    ))
    child_id = child["id"]

    outfile = tmp_path / "roundtrip.jsonl"
    run_grns(env, "export", "-o", str(outfile))

    with make_server("roundtrip_target") as env2:
        result = json_stdout(run_grns(env2, "import", "-i", str(outfile), "--json"))
        assert int(result["created"]) == 2

        shown = json_stdout(run_grns(env2, "show", child_id, "--json"))
        assert shown["title"] == "Child task"
        assert "important" in shown.get("labels", [])
        assert shown.get("custom", {}).get("env") == "staging"

        deps = shown.get("deps", [])
        assert len(deps) == 1
        dep_pairs = {(dep["parent_id"], dep["type"]) for dep in deps}
        assert dep_pairs == {(parent_id, "blocks")}


# ---------------------------------------------------------------------------
# Dep preservation on dedupe skip/error
# ---------------------------------------------------------------------------


DEDUPE_RECORD = json.dumps({
    "id": "gr-ch11",
    "title": "Child",
    "status": "open",
    "type": "task",
    "priority": 2,
    "created_at": "2026-01-01T00:00:00Z",
    "updated_at": "2026-01-01T00:00:00Z",
    "deps": [{"parent_id": "gr-pa22", "type": "blocks"}],
})


@pytest.mark.parametrize("dedupe_mode", ["skip", "error"])
def test_import_dedupe_does_not_rewrite_deps(running_server, tmp_path, dedupe_mode):
    env = running_server

    run_grns(env, "create", "Parent one", "--id", "gr-pa11", "--json")
    run_grns(env, "create", "Parent two", "--id", "gr-pa22", "--json")
    run_grns(env, "create", "Child", "--id", "gr-ch11", "--deps", "gr-pa11", "--json")

    infile = tmp_path / f"import_{dedupe_mode}_deps.jsonl"
    infile.write_text(DEDUPE_RECORD + "\n")

    run_grns(env, "import", "-i", str(infile), "--dedupe", dedupe_mode, "--json")

    shown = json_stdout(run_grns(env, "show", "gr-ch11", "--json"))
    dep_parent_ids = {dep["parent_id"] for dep in shown.get("deps", [])}
    assert dep_parent_ids == {"gr-pa11"}


# ---------------------------------------------------------------------------
# Overwrite dep semantics
# ---------------------------------------------------------------------------


def test_overwrite_explicit_empty_deps_clears(running_server, tmp_path):
    env = running_server

    run_grns(env, "create", "Parent", "--id", "gr-pa11", "--json")
    run_grns(env, "create", "Child", "--id", "gr-ch11", "--deps", "gr-pa11", "--json")

    record = json.dumps({
        "id": "gr-ch11", "title": "Child", "status": "open", "type": "task",
        "priority": 2, "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z", "deps": [],
    })
    infile = tmp_path / "import_clear_deps.jsonl"
    infile.write_text(record + "\n")

    run_grns(env, "import", "-i", str(infile), "--dedupe", "overwrite", "--json")

    shown = json_stdout(run_grns(env, "show", "gr-ch11", "--json"))
    assert shown.get("deps", []) == []


def test_overwrite_without_deps_field_preserves(running_server, tmp_path):
    env = running_server

    run_grns(env, "create", "Parent", "--id", "gr-pa11", "--json")
    run_grns(env, "create", "Child", "--id", "gr-ch11", "--deps", "gr-pa11", "--json")

    record = json.dumps({
        "id": "gr-ch11", "title": "Child renamed", "status": "open", "type": "task",
        "priority": 2, "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    })
    infile = tmp_path / "import_preserve_deps.jsonl"
    infile.write_text(record + "\n")

    run_grns(env, "import", "-i", str(infile), "--dedupe", "overwrite", "--json")

    shown = json_stdout(run_grns(env, "show", "gr-ch11", "--json"))
    dep_parent_ids = {dep["parent_id"] for dep in shown.get("deps", [])}
    assert dep_parent_ids == {"gr-pa11"}


# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------


def test_import_rejects_invalid_status(running_server, tmp_path):
    env = running_server

    record = json.dumps({
        "id": "gr-aa11", "title": "Bad status", "status": "nope", "type": "task",
        "priority": 2, "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    })
    infile = tmp_path / "import_invalid_status.jsonl"
    infile.write_text(record + "\n")

    proc = run_grns_fail(env, "import", "-i", str(infile), "--json")
    assert proc.returncode != 0
    assert "invalid status" in proc.stdout + proc.stderr


# ---------------------------------------------------------------------------
# Status normalization on overwrite
# ---------------------------------------------------------------------------


def test_overwrite_closed_sets_closed_at(running_server, tmp_path):
    env = running_server

    run_grns(env, "create", "Task", "--id", "gr-aa11", "--json")

    record = json.dumps({
        "id": "gr-aa11", "title": "Task", "status": "closed", "type": "task",
        "priority": 2, "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    })
    infile = tmp_path / "import_closed.jsonl"
    infile.write_text(record + "\n")

    run_grns(env, "import", "-i", str(infile), "--dedupe", "overwrite", "--json")

    shown = json_stdout(run_grns(env, "show", "gr-aa11", "--json"))
    assert shown["status"] == "closed"
    assert "closed_at" in shown


def test_overwrite_open_clears_closed_at(running_server, tmp_path):
    env = running_server

    run_grns(env, "create", "Task", "--id", "gr-aa11", "--json")
    run_grns(env, "close", "gr-aa11", "--json")

    record = json.dumps({
        "id": "gr-aa11", "title": "Task", "status": "open", "type": "task",
        "priority": 2, "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    })
    infile = tmp_path / "import_open.jsonl"
    infile.write_text(record + "\n")

    run_grns(env, "import", "-i", str(infile), "--dedupe", "overwrite", "--json")

    shown = json_stdout(run_grns(env, "show", "gr-aa11", "--json"))
    assert shown["status"] == "open"
    assert "closed_at" not in shown or shown["closed_at"] is None


# ---------------------------------------------------------------------------
# Error reporting
# ---------------------------------------------------------------------------


def test_import_invalid_json_reports_line_number(running_server, tmp_path):
    env = running_server

    infile = tmp_path / "import_invalid_line.jsonl"
    infile.write_text(
        '{"id":"gr-aa11","title":"Good","status":"open","type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}\n'
        '{"id":"gr-bb22","title":"Bad",\n'
    )

    proc = run_grns_fail(env, "import", "-i", str(infile), "--stream", "--json")
    assert proc.returncode != 0
    assert "line 2" in proc.stdout + proc.stderr
