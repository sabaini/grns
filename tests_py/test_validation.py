"""Tests for input validation and error paths.

Migrated from tests/cli_validation_errors.bats.
"""

import json
import urllib.error
import urllib.request

import pytest

from tests_py.helpers import api_post, json_stdout, run_grns, run_grns_fail


# ---------------------------------------------------------------------------
# Invalid field values (parametrized)
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("args,error_substr", [
    (["create", "Bad type", "-t", "nope", "--json"], "invalid type"),
    (["create", "--json"], "title is required"),
    (["list", "--priority", "9", "--json"], "priority must be between 0 and 4"),
    (["list", "--spec", "[", "--json"], "invalid spec regex"),
])
def test_cli_rejects_invalid_input(running_server, args, error_substr):
    proc = run_grns_fail(running_server, *args)
    assert proc.returncode != 0
    assert error_substr in proc.stdout + proc.stderr


# ---------------------------------------------------------------------------
# Priority range enforcement
# ---------------------------------------------------------------------------


def test_priority_range_enforced_on_create(running_server):
    proc = run_grns_fail(running_server, "create", "Bad priority", "-t", "task", "-p", "9", "--json")
    assert proc.returncode != 0
    assert "priority must be between 0 and 4" in proc.stdout + proc.stderr


def test_priority_range_enforced_on_update(running_server):
    env = running_server
    created = json_stdout(run_grns(env, "create", "Good priority", "-t", "task", "-p", "1", "--json"))
    task_id = created["id"]

    proc = run_grns_fail(env, "update", task_id, "--priority", "9", "--json")
    assert proc.returncode != 0
    assert "priority must be between 0 and 4" in proc.stdout + proc.stderr


# ---------------------------------------------------------------------------
# Update rejects invalid status
# ---------------------------------------------------------------------------


def test_update_rejects_invalid_status(running_server):
    env = running_server
    created = json_stdout(run_grns(env, "create", "Valid task", "-t", "task", "-p", "1", "--json"))
    task_id = created["id"]

    proc = run_grns_fail(env, "update", task_id, "--status", "nope", "--json")
    assert proc.returncode != 0
    assert "invalid status" in proc.stdout + proc.stderr


# ---------------------------------------------------------------------------
# Invalid IDs
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("cmd,extra_args", [
    ("show", []),
    ("update", ["--status", "open"]),
])
def test_invalid_id_rejected(running_server, cmd, extra_args):
    proc = run_grns_fail(running_server, cmd, "bad-id", *extra_args, "--json")
    assert proc.returncode != 0
    assert "invalid id" in (proc.stdout + proc.stderr).lower()


def test_invalid_id_rejected_on_dep_add(running_server):
    env = running_server
    parent = json_stdout(run_grns(env, "create", "Parent", "-t", "task", "-p", "1", "--json"))

    proc = run_grns_fail(env, "dep", "add", "bad-id", parent["id"], "--json")
    assert proc.returncode != 0
    assert "invalid" in (proc.stdout + proc.stderr).lower()


# ---------------------------------------------------------------------------
# Update requires fields
# ---------------------------------------------------------------------------


def test_update_requires_at_least_one_field(running_server):
    env = running_server
    created = json_stdout(run_grns(env, "create", "No field update", "-t", "task", "-p", "1", "--json"))
    task_id = created["id"]

    proc = run_grns_fail(env, "update", task_id, "--json")
    assert proc.returncode != 0
    assert "no fields to update" in proc.stdout + proc.stderr


# ---------------------------------------------------------------------------
# Conflict and not-found
# ---------------------------------------------------------------------------


def test_duplicate_id_returns_conflict(running_server):
    env = running_server
    run_grns(env, "create", "First", "--id", "gr-ab12", "-t", "task", "-p", "1", "--json")

    proc = run_grns_fail(env, "create", "Second", "--id", "gr-ab12", "-t", "task", "-p", "1", "--json")
    assert proc.returncode != 0
    assert "conflict" in (proc.stdout + proc.stderr).lower()


def test_nonexistent_id_returns_not_found(running_server):
    proc = run_grns_fail(running_server, "show", "gr-zzzz", "--json")
    assert proc.returncode != 0
    assert "not_found" in proc.stdout + proc.stderr


@pytest.mark.parametrize("cmd", ["close", "reopen"])
def test_close_reopen_nonexistent_returns_not_found(running_server, cmd):
    proc = run_grns_fail(running_server, cmd, "gr-zzzz", "--json")
    assert proc.returncode != 0
    assert "not_found" in proc.stdout + proc.stderr


# ---------------------------------------------------------------------------
# All-or-nothing atomicity for close/reopen
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("action,setup_action", [
    ("close", None),
    ("reopen", "close"),
])
def test_mixed_ids_all_or_nothing(running_server, action, setup_action):
    env = running_server
    run_grns(env, "create", "Mixed target", "--id", "gr-mx11", "--json")

    if setup_action:
        run_grns(env, setup_action, "gr-mx11", "--json")

    expected_status = "closed" if setup_action else "open"

    # Action with mixed valid+missing IDs should fail.
    proc = run_grns_fail(env, action, "gr-mx11", "gr-mx99", "--json")
    assert proc.returncode != 0
    assert "not_found" in proc.stdout + proc.stderr

    # Original task should be unchanged.
    shown = json_stdout(run_grns(env, "show", "gr-mx11", "--json"))
    assert shown["status"] == expected_status


# ---------------------------------------------------------------------------
# API-level validation
# ---------------------------------------------------------------------------


def test_api_rejects_malformed_query_params(running_server):
    env = running_server
    url = env["GRNS_API_URL"] + "/v1/tasks?offset=-1"

    with pytest.raises(urllib.error.HTTPError) as exc_info:
        urllib.request.urlopen(url)

    assert exc_info.value.code == 400
    body = json.loads(exc_info.value.read().decode("utf-8"))
    assert "offset" in body.get("error", "")
