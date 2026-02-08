"""Extended property-based tests for grns.

Covers: timestamp invariants, close/reopen state machine, custom fields,
dependencies, pagination, filter composition, labels, description roundtrip,
list ordering, batch get order, ID format, and import dedupe modes.
"""

import json
import re
import string
import time

import pytest
from hypothesis import given, settings, assume, HealthCheck
from hypothesis import strategies as st

from tests_py.helpers import api_get, api_patch, api_post, json_stdout, run_grns, run_grns_fail
from tests_py.strategies import (
    VALID_TYPES,
    custom_field_maps,
    mixed_case_label_lists,
    printable_titles,
    valid_labels,
    valid_label_lists,
    valid_priorities,
    valid_types,
)

pytestmark = pytest.mark.hypothesis

SETTINGS = settings(
    max_examples=30,
    deadline=None,
    suppress_health_check=[HealthCheck.function_scoped_fixture],
)

ID_PATTERN = re.compile(r"^[a-z]{2}-[0-9a-z]{4}$")


class _Counter:
    """Monotonically-increasing counter for unique IDs across hypothesis examples."""

    def __init__(self):
        self._n = 0

    def next(self) -> str:
        chars = string.digits + string.ascii_lowercase
        n = self._n
        self._n += 1
        c1 = chars[n // 36 % 36]
        c2 = chars[n % 36]
        return f"{c1}{c2}"


_counter = _Counter()


# ---------------------------------------------------------------------------
# 1. Timestamp Invariants
# ---------------------------------------------------------------------------


@SETTINGS
@given(new_priority=valid_priorities(), new_type=valid_types())
def test_created_at_immutable_across_updates(running_server, new_priority, new_type):
    """created_at never changes regardless of how many updates are applied."""
    env = running_server
    created = api_post(env, "/v1/projects/gr/tasks", {"title": "ts immutable"})
    original_created_at = created["created_at"]
    task_id = created["id"]

    api_patch(env, f"/v1/projects/gr/tasks/{task_id}", {"priority": new_priority})
    api_patch(env, f"/v1/projects/gr/tasks/{task_id}", {"type": new_type})

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert shown["created_at"] == original_created_at


@SETTINGS
@given(new_priority=valid_priorities())
def test_updated_at_advances_on_mutation(running_server, new_priority):
    """updated_at is >= the previous value after a mutation."""
    env = running_server
    created = api_post(env, "/v1/projects/gr/tasks", {"title": "ts advance", "priority": 0})
    task_id = created["id"]
    original_updated = created["updated_at"]

    time.sleep(0.01)
    api_patch(env, f"/v1/projects/gr/tasks/{task_id}", {"priority": new_priority})

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert shown["updated_at"] >= original_updated


@SETTINGS
@given(do_reopen=st.booleans())
def test_closed_at_set_iff_status_closed(running_server, do_reopen):
    """closed_at is non-null when status=closed, null when reopened."""
    env = running_server
    created = api_post(env, "/v1/projects/gr/tasks", {"title": "closed_at invariant"})
    task_id = created["id"]

    # Initial: open, no closed_at.
    assert created.get("closed_at") is None

    # Close: closed_at should be set.
    run_grns(env, "close", task_id, "--json")
    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert shown["status"] == "closed"
    assert shown.get("closed_at") is not None

    if do_reopen:
        run_grns(env, "reopen", task_id, "--json")
        shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
        assert shown["status"] == "open"
        assert shown.get("closed_at") is None


# ---------------------------------------------------------------------------
# 2. Close/Reopen State Machine
# ---------------------------------------------------------------------------


@SETTINGS
@given(actions=st.lists(st.sampled_from(["close", "reopen"]), min_size=1, max_size=8))
def test_close_reopen_state_machine(running_server, actions):
    """After any sequence of close/reopen attempts, the final state is
    consistent: status matches expectations and closed_at agrees."""
    env = running_server
    created = api_post(env, "/v1/projects/gr/tasks", {"title": "state machine"})
    task_id = created["id"]

    current_status = "open"
    for action in actions:
        proc = run_grns_fail(env, action, task_id, "--json")
        if proc.returncode == 0:
            if action == "close":
                current_status = "closed"
            elif action == "reopen":
                current_status = "open"

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert shown["status"] == current_status
    if current_status == "closed":
        assert shown.get("closed_at") is not None
    else:
        assert shown.get("closed_at") is None


# ---------------------------------------------------------------------------
# 3. Custom Fields Roundtrip
# ---------------------------------------------------------------------------


@SETTINGS
@given(custom=custom_field_maps())
def test_custom_fields_roundtrip(running_server, custom):
    """Arbitrary JSON-compatible custom field maps survive create -> show."""
    env = running_server
    created = api_post(env, "/v1/projects/gr/tasks", {"title": "custom rt", "custom": custom})
    task_id = created["id"]

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert shown.get("custom") == custom


# ---------------------------------------------------------------------------
# 4. Dependency Properties
# ---------------------------------------------------------------------------


@SETTINGS
@given(data=st.data())
def test_dep_add_idempotent(running_server, data):
    """Adding the same dependency twice produces exactly one edge."""
    env = running_server
    parent = api_post(env, "/v1/projects/gr/tasks", {"title": "dep parent"})
    child = api_post(env, "/v1/projects/gr/tasks", {"title": "dep child"})

    n_adds = data.draw(st.integers(min_value=2, max_value=5))
    for _ in range(n_adds):
        run_grns(env, "dep", "add", child["id"], parent["id"], "--json")

    shown = api_get(env, f"/v1/projects/gr/tasks/{child['id']}")
    deps = shown.get("deps", [])
    parent_ids = [d["parent_id"] for d in deps]
    assert parent_ids.count(parent["id"]) == 1


# ---------------------------------------------------------------------------
# 5. Pagination Consistency
# ---------------------------------------------------------------------------


@SETTINGS
@given(
    n_tasks=st.integers(min_value=3, max_value=8),
    page_size=st.integers(min_value=1, max_value=4),
)
def test_pagination_covers_all_tasks(running_server, n_tasks, page_size):
    """Paginating through all results yields every task exactly once."""
    env = running_server
    batch = _counter.next()
    label = f"pg{batch}"

    created_ids = set()
    for i in range(n_tasks):
        t = api_post(env, "/v1/projects/gr/tasks", {"title": f"pag {batch} {i}", "labels": [label]})
        created_ids.add(t["id"])

    seen_ids = []
    offset = 0
    while True:
        results = json_stdout(run_grns(
            env, "list", "--label", label,
            "--limit", str(page_size), "--offset", str(offset), "--json",
        ))
        if not results:
            break
        for r in results:
            seen_ids.append(r["id"])
        if len(results) < page_size:
            break
        offset += page_size

    assert set(seen_ids) == created_ids
    assert len(seen_ids) == len(created_ids), "duplicate tasks in pagination"


# ---------------------------------------------------------------------------
# 6. Filter Composition (AND semantics)
# ---------------------------------------------------------------------------


@SETTINGS
@given(data=st.data())
def test_filter_results_match_all_criteria(running_server, data):
    """Every task returned by a filtered list satisfies ALL active filters."""
    env = running_server
    batch = _counter.next()
    label = f"fc{batch}"

    # Create tasks with varied fields, scoped by a unique label.
    for i in range(5):
        t = data.draw(valid_types())
        p = data.draw(valid_priorities())
        api_post(env, "/v1/projects/gr/tasks", {
            "title": f"fc {batch} {i}",
            "type": t,
            "priority": p,
            "labels": [label],
        })

    # Close some tasks.
    all_tasks = json_stdout(run_grns(env, "list", "--label", label, "--json"))
    n_close = data.draw(st.integers(min_value=0, max_value=min(2, len(all_tasks))))
    for i in range(n_close):
        run_grns(env, "close", all_tasks[i]["id"], "--json")

    # Pick random filter values.
    filter_type = data.draw(valid_types())
    filter_status = data.draw(st.sampled_from(["open", "closed"]))

    results = json_stdout(run_grns(
        env, "list", "--label", label,
        "--type", filter_type, "--status", filter_status, "--json",
    ))

    for task in results:
        assert task["type"] == filter_type, f"type mismatch: {task['type']} != {filter_type}"
        assert task["status"] == filter_status, f"status mismatch: {task['status']} != {filter_status}"


# ---------------------------------------------------------------------------
# 7. Label Add/Remove Idempotency
# ---------------------------------------------------------------------------


def test_dash_prefixed_label_via_cli(running_server):
    """Labels starting with '-' work when --json is before positional args."""
    env = running_server
    created = api_post(env, "/v1/projects/gr/tasks", {"title": "dash label test"})
    task_id = created["id"]
    run_grns(env, "label", "add", "--json", task_id, "-review")
    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert "-review" in shown.get("labels", [])

    # Also verify removal works.
    run_grns(env, "label", "remove", "--json", task_id, "-review")
    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert "-review" not in shown.get("labels", [])


@SETTINGS
@given(
    initial=st.lists(valid_labels(), min_size=0, max_size=3, unique=True),
    to_add=valid_labels(),
)
def test_label_add_idempotent(running_server, initial, to_add):
    """Adding the same label multiple times produces exactly one copy."""
    env = running_server
    created = api_post(env, "/v1/projects/gr/tasks", {"title": "label idem", "labels": initial})
    task_id = created["id"]

    # Add same label twice via CLI.
    run_grns(env, "label", "add", "--json", task_id, to_add)
    run_grns(env, "label", "add", "--json", task_id, to_add)

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    result_labels = shown.get("labels", [])

    expected = sorted(set(lbl.lower() for lbl in initial) | {to_add.lower()})
    assert result_labels == expected


@SETTINGS
@given(labels=st.lists(valid_labels(), min_size=2, max_size=5, unique=True))
def test_label_remove_nonexistent_is_safe(running_server, labels):
    """Removing a label that isn't on the task doesn't error or affect existing labels."""
    env = running_server
    on_task = [labels[0]]
    absent = labels[1]

    created = api_post(env, "/v1/projects/gr/tasks", {"title": "label rm safe", "labels": on_task})
    task_id = created["id"]

    # Remove a label that isn't on the task — should succeed.
    run_grns(env, "label", "remove", "--json", task_id, absent)

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    result_labels = shown.get("labels", [])
    assert labels[0].lower() in result_labels


@SETTINGS
@given(labels=mixed_case_label_lists(min_size=1, max_size=6))
def test_label_add_api_normalizes(running_server, labels):
    """Adding labels via the label API with mixed case and duplicates
    produces a normalized (lowercase, deduped, sorted) result."""
    env = running_server
    assume(all(lbl.strip() for lbl in labels))

    created = api_post(env, "/v1/projects/gr/tasks", {"title": "label api norm"})
    task_id = created["id"]

    # Add labels via dedicated label endpoint.
    for lbl in labels:
        api_post(env, f"/v1/projects/gr/tasks/{task_id}/labels", {"labels": [lbl]})

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    result_labels = shown.get("labels", [])
    expected = sorted(set(lbl.lower() for lbl in labels))
    assert result_labels == expected


# ---------------------------------------------------------------------------
# 8. Description Roundtrip
# ---------------------------------------------------------------------------


@SETTINGS
@given(desc=printable_titles())
def test_description_roundtrip(running_server, desc):
    """Description text survives create -> show with expected edge-whitespace normalization."""
    env = running_server
    created = api_post(env, "/v1/projects/gr/tasks", {"title": "desc rt", "description": desc})
    task_id = created["id"]

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert shown.get("description", "") == desc.strip()


# ---------------------------------------------------------------------------
# 9. List Ordering Guarantee
# ---------------------------------------------------------------------------


@SETTINGS
@given(n_tasks=st.integers(min_value=2, max_value=6))
def test_list_ordered_by_updated_at_desc(running_server, n_tasks):
    """Default list ordering is by updated_at descending (most recent first)."""
    env = running_server
    batch = _counter.next()
    label = f"or{batch}"

    for i in range(n_tasks):
        api_post(env, "/v1/projects/gr/tasks", {"title": f"order {batch} {i}", "labels": [label]})
        time.sleep(0.015)  # Ensure distinct timestamps.

    results = json_stdout(run_grns(env, "list", "--label", label, "--json"))

    for i in range(len(results) - 1):
        assert results[i]["updated_at"] >= results[i + 1]["updated_at"], (
            f"ordering violated at index {i}: "
            f"{results[i]['updated_at']} < {results[i+1]['updated_at']}"
        )


# ---------------------------------------------------------------------------
# 10. Batch Get Preserves Request Order
# ---------------------------------------------------------------------------


@SETTINGS
@given(data=st.data())
def test_batch_get_preserves_request_order(running_server, data):
    """Showing multiple tasks returns them in the requested order."""
    env = running_server
    n = data.draw(st.integers(min_value=2, max_value=5))

    ids = []
    for i in range(n):
        t = api_post(env, "/v1/projects/gr/tasks", {"title": f"batch order {i}"})
        ids.append(t["id"])

    shuffled = list(data.draw(st.permutations(ids)))

    results = json_stdout(run_grns(env, "show", *shuffled, "--json"))
    result_ids = [r["id"] for r in results]
    assert result_ids == shuffled


# ---------------------------------------------------------------------------
# 11. ID Format Always Valid
# ---------------------------------------------------------------------------


@SETTINGS
@given(title=printable_titles(), priority=valid_priorities(), task_type=valid_types())
def test_auto_generated_id_matches_pattern(running_server, title, priority, task_type):
    """Every auto-generated task ID matches ^[a-z]{2}-[0-9a-z]{4}$."""
    env = running_server
    created = api_post(env, "/v1/projects/gr/tasks", {
        "title": title,
        "priority": priority,
        "type": task_type,
    })
    assert ID_PATTERN.match(created["id"]), f"ID {created['id']!r} doesn't match pattern"


# ---------------------------------------------------------------------------
# 12. Import Dedupe Modes
# ---------------------------------------------------------------------------


@SETTINGS
@given(dedupe_mode=st.sampled_from(["skip", "overwrite"]))
def test_import_same_data_twice(running_server, tmp_path, dedupe_mode):
    """Re-importing the same task with skip/overwrite creates 0 new tasks."""
    env = running_server
    batch = _counter.next()
    task_id = f"gr-{batch}00"
    title = f"dedupe {batch}"

    record = json.dumps({
        "id": task_id,
        "title": title,
        "status": "open",
        "type": "task",
        "priority": 2,
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    })

    infile = tmp_path / f"dedupe_{batch}.jsonl"
    infile.write_text(record + "\n")

    # First import.
    result1 = json_stdout(run_grns(env, "import", "-i", str(infile), "--json"))
    assert int(result1["created"]) == 1

    # Second import with dedupe mode.
    result2 = json_stdout(run_grns(env, "import", "-i", str(infile), "--dedupe", dedupe_mode, "--json"))
    assert int(result2["created"]) == 0

    # Task still exists with correct title.
    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert shown["title"] == title
