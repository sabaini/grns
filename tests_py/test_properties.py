"""Property-based tests for grns using Hypothesis.

These tests exercise the full CLI/API stack with randomized inputs to find
edge cases in validation, normalization, and roundtrip consistency.
"""

import json
import urllib.error

import pytest
from hypothesis import given, settings, assume, HealthCheck
from hypothesis import strategies as st

from tests_py.helpers import api_get, api_patch, api_post, json_stdout, run_grns
from tests_py.strategies import (
    VALID_STATUSES,
    VALID_TYPES,
    case_varied_statuses,
    case_varied_types,
    invalid_priorities,
    invalid_statuses,
    mixed_case_label_lists,
    printable_titles,
    valid_priorities,
    valid_statuses,
    valid_types,
)

pytestmark = pytest.mark.hypothesis


class _Counter:
    """Counter for generating unique ID suffixes across hypothesis examples."""

    def __init__(self):
        self._n = 0

    def next(self) -> str:
        """Return a 2-char base36 suffix and increment."""
        import string
        chars = string.digits + string.ascii_lowercase
        n = self._n
        self._n += 1
        c1 = chars[n // 36 % 36]
        c2 = chars[n % 36]
        return f"{c1}{c2}"


_counter = _Counter()

# Keep CI fast — 30 examples is enough to catch most edge cases.
# Suppress function_scoped_fixture: we intentionally share one server across
# all hypothesis examples within a single test (accumulating state is fine).
SETTINGS = settings(
    max_examples=30,
    deadline=None,
    suppress_health_check=[HealthCheck.function_scoped_fixture],
)


# ---------------------------------------------------------------------------
# Create → Show roundtrip
# ---------------------------------------------------------------------------


@SETTINGS
@given(title=printable_titles(), priority=valid_priorities(), task_type=valid_types())
def test_create_show_roundtrip(running_server, title, priority, task_type):
    """Creating a task and showing it returns the same field values."""
    env = running_server

    created = api_post(env, "/v1/projects/gr/tasks", {
        "title": title,
        "priority": priority,
        "type": task_type,
    })
    task_id = created["id"]

    # Verify create response matches input
    assert created["title"] == title.strip()
    assert created["priority"] == priority
    assert created["type"] == task_type

    # Verify show returns exactly what create stored
    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert shown["title"] == created["title"]
    assert shown["priority"] == priority
    assert shown["type"] == task_type
    assert shown["status"] == "open"


# ---------------------------------------------------------------------------
# Priority bounds
# ---------------------------------------------------------------------------


@SETTINGS
@given(priority=valid_priorities())
def test_valid_priority_accepted(running_server, priority):
    """Any priority in [0, 4] is accepted on create."""
    env = running_server
    resp = api_post(env, "/v1/projects/gr/tasks", {"title": "prio test", "priority": priority})
    assert resp["priority"] == priority


@SETTINGS
@given(priority=invalid_priorities())
def test_invalid_priority_rejected(running_server, priority):
    """Priorities outside [0, 4] are rejected with HTTP 400."""
    env = running_server

    with pytest.raises(urllib.error.HTTPError) as exc_info:
        api_post(env, "/v1/projects/gr/tasks", {"title": "bad prio", "priority": priority})
    assert exc_info.value.code == 400


# ---------------------------------------------------------------------------
# Status normalization (case-insensitive)
# ---------------------------------------------------------------------------


@SETTINGS
@given(status=case_varied_statuses())
def test_status_normalization_case_insensitive(running_server, status):
    """Updating status with any casing normalizes to lowercase."""
    env = running_server

    created = api_post(env, "/v1/projects/gr/tasks", {"title": "status norm"})
    task_id = created["id"]

    updated = api_patch(env, f"/v1/projects/gr/tasks/{task_id}", {"status": status})
    assert updated["status"] == status.strip().lower()
    assert updated["status"] in VALID_STATUSES


# ---------------------------------------------------------------------------
# Type normalization (case-insensitive)
# ---------------------------------------------------------------------------


@SETTINGS
@given(task_type=case_varied_types())
def test_type_normalization_case_insensitive(running_server, task_type):
    """Creating with any casing of a valid type normalizes to lowercase."""
    env = running_server

    created = api_post(env, "/v1/projects/gr/tasks", {"title": "type norm", "type": task_type})
    assert created["type"] == task_type.strip().lower()
    assert created["type"] in VALID_TYPES


# ---------------------------------------------------------------------------
# Labels: normalization, dedup, and sorting
# ---------------------------------------------------------------------------


@SETTINGS
@given(labels=mixed_case_label_lists(min_size=1, max_size=6))
def test_labels_normalized_deduped_sorted(running_server, labels):
    """Labels are lowercased, deduplicated, and returned sorted — even when
    the input contains duplicates and mixed case."""
    env = running_server
    assume(all(lbl.strip() for lbl in labels))

    created = api_post(env, "/v1/projects/gr/tasks", {"title": "label test", "labels": labels})
    task_id = created["id"]

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    result_labels = shown.get("labels", [])

    expected = sorted(set(lbl.lower() for lbl in labels))

    assert result_labels == sorted(result_labels), "labels not sorted"
    assert len(result_labels) == len(set(result_labels)), "labels contain duplicates"
    for lbl in result_labels:
        assert lbl == lbl.lower(), f"label {lbl!r} not lowercase"
    assert result_labels == expected, (
        f"expected {expected}, got {result_labels} from input {labels}"
    )


# ---------------------------------------------------------------------------
# Update preserves unmodified fields
# ---------------------------------------------------------------------------


@SETTINGS
@given(
    field_to_update=st.sampled_from(["priority", "status", "type", "description"]),
    new_priority=valid_priorities(),
    new_status=valid_statuses(),
    new_type=valid_types(),
    new_desc=printable_titles(),
)
def test_update_preserves_unmodified_fields(
    running_server, field_to_update, new_priority, new_status, new_type, new_desc
):
    """Updating a single field leaves all other fields unchanged."""
    env = running_server

    created = api_post(env, "/v1/projects/gr/tasks", {
        "title": "preserve test",
        "type": "bug",
        "priority": 3,
        "description": "original desc",
    })
    task_id = created["id"]

    # Build a patch with exactly one field
    patch = {
        "priority": new_priority,
        "status": new_status,
        "type": new_type,
        "description": new_desc,
    }
    single_patch = {field_to_update: patch[field_to_update]}

    updated = api_patch(env, f"/v1/projects/gr/tasks/{task_id}", single_patch)

    # The updated field should have the new value
    if field_to_update == "priority":
        assert updated["priority"] == new_priority
    elif field_to_update == "status":
        assert updated["status"] == new_status
    elif field_to_update == "type":
        assert updated["type"] == new_type
    elif field_to_update == "description":
        assert updated.get("description", "") == new_desc

    # All OTHER fields must be preserved
    if field_to_update != "title":
        assert updated["title"] == created["title"], "title was clobbered"
    if field_to_update != "type":
        assert updated["type"] == created["type"], "type was clobbered"
    if field_to_update != "priority":
        assert updated["priority"] == created["priority"], "priority was clobbered"
    if field_to_update != "description":
        assert updated.get("description", "") == created.get("description", ""), "description was clobbered"


# ---------------------------------------------------------------------------
# Title whitespace trimming
# ---------------------------------------------------------------------------


@SETTINGS
@given(title=printable_titles())
def test_title_whitespace_trimmed(running_server, title):
    """Titles with leading/trailing whitespace are trimmed in response."""
    env = running_server
    padded = f"  {title}  "

    created = api_post(env, "/v1/projects/gr/tasks", {"title": padded})
    assert created["title"] == padded.strip()


# ---------------------------------------------------------------------------
# Invalid status rejected
# ---------------------------------------------------------------------------


@SETTINGS
@given(status=invalid_statuses())
def test_invalid_status_rejected(running_server, status):
    """Updating with an invalid status string is rejected."""
    env = running_server

    created = api_post(env, "/v1/projects/gr/tasks", {"title": "inv status"})
    task_id = created["id"]

    with pytest.raises(urllib.error.HTTPError) as exc_info:
        api_patch(env, f"/v1/projects/gr/tasks/{task_id}", {"status": status})
    assert exc_info.value.code == 400


# ---------------------------------------------------------------------------
# Import → Export roundtrip
# ---------------------------------------------------------------------------


@SETTINGS
@given(
    priority=valid_priorities(),
    task_type=valid_types(),
    status=valid_statuses(),
)
def test_import_export_roundtrip(running_server, tmp_path, priority, task_type, status):
    """Tasks imported via JSONL can be exported with all fields preserved."""
    env = running_server

    batch = _counter.next()
    # Use distinct printable titles per batch
    titles = [f"Import task {batch}-{i}" for i in range(3)]

    import_lines = []
    task_ids = []
    for i, title in enumerate(titles):
        tid = f"gr-{batch}{i:02d}"
        task_ids.append(tid)
        record = {
            "id": tid,
            "title": title,
            "status": status,
            "type": task_type,
            "priority": priority,
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": "2026-01-01T00:00:00Z",
        }
        import_lines.append(json.dumps(record, separators=(",", ":")))

    import_file = tmp_path / f"roundtrip_{batch}.jsonl"
    import_file.write_text("\n".join(import_lines) + "\n", encoding="utf-8")

    result = json_stdout(run_grns(env, "import", "-i", str(import_file), "--json"))
    assert int(result["created"]) == len(titles)

    # Export and verify all fields survive
    export_proc = run_grns(env, "export")
    export_lines = [line for line in export_proc.stdout.strip().split("\n") if line]
    exported = {json.loads(line)["id"]: json.loads(line) for line in export_lines}

    for tid, title in zip(task_ids, titles):
        assert tid in exported, f"task {tid} missing from export"
        assert exported[tid]["title"] == title
        assert exported[tid]["type"] == task_type
        assert exported[tid]["priority"] == priority
        assert exported[tid]["status"] == status, (
            f"status mismatch: expected {status!r}, got {exported[tid]['status']!r}"
        )
