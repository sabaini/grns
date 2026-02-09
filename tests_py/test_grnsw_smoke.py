import json
import subprocess
import sys
from pathlib import Path

from tests_py.helpers import json_stdout, run_grns


REPO_ROOT = Path(__file__).resolve().parents[1]
GRNSW = REPO_ROOT / "scripts" / "grnsw.py"


def run_grnsw(env: dict[str, str], *args: str, check: bool = True) -> subprocess.CompletedProcess[str]:
    proc_env = dict(env)
    proc_env["GRNSW_GRNS_BIN"] = env["GRNS_BIN"]
    proc = subprocess.run(
        [sys.executable, str(GRNSW), *args],
        text=True,
        capture_output=True,
        env=proc_env,
        cwd=env.get("GRNS_REPO_ROOT"),
    )
    if check and proc.returncode != 0:
        raise AssertionError(
            f"grnsw failed ({proc.returncode}): {' '.join(args)}\n"
            f"stdout={proc.stdout}\n"
            f"stderr={proc.stderr}"
        )
    return proc


def json_out(proc: subprocess.CompletedProcess[str]) -> dict:
    return json.loads(proc.stdout)


def test_grnsw_doctor_reports_ok(running_server):
    proc = run_grnsw(running_server, "--json", "doctor")
    data = json_out(proc)

    assert data["ok"] is True
    names = {c["name"] for c in data["checks"]}
    assert "grns" in names
    assert "jq" in names
    assert data["info"]["project_prefix"] == "gr"


def test_grnsw_scaffold_phase_and_validation(running_server):
    env = running_server

    epic = json_out(
        run_grnsw(
            env,
            "--json",
            "epic",
            "new",
            "Auth epic",
            "--spec-id",
            "docs/specs/auth.md",
            "--design",
            "docs/specs/auth.md",
        )
    )
    epic_id = epic["created_id"]

    phase = json_out(run_grnsw(env, "--json", "phase", "add", "Phase 1", "--epic", epic_id))
    phase_id = phase["created_id"]

    validation = json_out(
        run_grnsw(
            env,
            "--json",
            "validation",
            "add",
            "Validate Phase 1",
            "--milestone",
            phase_id,
            "--reviewer",
            "alice",
            "--acceptance",
            "JWT issued for valid credentials",
        )
    )
    validation_id = validation["created_id"]

    phase_task = json_stdout(run_grns(env, "show", phase_id, "--json"))
    assert phase_task["parent_id"] == epic_id
    assert "phase" in phase_task["labels"]

    validation_task = json_stdout(run_grns(env, "show", validation_id, "--json"))
    assert validation_task["parent_id"] == phase_id
    assert validation_task["assignee"] == "alice"
    assert "validation" in validation_task["labels"]
    assert "JWT issued" in validation_task["acceptance_criteria"]
    assert any(dep["parent_id"] == phase_id and dep["type"] == "blocks" for dep in validation_task["deps"])


def test_grnsw_discover_add_creates_dependency_and_custom(running_server):
    env = running_server
    base = json_stdout(run_grns(env, "create", "Main task", "-t", "task", "-p", "1", "--json"))
    base_id = base["id"]

    discovered = json_out(
        run_grnsw(env, "--json", "discover", "add", "Follow-up task", "--from", base_id, "--priority", "2")
    )
    discovered_id = discovered["created_id"]

    new_task = json_stdout(run_grns(env, "show", discovered_id, "--json"))
    assert new_task["custom"]["discovered_from"] == base_id

    base_task = json_stdout(run_grns(env, "show", base_id, "--json"))
    assert any(dep["parent_id"] == discovered_id and dep["type"] == "blocks" for dep in base_task["deps"])


def test_grnsw_gate_human_creates_labeled_blocker(running_server):
    env = running_server
    agent = json_stdout(run_grns(env, "create", "Agent task", "-t", "task", "-p", "1", "--json"))
    agent_id = agent["id"]

    gate = json_out(
        run_grnsw(
            env,
            "--json",
            "gate",
            "human",
            "Need API spec decision",
            "--agent",
            agent_id,
            "--assignee",
            "alice",
            "--kind",
            "spec",
            "--description",
            "Need API spec before implementation",
        )
    )
    human_id = gate["human_task_id"]

    human_task = json_stdout(run_grns(env, "show", human_id, "--json"))
    assert human_task["assignee"] == "alice"
    assert "human-input" in human_task["labels"]
    assert "spec" in human_task["labels"]
    assert "Requested spec" in human_task["acceptance_criteria"]

    agent_task = json_stdout(run_grns(env, "show", agent_id, "--json"))
    assert any(dep["parent_id"] == human_id and dep["type"] == "blocks" for dep in agent_task["deps"])


def test_grnsw_checkpoint_set_updates_task_notes(running_server):
    env = running_server
    task = json_stdout(run_grns(env, "create", "Checkpoint target", "-t", "task", "-p", "1", "--json"))
    task_id = task["id"]

    out = json_out(
        run_grnsw(
            env,
            "--json",
            "checkpoint",
            "set",
            "--task",
            task_id,
            "--stopped-at",
            "handlers.go",
            "--tried",
            "regex filter",
            "--next",
            "write integration test",
        )
    )

    note = out["note"]
    assert note.startswith("[")
    assert "Stopped at: handlers.go." in note
    assert "Tried: regex filter." in note
    assert "Next: write integration test." in note

    shown = json_stdout(run_grns(env, "show", task_id, "--json"))
    assert shown["notes"] == note


def test_grnsw_triage_human_lists_human_input_tasks(running_server):
    env = running_server
    agent = json_stdout(run_grns(env, "create", "Agent target", "-t", "task", "-p", "1", "--json"))

    gate = json_out(
        run_grnsw(
            env,
            "--json",
            "gate",
            "human",
            "Decision needed",
            "--agent",
            agent["id"],
            "--assignee",
            "alice",
            "--kind",
            "decision",
        )
    )
    human_id = gate["human_task_id"]

    triage = json_out(run_grnsw(env, "--json", "triage", "human"))
    ids = {task["id"] for task in triage["tasks"]}

    assert triage["count"] >= 1
    assert human_id in ids


def test_grnsw_triage_stale_human_dry_run_builds_expected_query(running_server):
    out = json_out(
        run_grnsw(
            running_server,
            "--json",
            "--dry-run",
            "triage",
            "stale-human",
            "--days",
            "3",
            "--limit",
            "20",
        )
    )

    assert out["count"] == 0
    assert out["tasks"] == []
    assert out["query"]["limit"] == 20
    assert out["query"]["status"] == "open,in_progress,blocked,deferred"
    assert "updated_before" in out["query"]
    assert any("--updated-before" in cmd for cmd in out["commands"])


def test_grnsw_backup_create_writes_snapshot(tmp_path: Path, running_server):
    env = running_server
    created = json_stdout(run_grns(env, "create", "Backup seed", "-t", "task", "-p", "1", "--json"))

    backup_dir = tmp_path / "backups"
    out = json_out(run_grnsw(env, "--json", "backup", "create", "--dir", str(backup_dir)))

    backup_file = Path(out["backup_file"])
    assert backup_file.exists()
    content = backup_file.read_text(encoding="utf-8")
    assert created["id"] in content


def test_grnsw_backup_restore_requires_yes(running_server, tmp_path: Path):
    missing = tmp_path / "missing.ndjson"
    proc = run_grnsw(
        running_server,
        "backup",
        "restore",
        "--file",
        str(missing),
        "--db",
        str(tmp_path / "restore.db"),
        check=False,
    )

    assert proc.returncode != 0
    assert "requires --yes" in proc.stderr
