from concurrent.futures import ThreadPoolExecutor

from tests_py.helpers import json_stdout, run_grns


def test_concurrent_creates_generate_unique_ids(running_server):
    env = running_server

    def create_one(idx: int) -> str:
        proc = run_grns(env, "create", f"Concurrent task {idx}", "--json")
        return json_stdout(proc)["id"]

    with ThreadPoolExecutor(max_workers=8) as pool:
        ids = list(pool.map(create_one, range(30)))

    assert len(ids) == len(set(ids))

    listed = json_stdout(run_grns(env, "list", "--json"))
    assert len(listed) == 30


def test_concurrent_updates_preserve_task_invariants(running_server):
    env = running_server

    created = json_stdout(run_grns(env, "create", "Race target", "--json"))
    task_id = created["id"]

    written_priorities = [0, 1, 2, 3, 4]

    def update_priority(value: int):
        run_grns(env, "update", task_id, "--priority", str(value), "--json")

    with ThreadPoolExecutor(max_workers=8) as pool:
        list(pool.map(update_priority, written_priorities * 8))

    task = json_stdout(run_grns(env, "show", task_id, "--json"))
    # Final priority must be one of the values we actually wrote
    assert int(task["priority"]) in written_priorities, (
        f"priority {task['priority']} not in {written_priorities}"
    )
    # Title must be unchanged — concurrent priority updates must not corrupt other fields
    assert task["title"] == "Race target", (
        f"title was corrupted: {task['title']!r}"
    )
    # Status must still be "open" — we never updated it
    assert task["status"] == "open", (
        f"status was corrupted: {task['status']!r}"
    )
