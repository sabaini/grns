#!/usr/bin/env python3
"""grnsw: workflow companion CLI for grns.

This helper codifies common workflow patterns (epic/phase/milestone scaffolding,
human gates, checkpoints, triage, and backups) while delegating all stateful
operations to the `grns` CLI.
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import os
import pathlib
import re
import shlex
import shutil
import subprocess
import sys
import time
from dataclasses import dataclass, field
from typing import Any


ACTIVE_STATUSES = "open,in_progress,blocked,deferred"
VALID_TASK_TYPES = {"bug", "feature", "task", "epic", "chore"}
VALID_TASK_STATUSES = {"open", "in_progress", "blocked", "deferred", "closed", "pinned", "tombstone"}
VALID_HUMAN_GATE_KINDS = {"decision", "spec", "approval", "other"}
VALID_WORKER_OUTCOMES = {"done", "blocked", "needs_human", "failed", "deferred"}
SHA40_RE = re.compile(r"^[0-9a-f]{40}$")


class RunnerError(RuntimeError):
    """Raised when an external command fails or returns invalid output."""


@dataclass
class Runner:
    grns_bin: str
    json_output: bool
    dry_run: bool
    verbose: bool
    base_env: dict[str, str] = field(default_factory=lambda: dict(os.environ))
    commands: list[str] = field(default_factory=list)
    _placeholder_counter: int = 0

    def _format_command(self, cmd: list[str], env_overrides: dict[str, str] | None = None) -> str:
        rendered = shlex.join(cmd)
        if env_overrides:
            env_part = " ".join(f"{key}={shlex.quote(value)}" for key, value in sorted(env_overrides.items()))
            return f"{env_part} {rendered}"
        return rendered

    def _log(self, message: str) -> None:
        if self.verbose or self.dry_run:
            print(message, file=sys.stderr)

    def run(
        self,
        cmd: list[str],
        *,
        check: bool = True,
        env_overrides: dict[str, str] | None = None,
    ) -> subprocess.CompletedProcess[str]:
        rendered = self._format_command(cmd, env_overrides)
        self.commands.append(rendered)
        self._log(f"+ {rendered}")

        if self.dry_run:
            return subprocess.CompletedProcess(cmd, 0, "", "")

        env = dict(self.base_env)
        if env_overrides:
            env.update(env_overrides)

        try:
            proc = subprocess.run(
                cmd,
                text=True,
                capture_output=True,
                env=env,
                check=False,
            )
        except FileNotFoundError as exc:
            raise RunnerError(f"command not found: {cmd[0]}") from exc

        if check and proc.returncode != 0:
            stderr = (proc.stderr or "").strip()
            stdout = (proc.stdout or "").strip()
            parts = [f"command failed ({proc.returncode}): {rendered}"]
            if stdout:
                parts.append(f"stdout: {stdout}")
            if stderr:
                parts.append(f"stderr: {stderr}")
            raise RunnerError("\n".join(parts))

        return proc

    def run_json(
        self,
        cmd: list[str],
        *,
        check: bool = True,
        env_overrides: dict[str, str] | None = None,
    ) -> Any:
        proc = self.run(cmd, check=check, env_overrides=env_overrides)
        if self.dry_run:
            return {}

        text = (proc.stdout or "").strip()
        if text == "":
            return {}

        try:
            return json.loads(text)
        except json.JSONDecodeError as exc:
            raise RunnerError(f"expected JSON output but got: {text[:200]}") from exc

    def next_placeholder(self, prefix: str = "task") -> str:
        self._placeholder_counter += 1
        return f"<{prefix}-{self._placeholder_counter}>"


def split_csv(values: list[str] | None) -> list[str]:
    out: list[str] = []
    for value in values or []:
        for part in value.split(","):
            item = part.strip()
            if item:
                out.append(item)
    return out


def merge_labels(*groups: list[str]) -> list[str]:
    seen: set[str] = set()
    out: list[str] = []
    for group in groups:
        for label in group:
            normalized = label.strip().lower()
            if not normalized:
                continue
            if normalized in seen:
                continue
            seen.add(normalized)
            out.append(normalized)
    return out


def utc_timestamp() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")


def create_task(
    runner: Runner,
    *,
    title: str,
    task_type: str | None = None,
    priority: int | None = None,
    description: str | None = None,
    spec_id: str | None = None,
    parent_id: str | None = None,
    assignee: str | None = None,
    notes: str | None = None,
    design: str | None = None,
    acceptance: str | None = None,
    source_repo: str | None = None,
    labels: list[str] | None = None,
    custom: dict[str, str] | None = None,
) -> tuple[str, dict[str, Any]]:
    cmd = [runner.grns_bin, "create", title]
    if task_type:
        cmd += ["-t", task_type]
    if priority is not None:
        cmd += ["-p", str(priority)]
    if description:
        cmd += ["-d", description]
    if spec_id:
        cmd += ["--spec-id", spec_id]
    if parent_id:
        cmd += ["--parent", parent_id]
    if assignee:
        cmd += ["--assignee", assignee]
    if notes:
        cmd += ["--notes", notes]
    if design:
        cmd += ["--design", design]
    if acceptance:
        cmd += ["--acceptance", acceptance]
    if source_repo:
        cmd += ["--source-repo", source_repo]
    for label in labels or []:
        cmd += ["--label", label]
    for key, value in (custom or {}).items():
        cmd += ["--custom", f"{key}={value}"]
    cmd.append("--json")

    data = runner.run_json(cmd)
    task_id = runner.next_placeholder("created-id") if runner.dry_run else str(data.get("id", "")).strip()
    if task_id == "":
        raise RunnerError("create did not return task id")
    return task_id, data if isinstance(data, dict) else {}


def add_dependency(runner: Runner, *, blocked_id: str, blocker_id: str) -> dict[str, Any]:
    cmd = [runner.grns_bin, "dep", "add", blocked_id, blocker_id, "--json"]
    data = runner.run_json(cmd)
    if runner.dry_run:
        return {"child_id": blocked_id, "parent_id": blocker_id, "type": "blocks"}
    if not isinstance(data, dict):
        raise RunnerError("dependency add returned unexpected payload")
    return data


def update_notes(runner: Runner, *, task_id: str, notes: str) -> dict[str, Any]:
    cmd = [runner.grns_bin, "update", task_id, "--notes", notes, "--json"]
    data = runner.run_json(cmd)
    if runner.dry_run:
        return {"id": task_id, "notes": notes}
    if not isinstance(data, dict):
        raise RunnerError("update returned unexpected payload")
    return data


def update_status(runner: Runner, *, task_id: str, status: str) -> dict[str, Any]:
    cmd = [runner.grns_bin, "update", task_id, "--status", status, "--json"]
    data = runner.run_json(cmd)
    if runner.dry_run:
        return {"id": task_id, "status": status}
    if not isinstance(data, dict):
        raise RunnerError("status update returned unexpected payload")
    return data


def close_task(runner: Runner, *, task_id: str, commit_sha: str | None, commit_repo: str | None) -> dict[str, Any]:
    cmd = [runner.grns_bin, "close", task_id]
    if commit_sha:
        cmd += ["--commit", commit_sha]
    if commit_repo:
        cmd += ["--repo", commit_repo]
    cmd.append("--json")
    data = runner.run_json(cmd)
    if runner.dry_run:
        result: dict[str, Any] = {"ids": [task_id]}
        if commit_sha:
            result["commit"] = commit_sha
        return result
    if not isinstance(data, dict):
        raise RunnerError("close returned unexpected payload")
    return data


def show_task(runner: Runner, task_id: str) -> dict[str, Any]:
    data = runner.run_json([runner.grns_bin, "show", task_id, "--json"])
    if runner.dry_run:
        return {"id": task_id, "status": "open", "labels": [], "deps": []}
    if not isinstance(data, dict):
        raise RunnerError(f"show returned unexpected payload for {task_id}")
    return data


def list_tasks(
    runner: Runner,
    *,
    label: str | None = None,
    status: str | None = None,
    updated_before: str | None = None,
    limit: int | None = None,
) -> list[dict[str, Any]]:
    cmd = [runner.grns_bin, "list", "--json"]
    if label:
        cmd += ["--label", label]
    if status:
        cmd += ["--status", status]
    if updated_before:
        cmd += ["--updated-before", updated_before]
    if limit is not None and limit > 0:
        cmd += ["--limit", str(limit)]

    data = runner.run_json(cmd)
    if runner.dry_run:
        return []
    if not isinstance(data, list):
        raise RunnerError("list returned unexpected payload")
    return [item for item in data if isinstance(item, dict)]


def print_task_lines(tasks: list[dict[str, Any]]) -> None:
    if not tasks:
        print("no tasks")
        return
    for task in tasks:
        task_id = task.get("id", "")
        status = task.get("status", "")
        priority = task.get("priority", "")
        title = task.get("title", "")
        print(f"{task_id} [{status}] p{priority} {title}")


def maybe_with_commands(runner: Runner, payload: dict[str, Any]) -> dict[str, Any]:
    if runner.verbose or runner.dry_run:
        result = dict(payload)
        result["commands"] = list(runner.commands)
        return result
    return payload


def default_pi_bin() -> str:
    override = os.environ.get("GRNSW_PI_BIN", "").strip()
    if override:
        return override

    found = shutil.which("pi")
    if found:
        return found

    return "pi"


def default_work_task_template() -> str:
    override = os.environ.get("GRNSW_WORK_TASK_TEMPLATE", "").strip()
    if override:
        return override
    return ".pi/prompts/work-task.md"


def parse_embedded_json_object(text: str) -> dict[str, Any]:
    stripped = text.strip()
    if stripped == "":
        raise RunnerError("pi returned empty output; expected JSON object")

    try:
        payload = json.loads(stripped)
        if isinstance(payload, dict):
            return payload
    except json.JSONDecodeError:
        pass

    fence = re.search(r"```(?:json)?\s*(\{[\s\S]*?\})\s*```", text, re.IGNORECASE)
    if fence:
        candidate = fence.group(1)
        try:
            payload = json.loads(candidate)
            if isinstance(payload, dict):
                return payload
        except json.JSONDecodeError:
            pass

    decoder = json.JSONDecoder()
    for idx, ch in enumerate(text):
        if ch != "{":
            continue
        try:
            payload, _end = decoder.raw_decode(text[idx:])
        except json.JSONDecodeError:
            continue
        if isinstance(payload, dict):
            return payload

    snippet = stripped.replace("\n", " ")[:200]
    raise RunnerError(f"could not parse JSON object from pi output: {snippet}")


def normalize_priority(value: Any, *, default: int, field_name: str) -> int:
    if value is None:
        return default
    if isinstance(value, bool):
        raise RunnerError(f"{field_name} must be an integer between 0 and 4")
    try:
        parsed = int(value)
    except (TypeError, ValueError) as exc:
        raise RunnerError(f"{field_name} must be an integer between 0 and 4") from exc
    if parsed < 0 or parsed > 4:
        raise RunnerError(f"{field_name} must be an integer between 0 and 4")
    return parsed


def normalize_worker_payload(payload: dict[str, Any], task_id: str) -> dict[str, Any]:
    task_value = str(payload.get("task_id", "")).strip()
    if task_value and task_value != task_id:
        raise RunnerError(f"worker payload task_id mismatch: expected {task_id}, got {task_value}")

    outcome = str(payload.get("outcome", "done")).strip().lower()
    if outcome == "":
        outcome = "done"
    if outcome not in VALID_WORKER_OUTCOMES:
        raise RunnerError(f"invalid worker outcome: {outcome}")

    status = str(payload.get("status", "")).strip().lower()
    if status == "":
        if outcome == "done":
            status = "closed"
        elif outcome in {"needs_human", "blocked", "failed"}:
            status = "blocked"
        elif outcome == "deferred":
            status = "deferred"
        else:
            status = "in_progress"

    if status not in VALID_TASK_STATUSES:
        raise RunnerError(f"invalid worker status: {status}")

    summary = str(payload.get("summary", "")).strip()
    if summary == "":
        raise RunnerError("worker payload summary is required")

    notes = str(payload.get("notes", "")).strip()
    if notes == "":
        notes = f"[{utc_timestamp()}] Worker outcome: {outcome}. {summary}"

    commit_sha = str(payload.get("commit_sha", "")).strip().lower() or None
    commit_repo = str(payload.get("commit_repo", "")).strip() or None

    commit_obj = payload.get("commit")
    if isinstance(commit_obj, dict):
        if commit_sha is None:
            commit_sha = str(commit_obj.get("sha", "")).strip().lower() or None
        if commit_repo is None:
            commit_repo = str(commit_obj.get("repo", "")).strip() or None

    if commit_sha and not SHA40_RE.match(commit_sha):
        raise RunnerError("commit_sha must be a 40-character lowercase hex SHA")

    followups_raw = payload.get("followups", [])
    followups: list[dict[str, Any]] = []
    if followups_raw is None:
        followups_raw = []
    if not isinstance(followups_raw, list):
        raise RunnerError("followups must be an array when provided")

    for index, item in enumerate(followups_raw):
        if not isinstance(item, dict):
            raise RunnerError(f"followups[{index}] must be an object")

        title = str(item.get("title", "")).strip()
        if title == "":
            raise RunnerError(f"followups[{index}].title is required")

        item_type = str(item.get("type", "task")).strip().lower()
        if item_type not in VALID_TASK_TYPES:
            raise RunnerError(f"followups[{index}].type is invalid: {item_type}")

        priority = normalize_priority(item.get("priority"), default=2, field_name=f"followups[{index}].priority")
        description = str(item.get("description", "")).strip() or None

        labels_raw = item.get("labels", [])
        labels: list[str] = []
        if labels_raw is None:
            labels_raw = []
        if not isinstance(labels_raw, list):
            raise RunnerError(f"followups[{index}].labels must be an array")
        for label in labels_raw:
            normalized = str(label).strip().lower()
            if normalized:
                labels.append(normalized)

        followups.append(
            {
                "title": title,
                "type": item_type,
                "priority": priority,
                "description": description,
                "labels": labels,
                "blocks_current": bool(item.get("blocks_current", False)),
            }
        )

    human_gate_raw = payload.get("human_gate")
    human_gate: dict[str, Any] | None = None
    if isinstance(human_gate_raw, dict) and bool(human_gate_raw.get("needed", False)):
        title = str(human_gate_raw.get("title", "")).strip()
        assignee = str(human_gate_raw.get("assignee", "")).strip()
        kind = str(human_gate_raw.get("kind", "decision")).strip().lower() or "decision"
        description = str(human_gate_raw.get("description", "")).strip() or None
        acceptance = str(human_gate_raw.get("acceptance", "")).strip() or None
        priority = normalize_priority(human_gate_raw.get("priority"), default=1, field_name="human_gate.priority")

        if title == "":
            raise RunnerError("human_gate.title is required when human_gate.needed=true")
        if assignee == "":
            raise RunnerError("human_gate.assignee is required when human_gate.needed=true")
        if kind not in VALID_HUMAN_GATE_KINDS:
            raise RunnerError(f"human_gate.kind is invalid: {kind}")

        labels_raw = human_gate_raw.get("labels", [])
        labels: list[str] = []
        if labels_raw is None:
            labels_raw = []
        if not isinstance(labels_raw, list):
            raise RunnerError("human_gate.labels must be an array")
        for label in labels_raw:
            normalized = str(label).strip().lower()
            if normalized:
                labels.append(normalized)

        human_gate = {
            "title": title,
            "assignee": assignee,
            "kind": kind,
            "priority": priority,
            "description": description,
            "acceptance": acceptance,
            "labels": labels,
        }

    return {
        "task_id": task_id,
        "outcome": outcome,
        "status": status,
        "summary": summary,
        "notes": notes,
        "commit_sha": commit_sha,
        "commit_repo": commit_repo,
        "followups": followups,
        "human_gate": human_gate,
    }


def resolve_template_path(raw_path: str) -> str:
    path_value = os.path.expandvars(raw_path).strip()
    expanded = pathlib.Path(path_value).expanduser()
    if not expanded.is_absolute():
        expanded = pathlib.Path.cwd() / expanded
    return str(expanded)


def parse_pi_args(values: list[str]) -> list[str]:
    args: list[str] = []
    for value in values:
        args.extend(shlex.split(value))
    return args


def run_pi_work_task(runner: Runner, args: argparse.Namespace, task_id: str) -> tuple[dict[str, Any], str]:
    template_path = resolve_template_path(args.template)

    cmd = [
        args.pi_bin,
        "--no-session",
        "--no-skills",
        "--no-themes",
        "--no-prompt-templates",
        "--prompt-template",
        template_path,
    ]
    cmd.extend(parse_pi_args(args.pi_arg))
    cmd.extend(["-p", f"/work-task {task_id}"])

    if runner.dry_run:
        runner.run(cmd)
        payload = {
            "task_id": task_id,
            "outcome": "done",
            "status": "closed",
            "summary": "dry-run placeholder worker result",
            "notes": "dry-run placeholder note",
            "followups": [],
        }
        return normalize_worker_payload(payload, task_id), ""

    if not pathlib.Path(template_path).exists():
        raise RunnerError(f"work-task prompt template not found: {template_path}")

    proc = runner.run(cmd)
    raw_text = (proc.stdout or "").strip()
    payload = parse_embedded_json_object(raw_text)
    return normalize_worker_payload(payload, task_id), raw_text


def execute_worker_task(
    runner: Runner,
    *,
    task_id: str,
    pi_bin: str,
    template: str,
    claim: bool,
    repo: str,
    pi_args: list[str],
) -> dict[str, Any]:
    task = show_task(runner, task_id)

    claimed = False
    claim_update: dict[str, Any] | None = None
    if claim and str(task.get("status", "")).strip().lower() != "in_progress":
        claim_update = update_status(runner, task_id=task_id, status="in_progress")
        claimed = True

    worker_args = argparse.Namespace(pi_bin=pi_bin, template=template, pi_arg=pi_args)
    worker_payload, raw_pi_output = run_pi_work_task(runner, worker_args, task_id)

    notes_update = update_notes(runner, task_id=task_id, notes=worker_payload["notes"])

    created_followups: list[dict[str, Any]] = []
    for followup in worker_payload["followups"]:
        followup_id, _resp = create_task(
            runner,
            title=followup["title"],
            task_type=followup["type"],
            priority=followup["priority"],
            description=followup["description"],
            labels=followup["labels"],
            custom={"created_by": "grnsw-worker", "source_task": task_id},
        )
        deps_added: list[dict[str, Any]] = []
        if followup["blocks_current"]:
            deps_added.append(add_dependency(runner, blocked_id=task_id, blocker_id=followup_id))

        created_followups.append(
            {
                "id": followup_id,
                "title": followup["title"],
                "blocks_current": followup["blocks_current"],
                "deps_added": deps_added,
            }
        )

    human_gate_created: dict[str, Any] | None = None
    human_gate = worker_payload["human_gate"]
    if isinstance(human_gate, dict):
        labels = merge_labels(["human-input", human_gate["kind"]], human_gate["labels"])
        gate_id, _gate_resp = create_task(
            runner,
            title=human_gate["title"],
            task_type="task",
            priority=human_gate["priority"],
            assignee=human_gate["assignee"],
            description=human_gate["description"],
            acceptance=human_gate["acceptance"] or default_human_acceptance(human_gate["kind"]),
            labels=labels,
            custom={"created_by": "grnsw-worker", "source_task": task_id},
        )
        dep = add_dependency(runner, blocked_id=task_id, blocker_id=gate_id)
        human_gate_created = {
            "id": gate_id,
            "kind": human_gate["kind"],
            "assignee": human_gate["assignee"],
            "deps_added": [dep],
        }

    final_status = worker_payload["status"]
    status_result: dict[str, Any] | None = None
    close_result: dict[str, Any] | None = None

    commit_repo = worker_payload["commit_repo"] or repo

    if final_status == "closed":
        close_result = close_task(
            runner,
            task_id=task_id,
            commit_sha=worker_payload["commit_sha"],
            commit_repo=commit_repo,
        )
    else:
        status_result = update_status(runner, task_id=task_id, status=final_status)

    result: dict[str, Any] = {
        "task_id": task_id,
        "claimed": claimed,
        "claim_update": claim_update,
        "worker_result": worker_payload,
        "notes_update": notes_update,
        "followups_created": created_followups,
        "human_gate_created": human_gate_created,
        "status_result": status_result,
        "close_result": close_result,
        "final_status": final_status,
    }

    if runner.verbose or runner.dry_run:
        result["pi_raw_output"] = raw_pi_output

    return result


def list_ready_tasks(runner: Runner, *, limit: int) -> list[dict[str, Any]]:
    cmd = [runner.grns_bin, "ready", "--json"]
    if limit > 0:
        cmd += ["--limit", str(limit)]

    data = runner.run_json(cmd)
    if runner.dry_run:
        return [{"id": "gr-dry1", "type": "task", "labels": [], "status": "open", "title": "dry-run task"}]
    if not isinstance(data, list):
        raise RunnerError("ready returned unexpected payload")
    return [item for item in data if isinstance(item, dict)]


def summarize_worker_task_result(result: dict[str, Any]) -> dict[str, Any]:
    worker_result = result.get("worker_result") if isinstance(result.get("worker_result"), dict) else {}
    human_gate = result.get("human_gate_created") if isinstance(result.get("human_gate_created"), dict) else None
    summary: dict[str, Any] = {
        "task_id": result.get("task_id"),
        "outcome": worker_result.get("outcome"),
        "final_status": result.get("final_status"),
        "followups_created": len(result.get("followups_created", [])),
        "human_gate_id": human_gate.get("id") if human_gate else None,
    }
    return summary


def handle_worker_run_task(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    result = execute_worker_task(
        runner,
        task_id=args.task_id,
        pi_bin=args.pi_bin,
        template=args.template,
        claim=args.claim,
        repo=args.repo,
        pi_args=args.pi_arg,
    )

    if not runner.json_output:
        print(f"task: {args.task_id}")
        print(f"outcome: {result['worker_result']['outcome']}")
        print(f"final_status: {result['final_status']}")
        if result["close_result"] is not None:
            print("closed: yes")
        if result["followups_created"]:
            print(f"followups_created: {len(result['followups_created'])}")
        if result["human_gate_created"] is not None:
            print(f"human_gate: {result['human_gate_created']['id']}")

    return maybe_with_commands(runner, result)


def handle_worker_loop(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    if args.max_tasks < 0:
        raise RunnerError("--max-tasks must be >= 0")
    if args.max_errors < 1:
        raise RunnerError("--max-errors must be >= 1")
    if args.ready_limit < 0:
        raise RunnerError("--ready-limit must be >= 0")
    if args.interval < 0:
        raise RunnerError("--interval must be >= 0")
    if args.max_idle_cycles < 0:
        raise RunnerError("--max-idle-cycles must be >= 0")

    if runner.dry_run and args.watch and args.max_idle_cycles == 0 and args.max_tasks == 0:
        raise RunnerError("in --dry-run watch mode, set --max-tasks or --max-idle-cycles to avoid infinite loop")

    allowed_types = {item.lower() for item in split_csv(args.type)}
    if not allowed_types:
        allowed_types = {"bug", "feature", "task", "chore"}
    invalid_types = sorted(allowed_types - VALID_TASK_TYPES)
    if invalid_types:
        raise RunnerError(f"invalid --type values: {', '.join(invalid_types)}")

    require_labels = {item.lower() for item in split_csv(args.require_label)}
    exclude_labels = {item.lower() for item in split_csv(args.exclude_label)}

    processed: set[str] = set()
    runs: list[dict[str, Any]] = []
    executed = 0
    errors = 0
    iterations = 0
    idle_cycles = 0
    skipped = {
        "already_processed": 0,
        "type_filtered": 0,
        "require_label_filtered": 0,
        "exclude_label_filtered": 0,
    }

    stopped_reason = ""

    while True:
        iterations += 1

        ready_tasks = list_ready_tasks(runner, limit=args.ready_limit)

        selected: dict[str, Any] | None = None
        for task in ready_tasks:
            task_id = str(task.get("id", "")).strip()
            if task_id == "":
                continue
            if task_id in processed:
                skipped["already_processed"] += 1
                continue

            task_type = str(task.get("type", "")).strip().lower()
            if task_type and task_type not in allowed_types:
                skipped["type_filtered"] += 1
                continue

            raw_labels = task.get("labels", [])
            labels = {str(label).strip().lower() for label in raw_labels if str(label).strip()}

            if require_labels and labels.isdisjoint(require_labels):
                skipped["require_label_filtered"] += 1
                continue

            if exclude_labels and not labels.isdisjoint(exclude_labels):
                skipped["exclude_label_filtered"] += 1
                continue

            selected = task
            break

        if selected is None:
            idle_cycles += 1
            no_work_reason = "no_ready_tasks" if not ready_tasks else "no_eligible_ready_tasks"

            if not args.watch:
                stopped_reason = no_work_reason
                break

            if args.max_idle_cycles > 0 and idle_cycles >= args.max_idle_cycles:
                stopped_reason = "max_idle_cycles"
                break

            if args.interval > 0 and not runner.dry_run:
                time.sleep(args.interval)
            continue

        idle_cycles = 0

        task_id = str(selected.get("id", "")).strip()
        processed.add(task_id)

        try:
            task_result = execute_worker_task(
                runner,
                task_id=task_id,
                pi_bin=args.pi_bin,
                template=args.template,
                claim=args.claim,
                repo=args.repo,
                pi_args=args.pi_arg,
            )
            executed += 1
            summary = summarize_worker_task_result(task_result)
            if runner.verbose or runner.dry_run:
                summary["details"] = task_result
            runs.append(summary)
        except RunnerError as exc:
            errors += 1
            runs.append({"task_id": task_id, "error": str(exc)})
            if errors >= args.max_errors:
                stopped_reason = "max_errors"
                break
            if not args.continue_on_error:
                stopped_reason = "error"
                break

        if args.max_tasks > 0 and executed >= args.max_tasks:
            stopped_reason = "max_tasks"
            break

    if stopped_reason == "":
        stopped_reason = "complete"

    result: dict[str, Any] = {
        "executed": executed,
        "errors": errors,
        "iterations": iterations,
        "idle_cycles": idle_cycles,
        "watch": bool(args.watch),
        "interval_seconds": float(args.interval),
        "max_idle_cycles": int(args.max_idle_cycles),
        "stopped_reason": stopped_reason,
        "processed_task_ids": list(processed),
        "run_results": runs,
        "filters": {
            "types": sorted(allowed_types),
            "require_labels": sorted(require_labels),
            "exclude_labels": sorted(exclude_labels),
        },
        "skipped": skipped,
    }

    if not runner.json_output:
        print(f"executed: {executed}")
        print(f"errors: {errors}")
        print(f"iterations: {iterations}")
        print(f"idle_cycles: {idle_cycles}")
        print(f"stopped_reason: {stopped_reason}")
        if runs:
            print("runs:")
            for run in runs:
                if "error" in run:
                    print(f"- {run.get('task_id')}: error: {run['error']}")
                    continue
                print(
                    f"- {run.get('task_id')}: {run.get('outcome')} -> {run.get('final_status')} "
                    f"(followups={run.get('followups_created')}, human_gate={run.get('human_gate_id')})"
                )

    return maybe_with_commands(runner, result)


def handle_doctor(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    checks: list[dict[str, Any]] = []

    if os.sep in runner.grns_bin:
        grns_path = str(pathlib.Path(os.path.expandvars(runner.grns_bin)).expanduser())
        grns_ok = pathlib.Path(grns_path).exists()
        grns_detail = grns_path
    else:
        resolved = shutil.which(runner.grns_bin)
        grns_ok = bool(resolved)
        grns_detail = resolved or "not found in PATH"

    checks.append(
        {
            "name": "grns",
            "ok": grns_ok,
            "detail": grns_detail,
        }
    )

    jq_path = shutil.which("jq")
    checks.append({"name": "jq", "ok": bool(jq_path), "detail": jq_path or "not found in PATH"})

    info: dict[str, Any] | None = None
    info_error: str | None = None
    if checks[0]["ok"]:
        try:
            payload = runner.run_json([runner.grns_bin, "info", "--json"])
            if isinstance(payload, dict):
                info = payload
            else:
                info_error = "grns info returned non-object JSON"
        except RunnerError as exc:
            info_error = str(exc)

    ok = all(bool(c.get("ok")) for c in checks) and info_error is None

    result: dict[str, Any] = {
        "ok": ok,
        "checks": checks,
    }
    if info is not None:
        result["info"] = info
    if info_error:
        result["info_error"] = info_error

    if not runner.json_output:
        print("doctor:")
        for check in checks:
            status = "ok" if check["ok"] else "fail"
            print(f"- {check['name']}: {status} ({check['detail']})")
        if info is not None:
            print(f"- db_path: {info.get('db_path', '')}")
            print(f"- project_prefix: {info.get('project_prefix', '')}")
            print(f"- schema_version: {info.get('schema_version', '')}")
            print(f"- total_tasks: {info.get('total_tasks', '')}")
        if info_error:
            print(f"- info_error: {info_error}")

    return maybe_with_commands(runner, result)


def handle_server_restart(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    pkill = runner.run(["pkill", "-f", "grns srv"], check=False)
    info = runner.run_json([runner.grns_bin, "info", "--json"])

    result: dict[str, Any] = {
        "restarted": True,
        "pkill_exit_code": pkill.returncode,
        "info": info if isinstance(info, dict) else {},
    }

    if not runner.json_output:
        print("server restart: ok")
        if isinstance(info, dict):
            print(f"project_prefix: {info.get('project_prefix', '')}")
            print(f"db_path: {info.get('db_path', '')}")

    return maybe_with_commands(runner, result)


def handle_epic_new(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    labels = merge_labels(["epic"], split_csv(args.label))
    created_id, _ = create_task(
        runner,
        title=args.title,
        task_type="epic",
        priority=args.priority,
        spec_id=args.spec_id,
        design=args.design,
        assignee=args.assignee,
        labels=labels,
    )

    result = {
        "created_id": created_id,
        "task_type": "epic",
        "labels_applied": labels,
    }

    if not runner.json_output:
        print(created_id)

    return maybe_with_commands(runner, result)


def handle_phase_add(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    labels = merge_labels(["phase"], split_csv(args.label))
    depends_on = split_csv(args.depends_on)
    created_id, _ = create_task(
        runner,
        title=args.title,
        task_type="task",
        priority=args.priority,
        parent_id=args.epic,
        assignee=args.assignee,
        labels=labels,
    )

    deps_added: list[dict[str, Any]] = []
    for blocker in depends_on:
        deps_added.append(add_dependency(runner, blocked_id=created_id, blocker_id=blocker))

    result = {
        "created_id": created_id,
        "parent_id": args.epic,
        "labels_applied": labels,
        "deps_added": deps_added,
    }

    if not runner.json_output:
        print(created_id)

    return maybe_with_commands(runner, result)


def handle_milestone_add(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    labels = merge_labels(["milestone"], split_csv(args.label))
    depends_on = split_csv(args.depends_on)
    created_id, _ = create_task(
        runner,
        title=args.title,
        task_type="task",
        priority=args.priority,
        parent_id=args.phase,
        assignee=args.assignee,
        labels=labels,
    )

    deps_added: list[dict[str, Any]] = []
    for blocker in depends_on:
        deps_added.append(add_dependency(runner, blocked_id=created_id, blocker_id=blocker))

    result = {
        "created_id": created_id,
        "parent_id": args.phase,
        "labels_applied": labels,
        "deps_added": deps_added,
    }

    if not runner.json_output:
        print(created_id)

    return maybe_with_commands(runner, result)


def build_acceptance_text(args: argparse.Namespace) -> str:
    parts: list[str] = []
    for item in args.acceptance or []:
        value = item.strip()
        if value:
            parts.append(value)

    if args.acceptance_file:
        content = pathlib.Path(args.acceptance_file).expanduser().read_text(encoding="utf-8").strip()
        if content:
            parts.append(content)

    return "\n".join(parts).strip()


def handle_validation_add(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    labels = merge_labels(["validation"], split_csv(args.label))
    acceptance = build_acceptance_text(args)

    created_id, _ = create_task(
        runner,
        title=args.title,
        task_type=args.type,
        priority=args.priority,
        parent_id=args.milestone,
        assignee=args.reviewer,
        acceptance=acceptance if acceptance else None,
        labels=labels,
    )

    dep = add_dependency(runner, blocked_id=created_id, blocker_id=args.milestone)

    result = {
        "created_id": created_id,
        "parent_id": args.milestone,
        "labels_applied": labels,
        "deps_added": [dep],
    }

    if not runner.json_output:
        print(created_id)

    return maybe_with_commands(runner, result)


def handle_discover_add(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    labels = merge_labels(split_csv(args.label))
    created_id, _ = create_task(
        runner,
        title=args.title,
        task_type=args.type,
        priority=args.priority,
        labels=labels,
        custom={"discovered_from": args.from_task},
    )

    dep = add_dependency(runner, blocked_id=args.from_task, blocker_id=created_id)

    result = {
        "created_id": created_id,
        "linked_from": args.from_task,
        "labels_applied": labels,
        "deps_added": [dep],
    }

    if not runner.json_output:
        print(created_id)

    return maybe_with_commands(runner, result)


def default_human_acceptance(kind: str) -> str:
    if kind == "decision":
        return "Decision recorded with rationale and chosen option"
    if kind == "spec":
        return "Requested spec is created, reviewed, and linked"
    if kind == "approval":
        return "Approval decision recorded with approver and rationale"
    return "Required human input is recorded and linked"


def handle_gate_human(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    labels = merge_labels(["human-input", args.kind], split_csv(args.label))
    acceptance = args.acceptance or default_human_acceptance(args.kind)

    created_id, _ = create_task(
        runner,
        title=args.title,
        task_type="task",
        priority=args.priority,
        assignee=args.assignee,
        description=args.description,
        acceptance=acceptance,
        labels=labels,
    )

    dep = add_dependency(runner, blocked_id=args.agent, blocker_id=created_id)

    result = {
        "human_task_id": created_id,
        "agent_task_id": args.agent,
        "labels_applied": labels,
        "deps_added": [dep],
    }

    if not runner.json_output:
        print(created_id)

    return maybe_with_commands(runner, result)


def handle_checkpoint_set(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    note = (
        f"[{utc_timestamp()}] Stopped at: {args.stopped_at}. "
        f"Tried: {args.tried}. Next: {args.next_step}."
    )
    update = update_notes(runner, task_id=args.task, notes=note)

    result = {
        "task_id": args.task,
        "note": note,
        "updated": bool(update),
    }

    if not runner.json_output:
        print(f"updated notes for {args.task}")

    return maybe_with_commands(runner, result)


def handle_checkpoint_attach(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    cmd = [
        runner.grns_bin,
        "attach",
        "add-link",
        args.task,
        "--kind",
        args.kind,
        "--repo-path",
        args.repo_path,
        "--title",
        args.title,
        "--json",
    ]
    data = runner.run_json(cmd)
    attachment_id = runner.next_placeholder("attachment") if runner.dry_run else str(data.get("id", "")).strip()

    result = {
        "task_id": args.task,
        "attachment_id": attachment_id,
        "repo_path": args.repo_path,
        "kind": args.kind,
    }

    if not runner.json_output:
        print(attachment_id)

    return maybe_with_commands(runner, result)


def handle_triage_human(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    tasks = list_tasks(
        runner,
        label="human-input",
        status=args.status,
        limit=args.limit,
    )

    result = {
        "query": {"label": "human-input", "status": args.status, "limit": args.limit},
        "count": len(tasks),
        "tasks": tasks,
    }

    if not runner.json_output:
        print_task_lines(tasks)

    return maybe_with_commands(runner, result)


def handle_triage_stale_human(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    cutoff = (dt.datetime.now(dt.timezone.utc) - dt.timedelta(days=args.days)).date().isoformat()
    tasks = list_tasks(
        runner,
        label="human-input",
        status=args.status,
        updated_before=cutoff,
        limit=args.limit,
    )

    result = {
        "query": {
            "label": "human-input",
            "status": args.status,
            "updated_before": cutoff,
            "limit": args.limit,
        },
        "count": len(tasks),
        "tasks": tasks,
    }

    if not runner.json_output:
        print_task_lines(tasks)

    return maybe_with_commands(runner, result)


def handle_triage_validation(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    tasks = list_tasks(
        runner,
        label="validation",
        status=args.status,
        limit=args.limit,
    )

    result = {
        "query": {"label": "validation", "status": args.status, "limit": args.limit},
        "count": len(tasks),
        "tasks": tasks,
    }

    if not runner.json_output:
        print_task_lines(tasks)

    return maybe_with_commands(runner, result)


def handle_backup_create(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    backup_dir = pathlib.Path(os.path.expandvars(args.dir)).expanduser()
    filename = f"tasks-{dt.date.today().isoformat()}.ndjson"
    output_path = backup_dir / filename

    if not runner.dry_run:
        backup_dir.mkdir(parents=True, exist_ok=True)

    runner.run([runner.grns_bin, "export", "-o", str(output_path)])

    result = {
        "backup_file": str(output_path),
    }

    if not runner.json_output:
        print(str(output_path))

    return maybe_with_commands(runner, result)


def handle_backup_restore(runner: Runner, args: argparse.Namespace) -> dict[str, Any]:
    if not args.yes and not runner.dry_run:
        raise RunnerError("backup restore requires --yes (or use --dry-run)")

    import_file = str(pathlib.Path(os.path.expandvars(args.file)).expanduser())
    env_overrides: dict[str, str] | None = None

    if args.db:
        env_overrides = {"GRNS_DB": str(pathlib.Path(os.path.expandvars(args.db)).expanduser())}
        runner.run(["pkill", "-f", "grns srv"], check=False)

    import_cmd = [runner.grns_bin, "import", "-i", import_file]
    if args.stream:
        import_cmd.append("--stream")

    proc = runner.run(import_cmd, env_overrides=env_overrides)

    info_payload: dict[str, Any] | None = None
    if args.db:
        payload = runner.run_json([runner.grns_bin, "info", "--json"], env_overrides=env_overrides)
        if isinstance(payload, dict):
            info_payload = payload

    raw_output = "" if runner.dry_run else (proc.stdout or "").strip()
    result = {
        "restored_from": import_file,
        "db": env_overrides.get("GRNS_DB") if env_overrides else os.environ.get("GRNS_DB", ""),
        "stream": bool(args.stream),
    }
    if raw_output:
        result["import_output"] = raw_output
    if info_payload is not None:
        result["info"] = info_payload

    if not runner.json_output:
        print(f"restored from: {import_file}")
        if args.db:
            print(f"db: {env_overrides['GRNS_DB']}")

    return maybe_with_commands(runner, result)


def default_grns_bin() -> str:
    override = os.environ.get("GRNSW_GRNS_BIN", "").strip()
    if override:
        return override

    found = shutil.which("grns")
    if found:
        return found

    local = pathlib.Path.cwd() / "bin" / "grns"
    if local.exists():
        return str(local)

    return "grns"


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="grnsw",
        description="Workflow companion helper for grns",
    )
    parser.add_argument("--json", action="store_true", help="emit JSON output")
    parser.add_argument("--dry-run", action="store_true", help="print commands without mutating data")
    parser.add_argument("--verbose", action="store_true", help="print executed commands to stderr")
    parser.add_argument(
        "--grns-bin",
        default=default_grns_bin(),
        help="path to grns binary (default: $GRNSW_GRNS_BIN, then PATH grns, then ./bin/grns)",
    )

    sub = parser.add_subparsers(dest="command", required=True)

    doctor = sub.add_parser("doctor", help="verify environment and grns connectivity")
    doctor.set_defaults(func=handle_doctor)

    server = sub.add_parser("server", help="server lifecycle helpers")
    server_sub = server.add_subparsers(dest="server_command", required=True)
    server_restart = server_sub.add_parser("restart", help="restart local grns server process")
    server_restart.set_defaults(func=handle_server_restart)

    epic = sub.add_parser("epic", help="epic helpers")
    epic_sub = epic.add_subparsers(dest="epic_command", required=True)
    epic_new = epic_sub.add_parser("new", help="create an epic task")
    epic_new.add_argument("title")
    epic_new.add_argument("--priority", type=int, default=1)
    epic_new.add_argument("--spec-id")
    epic_new.add_argument("--design")
    epic_new.add_argument("--assignee")
    epic_new.add_argument("--label", action="append", default=[])
    epic_new.set_defaults(func=handle_epic_new)

    phase = sub.add_parser("phase", help="phase helpers")
    phase_sub = phase.add_subparsers(dest="phase_command", required=True)
    phase_add = phase_sub.add_parser("add", help="create a phase task")
    phase_add.add_argument("title")
    phase_add.add_argument("--epic", required=True, help="parent epic id")
    phase_add.add_argument("--priority", type=int, default=1)
    phase_add.add_argument("--assignee")
    phase_add.add_argument("--depends-on", action="append", default=[])
    phase_add.add_argument("--label", action="append", default=[])
    phase_add.set_defaults(func=handle_phase_add)

    milestone = sub.add_parser("milestone", help="milestone helpers")
    milestone_sub = milestone.add_subparsers(dest="milestone_command", required=True)
    milestone_add = milestone_sub.add_parser("add", help="create a milestone task")
    milestone_add.add_argument("title")
    milestone_add.add_argument("--phase", required=True, help="parent phase id")
    milestone_add.add_argument("--priority", type=int, default=1)
    milestone_add.add_argument("--assignee")
    milestone_add.add_argument("--depends-on", action="append", default=[])
    milestone_add.add_argument("--label", action="append", default=[])
    milestone_add.set_defaults(func=handle_milestone_add)

    validation = sub.add_parser("validation", help="validation helpers")
    validation_sub = validation.add_subparsers(dest="validation_command", required=True)
    validation_add = validation_sub.add_parser("add", help="create validation task linked to milestone")
    validation_add.add_argument("title")
    validation_add.add_argument("--milestone", required=True)
    validation_add.add_argument("--reviewer", required=True)
    validation_add.add_argument("--type", choices=["task", "chore"], default="chore")
    validation_add.add_argument("--priority", type=int, default=1)
    validation_add.add_argument("--acceptance", action="append", default=[])
    validation_add.add_argument("--acceptance-file")
    validation_add.add_argument("--label", action="append", default=[])
    validation_add.set_defaults(func=handle_validation_add)

    discover = sub.add_parser("discover", help="discovered-work helpers")
    discover_sub = discover.add_subparsers(dest="discover_command", required=True)
    discover_add = discover_sub.add_parser("add", help="create new work and block current task on it")
    discover_add.add_argument("title")
    discover_add.add_argument("--from", dest="from_task", required=True, help="task that discovered this work")
    discover_add.add_argument("--type", choices=["bug", "feature", "task", "epic", "chore"], default="task")
    discover_add.add_argument("--priority", type=int, default=2)
    discover_add.add_argument("--label", action="append", default=[])
    discover_add.set_defaults(func=handle_discover_add)

    gate = sub.add_parser("gate", help="gating helpers")
    gate_sub = gate.add_subparsers(dest="gate_command", required=True)
    gate_human = gate_sub.add_parser("human", help="create human-input gate and block agent task")
    gate_human.add_argument("title")
    gate_human.add_argument("--agent", required=True)
    gate_human.add_argument("--assignee", required=True)
    gate_human.add_argument("--kind", choices=["decision", "spec", "approval", "other"], default="decision")
    gate_human.add_argument("--description")
    gate_human.add_argument("--acceptance")
    gate_human.add_argument("--priority", type=int, default=1)
    gate_human.add_argument("--label", action="append", default=[])
    gate_human.set_defaults(func=handle_gate_human)

    checkpoint = sub.add_parser("checkpoint", help="checkpoint helpers")
    checkpoint_sub = checkpoint.add_subparsers(dest="checkpoint_command", required=True)
    checkpoint_set = checkpoint_sub.add_parser("set", help="write formatted checkpoint note")
    checkpoint_set.add_argument("--task", required=True)
    checkpoint_set.add_argument("--stopped-at", required=True)
    checkpoint_set.add_argument("--tried", required=True)
    checkpoint_set.add_argument("--next", dest="next_step", required=True)
    checkpoint_set.set_defaults(func=handle_checkpoint_set)

    checkpoint_attach = checkpoint_sub.add_parser("attach", help="attach long-form handoff log path")
    checkpoint_attach.add_argument("--task", required=True)
    checkpoint_attach.add_argument("--repo-path", required=True)
    checkpoint_attach.add_argument("--title", default="Handoff log")
    checkpoint_attach.add_argument(
        "--kind",
        choices=["spec", "diagram", "artifact", "diagnostic", "archive", "other"],
        default="diagnostic",
    )
    checkpoint_attach.set_defaults(func=handle_checkpoint_attach)

    triage = sub.add_parser("triage", help="queue queries")
    triage_sub = triage.add_subparsers(dest="triage_command", required=True)

    triage_human = triage_sub.add_parser("human", help="list active human-input tasks")
    triage_human.add_argument("--status", default=ACTIVE_STATUSES)
    triage_human.add_argument("--limit", type=int, default=0)
    triage_human.set_defaults(func=handle_triage_human)

    triage_stale_human = triage_sub.add_parser("stale-human", help="list stale active human-input tasks")
    triage_stale_human.add_argument("--days", type=int, default=2)
    triage_stale_human.add_argument("--status", default=ACTIVE_STATUSES)
    triage_stale_human.add_argument("--limit", type=int, default=50)
    triage_stale_human.set_defaults(func=handle_triage_stale_human)

    triage_validation = triage_sub.add_parser("validation", help="list active validation tasks")
    triage_validation.add_argument("--status", default=ACTIVE_STATUSES)
    triage_validation.add_argument("--limit", type=int, default=0)
    triage_validation.set_defaults(func=handle_triage_validation)

    worker = sub.add_parser("worker", help="task execution worker helpers")
    worker_sub = worker.add_subparsers(dest="worker_command", required=True)

    worker_run_task = worker_sub.add_parser("run-task", help="run one task via pi and apply worker result")
    worker_run_task.add_argument("task_id", help="task id to execute")
    worker_run_task.add_argument(
        "--pi-bin",
        default=default_pi_bin(),
        help="path to pi binary (default: $GRNSW_PI_BIN or PATH pi)",
    )
    worker_run_task.add_argument(
        "--template",
        default=default_work_task_template(),
        help="prompt template path for /work-task",
    )
    worker_run_task.add_argument(
        "--claim",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="set task status to in_progress before running pi",
    )
    worker_run_task.add_argument(
        "--repo",
        default="",
        help="default repo slug for close annotation when worker returns commit_sha",
    )
    worker_run_task.add_argument(
        "--pi-arg",
        action="append",
        default=[],
        help="extra arg(s) passed to pi (repeatable; each value is shell-split)",
    )
    worker_run_task.set_defaults(func=handle_worker_run_task)

    worker_loop = worker_sub.add_parser("loop", help="run ready tasks until queue is empty (or limits hit)")
    worker_loop.add_argument(
        "--pi-bin",
        default=default_pi_bin(),
        help="path to pi binary (default: $GRNSW_PI_BIN or PATH pi)",
    )
    worker_loop.add_argument(
        "--template",
        default=default_work_task_template(),
        help="prompt template path for /work-task",
    )
    worker_loop.add_argument(
        "--claim",
        action=argparse.BooleanOptionalAction,
        default=True,
        help="set each selected task to in_progress before running pi",
    )
    worker_loop.add_argument(
        "--repo",
        default="",
        help="default repo slug for close annotation when worker returns commit_sha",
    )
    worker_loop.add_argument(
        "--pi-arg",
        action="append",
        default=[],
        help="extra arg(s) passed to pi (repeatable; each value is shell-split)",
    )
    worker_loop.add_argument(
        "--max-tasks",
        type=int,
        default=0,
        help="maximum number of tasks to execute (0 = no limit)",
    )
    worker_loop.add_argument(
        "--max-errors",
        type=int,
        default=1,
        help="maximum number of task failures before stopping",
    )
    worker_loop.add_argument(
        "--continue-on-error",
        action=argparse.BooleanOptionalAction,
        default=False,
        help="continue processing after a task failure until max-errors is hit",
    )
    worker_loop.add_argument(
        "--watch",
        action=argparse.BooleanOptionalAction,
        default=False,
        help="keep polling for ready tasks instead of exiting when queue is empty",
    )
    worker_loop.add_argument(
        "--interval",
        type=float,
        default=10.0,
        help="poll interval in seconds when --watch is enabled",
    )
    worker_loop.add_argument(
        "--max-idle-cycles",
        type=int,
        default=0,
        help="stop after N consecutive idle polls in watch mode (0 = no idle limit)",
    )
    worker_loop.add_argument(
        "--ready-limit",
        type=int,
        default=50,
        help="max ready tasks fetched per iteration",
    )
    worker_loop.add_argument(
        "--type",
        action="append",
        default=[],
        help="processable task type(s), comma-separated or repeatable (default: bug,feature,task,chore)",
    )
    worker_loop.add_argument(
        "--require-label",
        action="append",
        default=[],
        help="only process tasks that have at least one of these labels",
    )
    worker_loop.add_argument(
        "--exclude-label",
        action="append",
        default=[],
        help="skip tasks that contain any of these labels",
    )
    worker_loop.set_defaults(func=handle_worker_loop)

    backup = sub.add_parser("backup", help="backup helpers")
    backup_sub = backup.add_subparsers(dest="backup_command", required=True)

    backup_create = backup_sub.add_parser("create", help="export NDJSON snapshot")
    backup_create.add_argument("--dir", default="~/data/grns/backups")
    backup_create.set_defaults(func=handle_backup_create)

    backup_restore = backup_sub.add_parser("restore", help="restore from NDJSON snapshot")
    backup_restore.add_argument("--file", required=True)
    backup_restore.add_argument("--db", help="target GRNS_DB path for restore")
    backup_restore.add_argument("--stream", action=argparse.BooleanOptionalAction, default=True)
    backup_restore.add_argument("--yes", action="store_true", help="confirm destructive restore action")
    backup_restore.set_defaults(func=handle_backup_restore)

    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    runner = Runner(
        grns_bin=args.grns_bin,
        json_output=args.json,
        dry_run=args.dry_run,
        verbose=args.verbose,
    )

    try:
        result = args.func(runner, args)
    except (RunnerError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    if runner.json_output:
        print(json.dumps(result, indent=2, sort_keys=True))

    if args.command == "doctor" and isinstance(result, dict) and not bool(result.get("ok", False)):
        return 1

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
