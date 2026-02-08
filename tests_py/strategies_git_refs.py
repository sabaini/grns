"""Hypothesis strategies for task git-ref property tests."""

from __future__ import annotations

import re
import string

from hypothesis import strategies as st

GIT_OBJECT_TYPES = ["commit", "tag", "branch", "path", "blob", "tree"]
GIT_RELATION_BUILTINS = [
    "design_doc",
    "implements",
    "fix_commit",
    "closed_by",
    "introduced_by",
    "related",
]


def _random_case_strategy(value: str) -> st.SearchStrategy[str]:
    chars = [st.sampled_from([c.lower(), c.upper()]) for c in value]
    return st.tuples(*chars).map("".join)


def git_object_types() -> st.SearchStrategy[str]:
    return st.sampled_from(GIT_OBJECT_TYPES)


def git_hash_valid() -> st.SearchStrategy[str]:
    """40-char git hashes with mixed-case hex characters."""
    return st.text(alphabet="0123456789abcdefABCDEF", min_size=40, max_size=40)


def git_hash_invalid() -> st.SearchStrategy[str]:
    """Values that should fail git-hash validation (after trim/lower)."""

    definitely_invalid = st.one_of(
        st.text(alphabet="0123456789abcdefABCDEF", min_size=0, max_size=39),
        st.text(alphabet="0123456789abcdefABCDEF", min_size=41, max_size=64),
        st.builds(
            lambda a, b: a + "g" + b,
            st.text(alphabet="0123456789abcdefABCDEF", min_size=20, max_size=20),
            st.text(alphabet="0123456789abcdefABCDEF", min_size=19, max_size=19),
        ),
        st.sampled_from(["zzzz", "a" * 39 + "-", "a" * 20 + " " + "b" * 19]),
    )

    valid_after_trim = re.compile(r"^[0-9a-fA-F]{40}$")
    return definitely_invalid.filter(lambda s: valid_after_trim.fullmatch(s.strip()) is None)


def git_relation_valid() -> st.SearchStrategy[str]:
    """Built-ins (with random case) plus x-* extension relations."""

    builtins = st.sampled_from(GIT_RELATION_BUILTINS).flatmap(_random_case_strategy)
    custom = st.text(
        alphabet=string.ascii_lowercase + string.digits + "_-",
        min_size=1,
        max_size=20,
    ).map(lambda tail: f"x-{tail}")
    custom_case_varied = custom.flatmap(_random_case_strategy)
    return st.one_of(builtins, custom_case_varied)


def git_relation_invalid() -> st.SearchStrategy[str]:
    """Relation strings rejected by relation policy."""
    return st.one_of(
        st.sampled_from(["", " ", "x", "custom", "y-foo", "bad relation", "x*bad", "design.doc", "1abc", "_abc"]),
        st.text(min_size=1, max_size=20).filter(
            lambda s: not _relation_is_valid(s)
        ),
    )


def _relation_is_valid(raw: str) -> bool:
    relation = raw.strip().lower()
    if not relation:
        return False
    if re.fullmatch(r"[a-z][a-z0-9_-]*", relation) is None:
        return False
    return relation in GIT_RELATION_BUILTINS or relation.startswith("x-")


def repo_slug_canonical() -> st.SearchStrategy[str]:
    host_label = st.text(alphabet=string.ascii_lowercase + string.digits + "-", min_size=1, max_size=10)
    host = st.tuples(host_label, host_label).map(lambda parts: f"{parts[0]}.{parts[1]}")
    path_part = st.text(alphabet=string.ascii_lowercase + string.digits + "_-", min_size=1, max_size=12)
    return st.tuples(host, path_part, path_part).map(lambda parts: f"{parts[0]}/{parts[1]}/{parts[2]}")


@st.composite
def repo_slug_equivalent_forms(draw: st.DrawFn) -> tuple[str, list[str]]:
    canonical = draw(repo_slug_canonical())
    host, owner, name = canonical.split("/")

    maybe_case = draw(st.booleans())
    if maybe_case:
        host_form = draw(_random_case_strategy(host))
        owner_form = draw(_random_case_strategy(owner))
        name_form = draw(_random_case_strategy(name))
    else:
        host_form, owner_form, name_form = host, owner, name

    return canonical, [
        f"https://{host_form}/{owner_form}/{name_form}",
        f"git@{host_form}:{owner_form}/{name_form}.git",
        f"{host_form}/{owner_form}/{name_form}",
    ]


@st.composite
def repo_path_valid(draw: st.DrawFn) -> str:
    segment = st.text(
        alphabet=string.ascii_lowercase + string.digits + "._-",
        min_size=1,
        max_size=12,
    ).filter(lambda s: s not in {".", ".."})

    parts = draw(st.lists(segment, min_size=1, max_size=5))
    style = draw(st.sampled_from(["plain", "double", "dot"]))

    if style == "plain" or len(parts) == 1:
        return "/".join(parts)
    if style == "double":
        i = draw(st.integers(min_value=1, max_value=len(parts) - 1))
        return "/".join(parts[:i]) + "//" + "/".join(parts[i:])

    i = draw(st.integers(min_value=1, max_value=len(parts) - 1))
    return "/".join(parts[:i]) + "/./" + "/".join(parts[i:])


def repo_path_invalid() -> st.SearchStrategy[str]:
    return st.sampled_from([
        "/a",
        "/tmp/file",
        "../a",
        "a/../../b",
        "..",
        "./../a",
        "../../etc/passwd",
    ])


def small_json_meta() -> st.SearchStrategy[dict[str, object]]:
    key = st.text(alphabet=string.ascii_lowercase + string.digits + "_", min_size=1, max_size=16)
    scalar = st.one_of(
        st.none(),
        st.booleans(),
        st.integers(min_value=-10_000, max_value=10_000),
        st.text(max_size=40),
    )
    value = st.one_of(
        scalar,
        st.lists(scalar, max_size=4),
        st.dictionaries(key, scalar, max_size=4),
    )
    return st.dictionaries(key, value, min_size=0, max_size=5)
