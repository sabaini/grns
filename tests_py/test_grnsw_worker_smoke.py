import json
import stat
import subprocess
import sys
from pathlib import Path

from tests_py.helpers import json_stdout, run_grns


REPO_ROOT = Path(__file__).resolve().parents[1]
GRNSW = REPO_ROOT / "scripts" / "grnsw.py"


def run_grnsw(
    env: dict[str, str],
    *args: str,
    check: bool = True,
    extra_env: dict[str, str] | None = None,
) -> subprocess.CompletedProcess[str]:
    proc_env = dict(env)
    proc_env["GRNSW_GRNS_BIN"] = env["GRNS_BIN"]
    if extra_env:
        proc_env.update(extra_env)

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


def make_fake_pi_script(tmp_path: Path, payload: dict) -> str:
    script_path = tmp_path / "fake_pi.py"
    script_path.write_text(
        """#!/usr/bin/env python3
import json
import os
import re
import sys

raw_payload = os.environ.get('GRNSW_FAKE_PI_PAYLOAD', '{}')
raw_payload_map = os.environ.get('GRNSW_FAKE_PI_PAYLOAD_MAP', '{}')
payload = json.loads(raw_payload)
payload_map = json.loads(raw_payload_map)

task_id = ''
for idx, arg in enumerate(sys.argv):
    if arg != '-p':
        continue
    if idx + 1 >= len(sys.argv):
        continue
    prompt = sys.argv[idx + 1]
    match = re.search(r"/work-task\\s+(\\S+)", prompt)
    if match:
        task_id = match.group(1)
        break

selected = payload_map.get(task_id, payload)
if not isinstance(selected, dict):
    selected = payload
if task_id and isinstance(selected, dict) and not selected.get('task_id'):
    selected = dict(selected)
    selected['task_id'] = task_id

print(json.dumps(selected))
""",
        encoding="utf-8",
    )
    script_path.chmod(script_path.stat().st_mode | stat.S_IEXEC)
    return str(script_path)


def make_template_file(tmp_path: Path) -> str:
    template = tmp_path / "work-task.md"
    template.write_text(
        """---
description: fake work-task template for tests
---
Echo task $1
""",
        encoding="utf-8",
    )
    return str(template)


def test_worker_run_task_closes_and_creates_followup(running_server, tmp_path: Path):
    env = running_server
    task = json_stdout(run_grns(env, "create", "Worker target", "-t", "task", "-p", "1", "--json"))
    task_id = task["id"]

    fake_payload = {
        "task_id": task_id,
        "outcome": "done",
        "summary": "Implemented auth parser and validated behavior",
        "status": "closed",
        "notes": "Worker complete: tests passed",
        "commit_sha": "",
        "commit_repo": "",
        "followups": [
            {
                "title": "Add regression test for malformed token",
                "type": "task",
                "priority": 2,
                "description": "Cover malformed token edge case",
                "labels": ["auth", "testing"],
                "blocks_current": True,
            }
        ],
        "human_gate": {"needed": False},
    }

    fake_pi = make_fake_pi_script(tmp_path, fake_payload)
    template = make_template_file(tmp_path)

    proc = run_grnsw(
        env,
        "--json",
        "worker",
        "run-task",
        task_id,
        "--pi-bin",
        fake_pi,
        "--template",
        template,
        extra_env={"GRNSW_FAKE_PI_PAYLOAD": json.dumps(fake_payload)},
    )
    out = json_out(proc)

    assert out["task_id"] == task_id
    assert out["final_status"] == "closed"
    assert out["claimed"] is True
    assert len(out["followups_created"]) == 1

    followup_id = out["followups_created"][0]["id"]

    shown = json_stdout(run_grns(env, "show", task_id, "--json"))
    assert shown["status"] == "closed"
    assert any(dep["parent_id"] == followup_id and dep["type"] == "blocks" for dep in shown["deps"])

    followup = json_stdout(run_grns(env, "show", followup_id, "--json"))
    assert followup["title"] == "Add regression test for malformed token"
    assert "auth" in followup["labels"]
    assert followup["custom"]["source_task"] == task_id


def test_worker_run_task_creates_human_gate_and_blocks_task(running_server, tmp_path: Path):
    env = running_server
    task = json_stdout(run_grns(env, "create", "Needs decision", "-t", "task", "-p", "1", "--json"))
    task_id = task["id"]

    fake_payload = {
        "task_id": task_id,
        "outcome": "needs_human",
        "summary": "Need product decision before coding",
        "status": "blocked",
        "notes": "Blocked: awaiting product decision",
        "followups": [],
        "human_gate": {
            "needed": True,
            "title": "Decision: choose token format",
            "assignee": "alice",
            "kind": "decision",
            "description": "Need explicit decision between JWT and opaque tokens",
            "acceptance": "Decision recorded with rationale",
            "priority": 1,
            "labels": ["security"],
        },
    }

    fake_pi = make_fake_pi_script(tmp_path, fake_payload)
    template = make_template_file(tmp_path)

    proc = run_grnsw(
        env,
        "--json",
        "worker",
        "run-task",
        task_id,
        "--pi-bin",
        fake_pi,
        "--template",
        template,
        extra_env={"GRNSW_FAKE_PI_PAYLOAD": json.dumps(fake_payload)},
    )
    out = json_out(proc)

    assert out["final_status"] == "blocked"
    assert out["human_gate_created"] is not None

    gate_id = out["human_gate_created"]["id"]

    shown = json_stdout(run_grns(env, "show", task_id, "--json"))
    assert shown["status"] == "blocked"
    assert any(dep["parent_id"] == gate_id and dep["type"] == "blocks" for dep in shown["deps"])

    gate_task = json_stdout(run_grns(env, "show", gate_id, "--json"))
    assert gate_task["assignee"] == "alice"
    assert "human-input" in gate_task["labels"]
    assert "decision" in gate_task["labels"]
    assert "security" in gate_task["labels"]


def test_worker_loop_processes_ready_queue(running_server, tmp_path: Path):
    env = running_server

    first = json_stdout(run_grns(env, "create", "Worker loop task A", "-t", "task", "-p", "1", "--json"))
    second = json_stdout(run_grns(env, "create", "Worker loop task B", "-t", "task", "-p", "1", "--json"))

    first_id = first["id"]
    second_id = second["id"]

    payload_map = {
        first_id: {
            "task_id": first_id,
            "outcome": "done",
            "summary": "Completed first queued task",
            "status": "closed",
            "notes": "Worker closed first task",
            "followups": [],
            "human_gate": {"needed": False},
        },
        second_id: {
            "task_id": second_id,
            "outcome": "done",
            "summary": "Completed second queued task",
            "status": "closed",
            "notes": "Worker closed second task",
            "followups": [],
            "human_gate": {"needed": False},
        },
    }

    fake_pi = make_fake_pi_script(tmp_path, payload_map[first_id])
    template = make_template_file(tmp_path)

    proc = run_grnsw(
        env,
        "--json",
        "worker",
        "loop",
        "--pi-bin",
        fake_pi,
        "--template",
        template,
        "--max-tasks",
        "10",
        extra_env={
            "GRNSW_FAKE_PI_PAYLOAD": json.dumps(payload_map[first_id]),
            "GRNSW_FAKE_PI_PAYLOAD_MAP": json.dumps(payload_map),
        },
    )
    out = json_out(proc)

    assert out["executed"] == 2
    assert out["errors"] == 0
    assert out["stopped_reason"] == "no_ready_tasks"
    assert len(out["run_results"]) == 2

    first_shown = json_stdout(run_grns(env, "show", first_id, "--json"))
    second_shown = json_stdout(run_grns(env, "show", second_id, "--json"))
    assert first_shown["status"] == "closed"
    assert second_shown["status"] == "closed"


def test_worker_loop_respects_max_tasks_limit(running_server, tmp_path: Path):
    env = running_server

    first = json_stdout(run_grns(env, "create", "Worker loop limited A", "-t", "task", "-p", "1", "--json"))
    second = json_stdout(run_grns(env, "create", "Worker loop limited B", "-t", "task", "-p", "1", "--json"))

    first_id = first["id"]
    second_id = second["id"]

    payload_map = {
        first_id: {
            "task_id": first_id,
            "outcome": "done",
            "summary": "Closed one",
            "status": "closed",
            "notes": "closed by worker",
            "followups": [],
            "human_gate": {"needed": False},
        },
        second_id: {
            "task_id": second_id,
            "outcome": "done",
            "summary": "Closed two",
            "status": "closed",
            "notes": "closed by worker",
            "followups": [],
            "human_gate": {"needed": False},
        },
    }

    fake_pi = make_fake_pi_script(tmp_path, payload_map[first_id])
    template = make_template_file(tmp_path)

    proc = run_grnsw(
        env,
        "--json",
        "worker",
        "loop",
        "--pi-bin",
        fake_pi,
        "--template",
        template,
        "--max-tasks",
        "1",
        extra_env={
            "GRNSW_FAKE_PI_PAYLOAD": json.dumps(payload_map[first_id]),
            "GRNSW_FAKE_PI_PAYLOAD_MAP": json.dumps(payload_map),
        },
    )
    out = json_out(proc)

    assert out["executed"] == 1
    assert out["errors"] == 0
    assert out["stopped_reason"] == "max_tasks"

    statuses = [
        json_stdout(run_grns(env, "show", first_id, "--json"))["status"],
        json_stdout(run_grns(env, "show", second_id, "--json"))["status"],
    ]
    assert statuses.count("closed") == 1


def test_worker_loop_watch_stops_on_max_idle_cycles(running_server):
    env = running_server

    proc = run_grnsw(
        env,
        "--json",
        "worker",
        "loop",
        "--watch",
        "--interval",
        "0",
        "--max-idle-cycles",
        "2",
    )
    out = json_out(proc)

    assert out["watch"] is True
    assert out["executed"] == 0
    assert out["errors"] == 0
    assert out["idle_cycles"] == 2
    assert out["stopped_reason"] == "max_idle_cycles"


def test_worker_loop_dry_run_watch_requires_bound(running_server):
    proc = run_grnsw(
        running_server,
        "--dry-run",
        "worker",
        "loop",
        "--watch",
        check=False,
    )

    assert proc.returncode != 0
    assert "set --max-tasks or --max-idle-cycles" in proc.stderr
