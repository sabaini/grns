from concurrent.futures import ThreadPoolExecutor, as_completed
import threading
import time
import urllib.error

from tests_py.helpers import api_get, api_post, run_grns


def test_concurrent_create_same_explicit_id_only_one_succeeds(running_server):
    env = running_server
    task_id = "gr-c0de"
    attempts = 24

    def create_once(_idx: int):
        try:
            created = api_post(env, "/v1/tasks", {"id": task_id, "title": "same id race"})
            return "ok", created["id"]
        except urllib.error.HTTPError as exc:
            body = exc.read().decode("utf-8", errors="replace")
            if exc.code == 409:
                return "conflict", body
            return "error", f"{exc.code}: {body}"

    with ThreadPoolExecutor(max_workers=12) as pool:
        results = list(pool.map(create_once, range(attempts)))

    success = [value for kind, value in results if kind == "ok"]
    conflicts = [value for kind, value in results if kind == "conflict"]
    unexpected = [value for kind, value in results if kind == "error"]

    assert len(success) == 1
    assert success[0] == task_id
    assert len(conflicts) == attempts - 1
    assert unexpected == []

    shown = api_get(env, f"/v1/tasks/{task_id}")
    assert shown["id"] == task_id


def test_concurrent_close_reopen_preserves_closed_at_invariant(running_server):
    env = running_server
    created = api_post(env, "/v1/tasks", {"title": "Concurrent toggle target"})
    task_id = created["id"]

    ops = ["close" if i % 2 == 0 else "reopen" for i in range(80)]

    def mutate(op: str):
        path = "/v1/tasks/close" if op == "close" else "/v1/tasks/reopen"
        api_post(env, path, {"ids": [task_id]})

    with ThreadPoolExecutor(max_workers=12) as pool:
        list(pool.map(mutate, ops))

    shown = api_get(env, f"/v1/tasks/{task_id}")
    assert shown["status"] in {"open", "closed"}
    if shown["status"] == "closed":
        assert shown.get("closed_at") is not None
    else:
        assert shown.get("closed_at") is None
    assert shown["title"] == "Concurrent toggle target"


def test_concurrent_label_add_remove_keeps_labels_unique(running_server):
    env = running_server
    created = api_post(env, "/v1/tasks", {"title": "Concurrent label target"})
    task_id = created["id"]

    label_pool = ["alpha", "beta", "gamma"]

    def mutate(i: int):
        label = label_pool[i % len(label_pool)]
        if i % 3 == 0:
            run_grns(env, "label", "remove", "--json", task_id, label)
            return
        run_grns(env, "label", "add", "--json", task_id, label)

    with ThreadPoolExecutor(max_workers=10) as pool:
        list(pool.map(mutate, range(90)))

    shown = api_get(env, f"/v1/tasks/{task_id}")
    labels = shown.get("labels", [])

    assert labels == sorted(labels)
    assert len(labels) == len(set(labels))
    assert set(labels).issubset(set(label_pool))
    assert shown["status"] == "open"
    assert shown["title"] == "Concurrent label target"


def test_concurrent_create_and_list_visibility(running_server):
    env = running_server
    label = f"conc-vis-{time.time_ns()}"

    create_count = 48
    list_rounds = 36

    created_ids = set()
    observed_ids = set()
    lock = threading.Lock()

    def create_one(idx: int):
        created = api_post(
            env,
            "/v1/tasks",
            {"title": f"Visibility task {idx}", "labels": [label]},
        )
        with lock:
            created_ids.add(created["id"])

    def list_one(_idx: int):
        listed = api_get(env, f"/v1/tasks?label={label}&limit=500")
        ids = {item["id"] for item in listed}
        with lock:
            observed_ids.update(ids)

    with ThreadPoolExecutor(max_workers=16) as pool:
        futures = []
        for i in range(max(create_count, list_rounds)):
            if i < create_count:
                futures.append(pool.submit(create_one, i))
            if i < list_rounds:
                futures.append(pool.submit(list_one, i))

        for future in as_completed(futures):
            future.result()

    final_list = api_get(env, f"/v1/tasks?label={label}&limit=500")
    final_ids = {task["id"] for task in final_list}

    assert len(created_ids) == create_count
    assert created_ids == final_ids
    assert observed_ids.issubset(final_ids)
