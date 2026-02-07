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
    assert {r["title"] for r in results} == {"Fix auth bug"}


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

    all_results = json_stdout(run_grns(env, "list", "--json"))
    all_ids = {item["id"] for item in all_results}
    assert len(all_ids) >= 2

    page1 = json_stdout(run_grns(env, "list", "--limit", "1", "--json"))
    assert len(page1) == 1
    page1_id = page1[0]["id"]

    page2 = json_stdout(run_grns(env, "list", "--limit", "1", "--offset", "1", "--json"))
    assert len(page2) == 1
    page2_id = page2[0]["id"]

    assert page1_id in all_ids
    assert page2_id in all_ids
    assert page1_id != page2_id


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
