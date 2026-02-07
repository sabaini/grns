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

    def update_priority(value: int):
        run_grns(env, "update", task_id, "--priority", str(value), "--json")

    with ThreadPoolExecutor(max_workers=8) as pool:
        list(pool.map(update_priority, [0, 1, 2, 3, 4] * 8))

    task = json_stdout(run_grns(env, "show", task_id, "--json"))
    assert 0 <= int(task["priority"]) <= 4
    assert task["status"] in {"open", "in_progress", "blocked", "deferred", "closed", "pinned", "tombstone"}
