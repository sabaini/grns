"""Tests for stale task filtering.

Migrated from tests/cli_stale_filters.bats.
"""

import datetime
import sqlite3

from tests_py.helpers import json_stdout, run_grns


def test_stale_excludes_closed_unless_status_filter(running_server):
    env = running_server

    open_task = json_stdout(run_grns(env, "create", "Stale open", "-t", "task", "-p", "1", "--json"))
    closed_task = json_stdout(run_grns(env, "create", "Stale closed", "-t", "task", "-p", "1", "--json"))
    run_grns(env, "close", closed_task["id"], "--json")

    # Backdate both tasks to 40 days ago directly in the DB.
    old = (datetime.datetime.now(datetime.UTC) - datetime.timedelta(days=40)).strftime("%Y-%m-%dT%H:%M:%SZ")
    db_path = env["GRNS_DB"]
    conn = sqlite3.connect(db_path)
    conn.execute(
        "UPDATE tasks SET updated_at = ? WHERE id IN (?, ?)",
        (old, open_task["id"], closed_task["id"]),
    )
    conn.execute(
        "UPDATE tasks SET closed_at = ? WHERE id = ?",
        (old, closed_task["id"]),
    )
    conn.commit()
    conn.close()

    # Default stale: should include open task, exclude closed.
    results = json_stdout(run_grns(env, "stale", "--json"))
    result_ids = [r["id"] for r in results]
    assert open_task["id"] in result_ids
    assert closed_task["id"] not in result_ids

    # With explicit status filter: should include closed.
    results = json_stdout(run_grns(env, "stale", "--status", "closed", "--json"))
    result_ids = [r["id"] for r in results]
    assert closed_task["id"] in result_ids
