"""Tests for list command filters.

Migrated from tests/cli_list_filters.bats.
"""

from tests_py.helpers import json_stdout, run_grns


# ---------------------------------------------------------------------------
# Label filters (seeded data)
# ---------------------------------------------------------------------------


def test_label_filter(seeded_server):
    env = seeded_server

    results = json_stdout(run_grns(env, "list", "--label", "bug", "--json"))
    titles = [r["title"] for r in results]
    assert "Fix auth bug" in titles


def test_label_and_filter(seeded_server):
    env = seeded_server

    results = json_stdout(run_grns(env, "list", "--label", "bug,auth", "--json"))
    assert len(results) == 1
    assert results[0]["title"] == "Fix auth bug"


def test_label_any_filter(seeded_server):
    env = seeded_server

    results = json_stdout(run_grns(env, "list", "--label-any", "auth,frontend", "--json"))
    titles = [r["title"] for r in results]
    assert "Fix auth bug" in titles
    assert "Add settings page" in titles


def test_spec_regex_filter(seeded_server):
    env = seeded_server

    results = json_stdout(run_grns(env, "list", "--spec", r"auth\.md", "--json"))
    titles = [r["title"] for r in results]
    assert "Fix auth bug" in titles


# ---------------------------------------------------------------------------
# Pagination
# ---------------------------------------------------------------------------


def test_limit_and_offset(seeded_server):
    env = seeded_server

    page1 = json_stdout(run_grns(env, "list", "--limit", "1", "--json"))
    assert len(page1) == 1
    first_title = page1[0]["title"]
    assert first_title == "Write onboarding docs"

    page2 = json_stdout(run_grns(env, "list", "--limit", "1", "--offset", "1", "--json"))
    assert len(page2) == 1
    assert page2[0]["title"] == "Add settings page"


def test_offset_without_limit(seeded_server):
    env = seeded_server

    results = json_stdout(run_grns(env, "list", "--offset", "1", "--json"))
    assert len(results) == 2


# ---------------------------------------------------------------------------
# Multi-value status filter
# ---------------------------------------------------------------------------


def test_multi_value_status_filter(running_server):
    env = running_server

    open_task = json_stdout(run_grns(env, "create", "Open task", "-t", "task", "-p", "1", "--json"))
    closed_task = json_stdout(run_grns(env, "create", "Closed task", "-t", "task", "-p", "1", "--json"))
    run_grns(env, "close", closed_task["id"], "--json")

    results = json_stdout(run_grns(env, "list", "--status", "open,closed", "--json"))
    result_ids = [r["id"] for r in results]
    assert open_task["id"] in result_ids
    assert closed_task["id"] in result_ids
