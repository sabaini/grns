"""Stateful model tests for task git references."""

from __future__ import annotations

import json
import posixpath
from urllib.parse import urlparse
import urllib.error
import urllib.request

import pytest
from hypothesis import HealthCheck, settings
from hypothesis import strategies as st
from hypothesis.stateful import RuleBasedStateMachine, invariant, precondition, rule, run_state_machine_as_test

from tests_py.helpers import api_post
from tests_py.strategies_git_refs import (
    git_hash_valid,
    git_object_types,
    git_relation_valid,
    repo_path_valid,
    repo_slug_equivalent_forms,
)

pytestmark = pytest.mark.hypothesis

STATEFUL_SETTINGS = settings(
    max_examples=10,
    stateful_step_count=20,
    deadline=None,
    suppress_health_check=[HealthCheck.function_scoped_fixture],
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def request_json(env: dict[str, str], method: str, path: str, body: dict | None = None) -> tuple[int, dict | list]:
    url = env["GRNS_API_URL"] + path
    data = json.dumps(body).encode("utf-8") if body is not None else None
    headers = {"Content-Type": "application/json"} if body is not None else {}
    req = urllib.request.Request(url, data=data, method=method, headers=headers)

    try:
        with urllib.request.urlopen(req) as resp:
            raw = resp.read().decode("utf-8")
            parsed = json.loads(raw) if raw else {}
            return resp.status, parsed
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8")
        parsed = json.loads(raw) if raw else {}
        return exc.code, parsed


def canonical_repo_slug(raw: str) -> str:
    value = raw.strip()
    if not value:
        raise ValueError("repo is required")

    if "://" in value:
        parsed = urlparse(value)
        host = (parsed.hostname or "").strip()
        if not host:
            raise ValueError("invalid repo")
        value = f"{host}/{parsed.path.strip('/')}"
    elif "@" in value and ":" in value:
        host_part, path_part = value.split(":", 1)
        host = host_part.split("@")[-1].strip()
        value = f"{host}/{path_part.strip('/')}"

    value = value.strip().lower().rstrip("/")
    if value.endswith(".git"):
        value = value[:-4]

    parts = value.split("/")
    if len(parts) != 3:
        raise ValueError("repo must be host/owner/name")

    return "/".join(parts)


def ref_signature(ref: dict) -> tuple[str, str, str, str, str]:
    return (
        ref["repo"],
        ref["relation"],
        ref["object_type"],
        ref["object_value"],
        ref.get("resolved_commit", ""),
    )


@st.composite
def maybe_repo_input(draw: st.DrawFn) -> tuple[str | None, str | None]:
    if draw(st.booleans()):
        canonical, forms = draw(repo_slug_equivalent_forms())
        return draw(st.sampled_from(forms)), canonical
    return None, None


@st.composite
def maybe_source_repo(draw: st.DrawFn) -> tuple[str | None, str | None]:
    if draw(st.booleans()):
        canonical, forms = draw(repo_slug_equivalent_forms())
        return draw(st.sampled_from(forms)), canonical
    return None, None


@st.composite
def valid_ref_payload(draw: st.DrawFn) -> dict:
    object_type = draw(git_object_types())

    if object_type in {"commit", "blob", "tree"}:
        object_value = draw(git_hash_valid())
    elif object_type == "path":
        object_value = draw(repo_path_valid())
    else:
        object_value = draw(
            st.text(
                alphabet="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-",
                min_size=1,
                max_size=40,
            ).filter(lambda s: s.strip() and not any(ch.isspace() for ch in s))
        )

    relation = draw(git_relation_valid())
    resolved = draw(st.one_of(st.just(""), git_hash_valid()))
    repo_input, _ = draw(maybe_repo_input())

    payload = {
        "relation": relation,
        "object_type": object_type,
        "object_value": object_value,
    }
    if resolved:
        payload["resolved_commit"] = resolved
    if repo_input is not None:
        payload["repo"] = repo_input

    return payload


# ---------------------------------------------------------------------------
# Stateful test
# ---------------------------------------------------------------------------


def test_stateful_task_git_refs_model(running_server):
    env = running_server

    class GitRefStateMachine(RuleBasedStateMachine):
        def __init__(self):
            super().__init__()
            self.tasks: dict[str, str] = {}
            self.refs_by_task: dict[str, set[tuple[str, str, str, str, str]]] = {}
            self.id_to_signature: dict[str, tuple[str, tuple[str, str, str, str, str]]] = {}
            self.deleted_ref_ids: set[str] = set()

        @rule(source=maybe_source_repo())
        def create_task(self, source: tuple[str | None, str | None]):
            source_input, source_canonical = source
            body = {"title": "stateful git refs task"}
            if source_input is not None:
                body["source_repo"] = source_input

            created = api_post(env, "/v1/tasks", body)
            task_id = created["id"]
            self.tasks[task_id] = source_canonical or ""
            self.refs_by_task.setdefault(task_id, set())

        @precondition(lambda self: len(self.tasks) > 0)
        @rule(data=st.data(), payload=valid_ref_payload())
        def add_ref(self, data, payload):
            task_id = data.draw(st.sampled_from(sorted(self.tasks.keys())))

            repo_raw = payload.get("repo")
            if repo_raw is None:
                repo_canonical = self.tasks[task_id]
            else:
                repo_canonical = canonical_repo_slug(str(repo_raw))

            if not repo_canonical:
                status, err = request_json(env, "POST", f"/v1/tasks/{task_id}/git-refs", payload)
                assert status == 400
                assert err["code"] == "invalid_argument"
                return

            object_type = payload["object_type"].strip().lower()
            object_value = payload["object_value"].strip()
            if object_type in {"commit", "blob", "tree"}:
                object_value = object_value.lower()
            elif object_type == "path":
                object_value = posixpath.normpath(object_value)

            signature = (
                repo_canonical,
                payload["relation"].strip().lower(),
                object_type,
                object_value,
                payload.get("resolved_commit", "").strip().lower(),
            )

            status, resp = request_json(env, "POST", f"/v1/tasks/{task_id}/git-refs", payload)
            if signature in self.refs_by_task[task_id]:
                assert status == 409
                assert resp["code"] == "conflict"
                return

            assert status == 201
            self.refs_by_task[task_id].add(signature)
            self.id_to_signature[resp["id"]] = (task_id, signature)
            self.deleted_ref_ids.discard(resp["id"])

        @precondition(lambda self: len(self.id_to_signature) > 0 or len(self.deleted_ref_ids) > 0)
        @rule(data=st.data())
        def delete_ref(self, data):
            candidates = sorted(list(self.id_to_signature.keys()) + list(self.deleted_ref_ids))
            ref_id = data.draw(st.sampled_from(candidates))

            status, resp = request_json(env, "DELETE", f"/v1/git-refs/{ref_id}")
            if ref_id in self.id_to_signature:
                assert status == 200
                task_id, signature = self.id_to_signature.pop(ref_id)
                self.refs_by_task[task_id].discard(signature)
                self.deleted_ref_ids.add(ref_id)
                return

            assert status == 404
            assert resp["code"] == "not_found"

        @precondition(lambda self: len(self.tasks) > 0)
        @rule(data=st.data(), commit=git_hash_valid(), provided_repo=maybe_repo_input())
        def close_with_commit(self, data, commit: str, provided_repo: tuple[str | None, str | None]):
            task_ids = sorted(self.tasks.keys())
            chosen = data.draw(
                st.lists(
                    st.sampled_from(task_ids),
                    min_size=1,
                    max_size=min(3, len(task_ids)),
                    unique=True,
                )
            )

            repo_input, repo_canonical = provided_repo
            body = {"ids": chosen, "commit": commit}
            if repo_input is not None:
                body["repo"] = repo_input

            expected_new = 0
            missing_repo = False
            normalized_commit = commit.strip().lower()

            for task_id in chosen:
                task_repo = repo_canonical or self.tasks[task_id]
                if not task_repo:
                    missing_repo = True
                    break
                sig = (task_repo, "closed_by", "commit", normalized_commit, "")
                if sig not in self.refs_by_task[task_id]:
                    expected_new += 1

            status, resp = request_json(env, "POST", "/v1/tasks/close", body)
            if missing_repo:
                assert status == 400
                assert resp["code"] == "invalid_argument"
                return

            assert status == 200
            assert int(resp.get("annotated", -1)) == expected_new

            for task_id in chosen:
                task_repo = repo_canonical or self.tasks[task_id]
                sig = (task_repo, "closed_by", "commit", normalized_commit, "")
                self.refs_by_task[task_id].add(sig)

        @precondition(lambda self: len(self.tasks) > 0)
        @rule(data=st.data())
        def list_refs(self, data):
            task_id = data.draw(st.sampled_from(sorted(self.tasks.keys())))
            status, listed = request_json(env, "GET", f"/v1/tasks/{task_id}/git-refs")
            assert status == 200
            api_sigs = {ref_signature(ref) for ref in listed}
            assert api_sigs == self.refs_by_task[task_id]

        @invariant()
        def api_and_model_stay_in_sync(self):
            for task_id, expected_sigs in self.refs_by_task.items():
                status, listed = request_json(env, "GET", f"/v1/tasks/{task_id}/git-refs")
                assert status == 200
                api_sigs = [ref_signature(ref) for ref in listed]
                assert len(api_sigs) == len(set(api_sigs))
                assert set(api_sigs) == expected_sigs

                for ref in listed:
                    ref_id = ref["id"]
                    sig = ref_signature(ref)
                    self.id_to_signature[ref_id] = (task_id, sig)
                    self.deleted_ref_ids.discard(ref_id)

            for ref_id, (_, sig) in self.id_to_signature.items():
                status, fetched = request_json(env, "GET", f"/v1/git-refs/{ref_id}")
                assert status == 200
                assert ref_signature(fetched) == sig

            for ref_id in self.deleted_ref_ids:
                status, err = request_json(env, "GET", f"/v1/git-refs/{ref_id}")
                assert status == 404
                assert err["code"] == "not_found"

    run_state_machine_as_test(GitRefStateMachine, settings=STATEFUL_SETTINGS)
