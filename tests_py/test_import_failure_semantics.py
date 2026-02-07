from pathlib import Path

from tests_py.helpers import json_stdout, run_grns


def test_import_strict_orphan_reports_structured_errors_and_partial_state(running_server, tmp_path: Path):
    env = running_server

    import_file = tmp_path / "strict_orphan.jsonl"
    import_file.write_text(
        '{"id":"gr-or11","title":"Orphan","status":"open","type":"task","priority":2,"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","deps":[{"parent_id":"gr-zz99","type":"blocks"}]}\n',
        encoding="utf-8",
    )

    proc = run_grns(
        env,
        "import",
        "-i",
        str(import_file),
        "--orphan-handling",
        "strict",
        "--json",
    )

    result = json_stdout(proc)
    assert int(result["created"]) == 1
    assert int(result["errors"]) == 1
    assert any("strict orphan dep" in msg for msg in result.get("messages", []))

    # Contract: strict orphan errors are reported structurally; task upsert can still be applied.
    show_proc = run_grns(env, "show", "gr-or11", "--json")
    shown = json_stdout(show_proc)
    assert shown["id"] == "gr-or11"
    assert shown.get("deps", []) == []
