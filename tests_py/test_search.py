"""Tests for FTS search functionality.

Migrated from tests/cli_search.bats.
"""

import pytest

from tests_py.helpers import json_stdout, run_grns, run_grns_fail


def test_search_finds_by_title(running_server):
    env = running_server

    auth = json_stdout(run_grns(env, "create", "Authentication module", "-t", "task", "-p", "2",
                                "-d", "Implement OAuth login", "--json"))
    json_stdout(run_grns(env, "create", "Caching layer", "-t", "feature", "-p", "2",
                         "-d", "Redis integration", "--json"))

    results = json_stdout(run_grns(env, "list", "--search", "authentication", "--json"))
    assert len(results) == 1
    assert {item["id"] for item in results} == {auth["id"]}
    assert {item["title"] for item in results} == {"Authentication module"}


def test_search_finds_by_description(running_server):
    env = running_server

    auth = json_stdout(run_grns(env, "create", "Authentication module", "-t", "task", "-p", "2",
                                "-d", "Implement OAuth login", "--json"))
    cache = json_stdout(run_grns(env, "create", "Caching layer", "-t", "feature", "-p", "2",
                                 "-d", "Redis integration", "--json"))

    results = json_stdout(run_grns(env, "list", "--search", "OAuth", "--json"))
    assert len(results) == 1
    result_ids = {item["id"] for item in results}
    assert auth["id"] in result_ids
    assert cache["id"] not in result_ids


def test_search_no_results(running_server):
    env = running_server
    # Ensure at least one task exists.
    run_grns(env, "create", "Some task", "--json")

    results = json_stdout(run_grns(env, "list", "--search", "nonexistent", "--json"))
    assert len(results) == 0


def test_search_composes_with_status_filter(running_server):
    env = running_server

    closed_task = json_stdout(run_grns(env, "create", "Searchable open", "-t", "task", "-p", "2", "--json"))
    run_grns(env, "close", closed_task["id"], "--json")

    open_task = json_stdout(run_grns(env, "create", "Searchable still open", "-t", "task", "-p", "2", "--json"))

    results = json_stdout(run_grns(env, "list", "--search", "searchable", "--status", "open", "--json"))
    assert len(results) == 1
    result_ids = {item["id"] for item in results}
    assert open_task["id"] in result_ids
    assert all(item["status"] == "open" for item in results)


def test_search_rejects_malformed_query(running_server):
    proc = run_grns_fail(running_server, "list", "--search", '"', "--json")
    assert proc.returncode != 0
    assert "invalid search query" in proc.stdout + proc.stderr
