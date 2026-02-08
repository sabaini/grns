"""Property-based tests for task↔git reference behavior."""

from __future__ import annotations

import json
import posixpath
import re
import sqlite3
from urllib.parse import urlparse
import urllib.error
import urllib.request

import pytest
from hypothesis import HealthCheck, assume, given, settings
from hypothesis import strategies as st

from tests_py.helpers import api_get, api_post, scoped_api_path
from tests_py.strategies_git_refs import (
    git_hash_invalid,
    git_hash_valid,
    git_object_types,
    git_relation_invalid,
    git_relation_valid,
    repo_path_invalid,
    repo_path_valid,
    repo_slug_canonical,
    repo_slug_equivalent_forms,
    small_json_meta,
)

pytestmark = pytest.mark.hypothesis

SETTINGS = settings(
    max_examples=30,
    deadline=None,
    suppress_health_check=[HealthCheck.function_scoped_fixture],
)

GIT_REF_ID_RE = re.compile(r"^gf-[0-9a-z]{4}$")
NOTE_TEXT = st.text(
    alphabet="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 -_.,:/",
    min_size=1,
    max_size=60,
).filter(lambda s: s.strip())


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def request_json(env: dict[str, str], method: str, path: str, body: dict | None = None) -> tuple[int, dict | list]:
    url = env["GRNS_API_URL"] + scoped_api_path(env, path)
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


def assert_error_contract(status: int, payload: dict, expected_status: int, expected_code: str) -> None:
    assert status == expected_status
    assert "error" in payload
    assert "code" in payload
    assert "error_code" in payload
    assert payload["code"] == expected_code
    assert isinstance(payload["error_code"], int)
    assert payload["error_code"] > 0


def create_task(env: dict[str, str], source_repo: str | None = None) -> dict:
    body: dict[str, object] = {"title": "git ref test task"}
    if source_repo is not None:
        body["source_repo"] = source_repo
    return api_post(env, "/v1/projects/gr/tasks", body)


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
    if any((not part) or any(ch.isspace() for ch in part) for part in parts):
        raise ValueError("repo must be host/owner/name")

    return "/".join(parts)


def normalize_hash(value: str) -> str:
    return value.strip().lower()


def normalize_object_value(object_type: str, object_value: str) -> str:
    object_type = object_type.strip().lower()
    value = object_value.strip()
    if object_type in {"commit", "blob", "tree"}:
        return normalize_hash(value)
    if object_type == "path":
        return posixpath.normpath(value)
    return value


def ref_signature(ref: dict) -> tuple[str, str, str, str, str]:
    return (
        ref["repo"],
        ref["relation"],
        ref["object_type"],
        ref["object_value"],
        ref.get("resolved_commit", ""),
    )


def hash_for_index(i: int) -> str:
    return f"{i:040x}"[-40:]


@st.composite
def valid_git_ref_payload(draw: st.DrawFn) -> dict:
    object_type = draw(git_object_types())
    relation = draw(git_relation_valid())

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

    resolved_commit = draw(st.one_of(st.just(""), git_hash_valid()))

    repo = None
    if draw(st.booleans()):
        _, forms = draw(repo_slug_equivalent_forms())
        repo = draw(st.sampled_from(forms))

    note = draw(st.one_of(st.none(), NOTE_TEXT))
    meta = draw(st.one_of(st.none(), small_json_meta().filter(lambda m: len(m) > 0)))

    payload = {
        "relation": relation,
        "object_type": object_type,
        "object_value": object_value,
    }
    if repo is not None:
        payload["repo"] = repo
    if resolved_commit:
        payload["resolved_commit"] = resolved_commit
    if note is not None:
        payload["note"] = note
    if meta is not None:
        payload["meta"] = meta

    return payload


# ---------------------------------------------------------------------------
# P0.1 Create → Get → List roundtrip
# ---------------------------------------------------------------------------


@SETTINGS
@given(data=st.data(), payload=valid_git_ref_payload())
def test_git_ref_create_get_list_roundtrip(running_server, data, payload):
    env = running_server

    _, source_forms = data.draw(repo_slug_equivalent_forms())
    source_repo = data.draw(st.sampled_from(source_forms))
    task = create_task(env, source_repo=source_repo)

    status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", payload)
    assert status == 201
    assert GIT_REF_ID_RE.match(created["id"])

    expected_repo = canonical_repo_slug(payload.get("repo", source_repo))
    assert created["repo"] == expected_repo
    assert created["relation"] == payload["relation"].strip().lower()
    assert created["object_type"] == payload["object_type"].strip().lower()
    assert created["object_value"] == normalize_object_value(payload["object_type"], payload["object_value"])
    assert created.get("resolved_commit", "") == normalize_hash(payload.get("resolved_commit", ""))

    if "note" in payload:
        assert created.get("note", "") == payload["note"].strip()
    if "meta" in payload:
        assert created.get("meta", {}) == payload["meta"]

    status, fetched = request_json(env, "GET", f"/v1/projects/gr/git-refs/{created['id']}")
    assert status == 200
    assert fetched["id"] == created["id"]
    assert ref_signature(fetched) == ref_signature(created)

    status, listed = request_json(env, "GET", f"/v1/projects/gr/tasks/{task['id']}/git-refs")
    assert status == 200
    assert isinstance(listed, list)
    matches = [ref for ref in listed if ref["id"] == created["id"]]
    assert len(matches) == 1


# ---------------------------------------------------------------------------
# P0.2 Repo canonicalization equivalence
# ---------------------------------------------------------------------------


@SETTINGS
@given(pair=repo_slug_equivalent_forms(), commit=git_hash_valid())
def test_repo_canonicalization_equivalence_conflicts(running_server, pair, commit):
    env = running_server
    canonical, forms = pair

    task = create_task(env, source_repo=forms[0])
    payload = {
        "repo": forms[0],
        "relation": "related",
        "object_type": "commit",
        "object_value": commit,
    }

    status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", payload)
    assert status == 201
    assert created["repo"] == canonical

    for form in forms[1:]:
        payload["repo"] = form
        status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", payload)
        assert_error_contract(status, err, 409, "conflict")


# ---------------------------------------------------------------------------
# P0.3 source_repo fallback
# ---------------------------------------------------------------------------


@SETTINGS
@given(pair=repo_slug_equivalent_forms())
def test_source_repo_fallback_and_missing_required(running_server, pair):
    env = running_server
    canonical, forms = pair

    # Case A: fallback works.
    task_a = create_task(env, source_repo=forms[1])
    payload = {
        "relation": "design_doc",
        "object_type": "path",
        "object_value": "docs/design.md",
    }
    status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task_a['id']}/git-refs", payload)
    assert status == 201
    assert created["repo"] == canonical

    # Case B: fallback unavailable.
    task_b = create_task(env)
    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task_b['id']}/git-refs", payload)
    assert_error_contract(status, err, 400, "invalid_argument")
    assert "required" in err["error"].lower()


# ---------------------------------------------------------------------------
# P0.4 Object-type-specific validation/normalization
# ---------------------------------------------------------------------------


@SETTINGS
@given(object_type=st.sampled_from(["commit", "blob", "tree"]), object_value=git_hash_valid(), resolved=git_hash_valid())
def test_hash_object_types_normalize_lowercase(running_server, object_type, object_value, resolved):
    env = running_server
    task = create_task(env, source_repo="github.com/acme/repo")

    status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
        "relation": "related",
        "object_type": object_type,
        "object_value": object_value,
        "resolved_commit": resolved,
    })

    assert status == 201
    assert created["object_value"] == object_value.lower()
    assert created["resolved_commit"] == resolved.lower()


@SETTINGS
@given(object_type=st.sampled_from(["commit", "blob", "tree"]), bad_hash=git_hash_invalid())
def test_hash_object_types_reject_invalid_hashes(running_server, object_type, bad_hash):
    env = running_server
    task = create_task(env, source_repo="github.com/acme/repo")

    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
        "relation": "related",
        "object_type": object_type,
        "object_value": bad_hash,
    })

    assert_error_contract(status, err, 400, "invalid_argument")


@SETTINGS
@given(path_value=repo_path_valid())
def test_path_object_type_normalizes_paths(running_server, path_value):
    env = running_server
    task = create_task(env, source_repo="github.com/acme/repo")

    status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
        "relation": "implements",
        "object_type": "path",
        "object_value": path_value,
    })

    assert status == 201
    assert created["object_value"] == posixpath.normpath(path_value.strip())


@SETTINGS
@given(path_value=repo_path_invalid())
def test_path_object_type_rejects_absolute_or_escaping_paths(running_server, path_value):
    env = running_server
    task = create_task(env, source_repo="github.com/acme/repo")

    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
        "relation": "implements",
        "object_type": "path",
        "object_value": path_value,
    })

    assert_error_contract(status, err, 400, "invalid_argument")


@SETTINGS
@given(
    object_type=st.sampled_from(["branch", "tag"]),
    good_ref=st.text(
        alphabet="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-",
        min_size=1,
        max_size=40,
    ).filter(lambda s: s.strip() and not any(ch.isspace() for ch in s)),
    bad_ref=st.sampled_from(["feat x", "tag\nname", "branch\tname"]),
)
def test_branch_tag_whitespace_rules(running_server, object_type, good_ref, bad_ref):
    env = running_server
    task = create_task(env, source_repo="github.com/acme/repo")

    status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
        "relation": "related",
        "object_type": object_type,
        "object_value": good_ref,
    })
    assert status == 201
    assert created["object_value"] == good_ref.strip()

    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
        "relation": "related",
        "object_type": object_type,
        "object_value": bad_ref,
    })
    assert_error_contract(status, err, 400, "invalid_argument")


# ---------------------------------------------------------------------------
# P0.5 Relation policy
# ---------------------------------------------------------------------------


@SETTINGS
@given(relation=git_relation_valid())
def test_relation_valid_values_normalize_and_pass(running_server, relation):
    env = running_server
    task = create_task(env, source_repo="github.com/acme/repo")

    status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
        "relation": relation,
        "object_type": "branch",
        "object_value": "main",
    })

    assert status == 201
    assert created["relation"] == relation.strip().lower()


@SETTINGS
@given(relation=git_relation_invalid())
def test_relation_invalid_values_rejected(running_server, relation):
    env = running_server
    task = create_task(env, source_repo="github.com/acme/repo")

    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
        "relation": relation,
        "object_type": "branch",
        "object_value": "main",
    })

    assert_error_contract(status, err, 400, "invalid_argument")


# ---------------------------------------------------------------------------
# P0.6 Dedupe uniqueness invariant
# ---------------------------------------------------------------------------


@SETTINGS
@given(repo_a=repo_slug_canonical(), repo_b=repo_slug_canonical(), h1=git_hash_valid(), h2=git_hash_valid(), h3=git_hash_valid())
def test_git_ref_dedupe_invariant(running_server, repo_a, repo_b, h1, h2, h3):
    env = running_server

    assume(repo_a != repo_b)
    assume(h1.lower() != h2.lower())
    assume(h2.lower() != h3.lower())
    assume(h1.lower() != h3.lower())

    task = create_task(env, source_repo=repo_a)

    base = {
        "repo": repo_a,
        "relation": "related",
        "object_type": "commit",
        "object_value": h1,
        "resolved_commit": h2,
    }

    status, _ = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", base)
    assert status == 201

    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", base)
    assert_error_contract(status, err, 409, "conflict")

    variants = [
        {**base, "repo": repo_b},
        {**base, "relation": "implements"},
        {**base, "object_type": "branch"},
        {**base, "object_value": h3},
        {**base, "resolved_commit": h3},
    ]

    for payload in variants:
        status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", payload)
        assert status == 201, created


# ---------------------------------------------------------------------------
# P0.7 Delete semantics
# ---------------------------------------------------------------------------


@SETTINGS
@given(data=st.data(), n_refs=st.integers(min_value=1, max_value=6))
def test_delete_semantics_remove_only_targeted_refs(running_server, data, n_refs):
    env = running_server
    task = create_task(env, source_repo="github.com/acme/repo")

    created_ids: list[str] = []
    for i in range(n_refs):
        status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
            "relation": "related",
            "object_type": "commit",
            "object_value": hash_for_index(i + 1),
        })
        assert status == 201
        created_ids.append(created["id"])

    delete_idx = data.draw(st.sets(st.integers(min_value=0, max_value=n_refs - 1), max_size=n_refs))
    deleted_ids = {created_ids[i] for i in delete_idx}
    remaining_ids = set(created_ids) - deleted_ids

    for ref_id in deleted_ids:
        status, _ = request_json(env, "DELETE", f"/v1/projects/gr/git-refs/{ref_id}")
        assert status == 200

    status, listed = request_json(env, "GET", f"/v1/projects/gr/tasks/{task['id']}/git-refs")
    assert status == 200
    listed_ids = {ref["id"] for ref in listed}
    assert deleted_ids.isdisjoint(listed_ids)
    assert remaining_ids == listed_ids

    for ref_id in deleted_ids:
        status, err = request_json(env, "GET", f"/v1/projects/gr/git-refs/{ref_id}")
        assert_error_contract(status, err, 404, "not_found")

    for ref_id in remaining_ids:
        status, _ = request_json(env, "GET", f"/v1/projects/gr/git-refs/{ref_id}")
        assert status == 200


# ---------------------------------------------------------------------------
# P0.8 Close annotation semantics
# ---------------------------------------------------------------------------


@SETTINGS
@given(data=st.data(), n_tasks=st.integers(min_value=1, max_value=4), commit=git_hash_valid(), include_repo=st.booleans())
def test_close_annotation_is_idempotent(running_server, data, n_tasks, commit, include_repo):
    env = running_server

    src_canonical, src_forms = data.draw(repo_slug_equivalent_forms())
    source_repo_input = data.draw(st.sampled_from(src_forms))

    ids = [create_task(env, source_repo=source_repo_input)["id"] for _ in range(n_tasks)]

    close_payload = {"ids": ids, "commit": commit}
    expected_repo = src_canonical

    if include_repo:
        req_canonical, req_forms = data.draw(repo_slug_equivalent_forms())
        close_payload["repo"] = data.draw(st.sampled_from(req_forms))
        expected_repo = req_canonical

    status, first = request_json(env, "POST", "/v1/projects/gr/tasks/close", close_payload)
    assert status == 200
    assert first["commit"] == commit.lower()
    assert first["annotated"] == n_tasks

    for task_id in ids:
        status, listed = request_json(env, "GET", f"/v1/projects/gr/tasks/{task_id}/git-refs")
        assert status == 200
        matching = [
            ref
            for ref in listed
            if ref["relation"] == "closed_by"
            and ref["object_type"] == "commit"
            and ref["object_value"] == commit.lower()
            and ref["repo"] == expected_repo
        ]
        assert len(matching) == 1

    status, second = request_json(env, "POST", "/v1/projects/gr/tasks/close", close_payload)
    assert status == 200
    assert second["annotated"] == 0

    for task_id in ids:
        status, listed = request_json(env, "GET", f"/v1/projects/gr/tasks/{task_id}/git-refs")
        assert status == 200
        sigs = [
            (
                ref["repo"],
                ref["relation"],
                ref["object_type"],
                ref["object_value"],
                ref.get("resolved_commit", ""),
            )
            for ref in listed
        ]
        assert len(sigs) == len(set(sigs))


def test_close_repo_without_commit_is_rejected(running_server):
    env = running_server
    task_id = create_task(env, source_repo="github.com/acme/repo")["id"]

    status, err = request_json(env, "POST", "/v1/projects/gr/tasks/close", {
        "ids": [task_id],
        "repo": "github.com/acme/repo",
    })
    assert_error_contract(status, err, 400, "invalid_argument")


def test_invalid_ids_and_missing_resources_for_git_refs(running_server):
    env = running_server

    # Invalid task/ref ids are rejected at validation layer.
    status, err = request_json(env, "POST", "/v1/projects/gr/tasks/bad-id/git-refs", {
        "repo": "github.com/acme/repo",
        "relation": "related",
        "object_type": "commit",
        "object_value": hash_for_index(1),
    })
    assert_error_contract(status, err, 400, "invalid_argument")

    status, err = request_json(env, "GET", "/v1/projects/gr/git-refs/bad-ref")
    assert_error_contract(status, err, 400, "invalid_argument")

    status, err = request_json(env, "DELETE", "/v1/projects/gr/git-refs/bad-ref")
    assert_error_contract(status, err, 400, "invalid_argument")

    # Missing task context returns not_found for list endpoint.
    status, err = request_json(env, "GET", "/v1/projects/gr/tasks/gr-zzzz/git-refs")
    assert_error_contract(status, err, 404, "not_found")


def test_close_with_invalid_commit_fails_and_does_not_close(running_server):
    env = running_server
    task_id = create_task(env, source_repo="github.com/acme/repo")["id"]

    status, err = request_json(env, "POST", "/v1/projects/gr/tasks/close", {
        "ids": [task_id],
        "commit": "not-a-hash",
    })
    assert_error_contract(status, err, 400, "invalid_argument")

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert shown["status"] == "open"


def test_invalid_repo_formats_rejected_on_create_and_close(running_server):
    env = running_server
    task_id = create_task(env, source_repo="github.com/acme/repo")["id"]

    bad_repo = "https://github.com/acme"

    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task_id}/git-refs", {
        "repo": bad_repo,
        "relation": "related",
        "object_type": "commit",
        "object_value": hash_for_index(2),
    })
    assert_error_contract(status, err, 400, "invalid_argument")

    status, err = request_json(env, "POST", "/v1/projects/gr/tasks/close", {
        "ids": [task_id],
        "commit": hash_for_index(3),
        "repo": bad_repo,
    })
    assert_error_contract(status, err, 400, "invalid_argument")

    shown = api_get(env, f"/v1/projects/gr/tasks/{task_id}")
    assert shown["status"] == "open"


def test_missing_required_fields_and_invalid_resolved_commit_rejected(running_server):
    env = running_server
    task_id = create_task(env, source_repo="github.com/acme/repo")["id"]

    payloads = [
        {"object_type": "branch", "object_value": "main"},  # missing relation
        {"relation": "related", "object_value": "main"},  # missing object_type
        {"relation": "related", "object_type": "branch"},  # missing object_value
    ]

    for payload in payloads:
        status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task_id}/git-refs", payload)
        assert_error_contract(status, err, 400, "invalid_argument")

    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task_id}/git-refs", {
        "relation": "related",
        "object_type": "commit",
        "object_value": hash_for_index(4),
        "resolved_commit": "deadbeef",
    })
    assert_error_contract(status, err, 400, "invalid_argument")


def test_dedupe_treats_empty_and_omitted_resolved_commit_as_equivalent(running_server):
    env = running_server
    task_id = create_task(env, source_repo="github.com/acme/repo")["id"]

    base = {
        "relation": "related",
        "object_type": "commit",
        "object_value": hash_for_index(5),
    }

    status, _ = request_json(env, "POST", f"/v1/projects/gr/tasks/{task_id}/git-refs", base)
    assert status == 201

    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task_id}/git-refs", {**base, "resolved_commit": ""})
    assert_error_contract(status, err, 409, "conflict")


# ---------------------------------------------------------------------------
# P1.1 Task deletion cascade
# ---------------------------------------------------------------------------


def test_task_delete_cascades_task_git_refs(running_server):
    env = running_server
    task = create_task(env, source_repo="github.com/acme/repo")

    created_ids = []
    for i in range(2):
        status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
            "relation": "related",
            "object_type": "commit",
            "object_value": hash_for_index(100 + i),
        })
        assert status == 201
        created_ids.append(created["id"])

    con = sqlite3.connect(env["GRNS_DB"])
    try:
        con.execute("PRAGMA foreign_keys = ON")
        con.execute("DELETE FROM tasks WHERE id = ?", (task["id"],))
        con.commit()
    finally:
        con.close()

    status, err = request_json(env, "GET", f"/v1/projects/gr/tasks/{task['id']}/git-refs")
    assert_error_contract(status, err, 404, "not_found")

    for ref_id in created_ids:
        status, err = request_json(env, "GET", f"/v1/projects/gr/git-refs/{ref_id}")
        assert_error_contract(status, err, 404, "not_found")


# ---------------------------------------------------------------------------
# P1.2 Repo catalog idempotency
# ---------------------------------------------------------------------------


@SETTINGS
@given(data=st.data(), n_tasks=st.integers(min_value=2, max_value=6))
def test_repo_catalog_idempotent_across_equivalent_repo_inputs(running_server, data, n_tasks):
    env = running_server

    canonical, forms = data.draw(repo_slug_equivalent_forms())

    for i in range(n_tasks):
        task = create_task(env, source_repo=forms[0])
        repo_form = data.draw(st.sampled_from(forms))
        status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {
            "repo": repo_form,
            "relation": "related",
            "object_type": "commit",
            "object_value": hash_for_index(200 + i),
        })
        assert status == 201
        assert created["repo"] == canonical

    con = sqlite3.connect(env["GRNS_DB"])
    try:
        row = con.execute("SELECT COUNT(*) FROM git_repos WHERE slug = ?", (canonical,)).fetchone()
        assert row is not None
        assert int(row[0]) == 1
    finally:
        con.close()


# ---------------------------------------------------------------------------
# P1.3 Note/meta roundtrip safety
# ---------------------------------------------------------------------------


@SETTINGS
@given(
    note1=NOTE_TEXT,
    note2=NOTE_TEXT,
    meta1=small_json_meta().filter(lambda m: len(m) > 0),
    meta2=small_json_meta().filter(lambda m: len(m) > 0),
)
def test_note_meta_roundtrip_and_dedupe_insensitivity(running_server, note1, note2, meta1, meta2):
    env = running_server

    assume(note1.strip() != note2.strip())
    assume(meta1 != meta2)

    task = create_task(env, source_repo="github.com/acme/repo")
    base = {
        "relation": "related",
        "object_type": "commit",
        "object_value": hash_for_index(300),
        "resolved_commit": hash_for_index(301),
        "note": note1,
        "meta": meta1,
    }

    status, created = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", base)
    assert status == 201
    assert created.get("note", "") == note1.strip()
    assert created.get("meta", {}) == meta1

    fetched = api_get(env, f"/v1/projects/gr/git-refs/{created['id']}")
    assert fetched.get("note", "") == note1.strip()
    assert fetched.get("meta", {}) == meta1

    listed = api_get(env, f"/v1/projects/gr/tasks/{task['id']}/git-refs")
    assert len(listed) == 1
    assert listed[0].get("note", "") == note1.strip()
    assert listed[0].get("meta", {}) == meta1

    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {**base, "note": note2})
    assert_error_contract(status, err, 409, "conflict")

    status, err = request_json(env, "POST", f"/v1/projects/gr/tasks/{task['id']}/git-refs", {**base, "meta": meta2})
    assert_error_contract(status, err, 409, "conflict")


# ---------------------------------------------------------------------------
# P1.4 Cross-task sharing
# ---------------------------------------------------------------------------


@SETTINGS
@given(repo=repo_slug_canonical(), commit=git_hash_valid())
def test_same_git_object_can_be_referenced_by_multiple_tasks(running_server, repo, commit):
    env = running_server

    t1 = create_task(env, source_repo=repo)
    t2 = create_task(env, source_repo=repo)

    payload = {
        "repo": repo,
        "relation": "related",
        "object_type": "commit",
        "object_value": commit,
    }

    status1, r1 = request_json(env, "POST", f"/v1/projects/gr/tasks/{t1['id']}/git-refs", payload)
    status2, r2 = request_json(env, "POST", f"/v1/projects/gr/tasks/{t2['id']}/git-refs", payload)

    assert status1 == 201
    assert status2 == 201
    assert r1["id"] != r2["id"]
