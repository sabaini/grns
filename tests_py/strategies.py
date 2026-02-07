"""Hypothesis strategies for generating valid (and invalid) grns data."""

import string

from hypothesis import strategies as st

# ---------------------------------------------------------------------------
# Domain constants (mirrors internal/models/domain.go)
# ---------------------------------------------------------------------------

VALID_STATUSES = ["open", "in_progress", "blocked", "deferred", "closed", "pinned", "tombstone"]
VALID_TYPES = ["bug", "feature", "task", "epic", "chore"]
PRIORITY_MIN = 0
PRIORITY_MAX = 4
ID_SUFFIX_CHARS = string.digits + string.ascii_lowercase  # [0-9a-z]

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


@st.composite
def _random_case(draw: st.DrawFn, s: str) -> str:
    """Randomly upper/lower-case each character in s."""
    return "".join(
        c.upper() if draw(st.booleans()) else c.lower()
        for c in s
    )


# ---------------------------------------------------------------------------
# Atomic strategies
# ---------------------------------------------------------------------------


def valid_ids(prefix: str = "gr") -> st.SearchStrategy[str]:
    """IDs matching ^[a-z]{2}-[0-9a-z]{4}$."""
    return st.text(alphabet=ID_SUFFIX_CHARS, min_size=4, max_size=4).map(
        lambda suffix: f"{prefix}-{suffix}"
    )


def valid_titles() -> st.SearchStrategy[str]:
    """Non-empty strings that survive trimming."""
    return st.text(min_size=1, max_size=120).filter(lambda s: s.strip())


def printable_titles() -> st.SearchStrategy[str]:
    """Non-empty printable strings (no control chars). Useful for tests that
    compare Python vs Go whitespace semantics."""
    alphabet = st.characters(whitelist_categories=("L", "M", "N", "P", "S", "Z"))
    return st.text(alphabet=alphabet, min_size=1, max_size=80).filter(lambda s: s.strip())


def valid_statuses() -> st.SearchStrategy[str]:
    return st.sampled_from(VALID_STATUSES)


def valid_types() -> st.SearchStrategy[str]:
    return st.sampled_from(VALID_TYPES)


def valid_priorities() -> st.SearchStrategy[int]:
    return st.integers(min_value=PRIORITY_MIN, max_value=PRIORITY_MAX)


def valid_labels() -> st.SearchStrategy[str]:
    """ASCII-only, no spaces, non-empty, lowercase labels."""
    label_chars = string.ascii_lowercase + string.digits + "-_"
    return st.text(alphabet=label_chars, min_size=1, max_size=30).filter(lambda s: s.strip())


def valid_label_lists(min_size: int = 0, max_size: int = 5) -> st.SearchStrategy[list[str]]:
    """Lists of unique valid labels."""
    return st.lists(valid_labels(), min_size=min_size, max_size=max_size, unique=True)


def mixed_case_label_lists(min_size: int = 1, max_size: int = 6) -> st.SearchStrategy[list[str]]:
    """Label lists that may contain duplicates and mixed case — for testing
    that the server normalizes (lowercases, deduplicates, sorts) labels."""
    label_chars = string.ascii_letters + string.digits + "-_"
    raw_label = st.text(alphabet=label_chars, min_size=1, max_size=20).filter(lambda s: s.strip())
    return st.lists(raw_label, min_size=min_size, max_size=max_size)


# ---------------------------------------------------------------------------
# Invalid strategies (for rejection testing)
# ---------------------------------------------------------------------------


def invalid_priorities() -> st.SearchStrategy[int]:
    """Integers outside the valid 0-4 range."""
    return st.one_of(
        st.integers(max_value=PRIORITY_MIN - 1),
        st.integers(min_value=PRIORITY_MAX + 1),
    )


def invalid_statuses() -> st.SearchStrategy[str]:
    """Strings that are not valid statuses."""
    return st.text(min_size=1, max_size=20).filter(
        lambda s: s.strip().lower() not in VALID_STATUSES
    )


def invalid_types() -> st.SearchStrategy[str]:
    """Strings that are not valid types."""
    return st.text(min_size=1, max_size=20).filter(
        lambda s: s.strip().lower() not in VALID_TYPES
    )


# ---------------------------------------------------------------------------
# Case-varied strategies (for normalization testing)
# ---------------------------------------------------------------------------


def case_varied_statuses() -> st.SearchStrategy[str]:
    """Valid statuses with truly random per-character casing."""
    return valid_statuses().flatmap(lambda s: _random_case(s))


def case_varied_types() -> st.SearchStrategy[str]:
    """Valid types with truly random per-character casing."""
    return valid_types().flatmap(lambda t: _random_case(t))
