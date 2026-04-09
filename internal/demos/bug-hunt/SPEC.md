# Task Tracker Specification

`tracker.py` provides three functions for managing a simple task list. Each function operates on a shared `tasks` list of dicts.

---

## `add_task(tasks, title, priority="medium")`

Adds a new task dict to `tasks` and returns the new task's ID.

- IDs are **1-indexed**: the first task added gets ID 1, the second gets ID 2, etc.
- `title` must not be empty or whitespace-only. Raise `ValueError` if it is.
- Default priority is `"medium"`. Valid priorities: `"low"`, `"medium"`, `"high"`.
- Each task is a dict: `{"id": int, "title": str, "priority": str, "done": False}`

---

## `get_high_priority(tasks)`

Returns all **undone** tasks with priority `"high"`, sorted by ID ascending.

- Only include tasks where `done` is `False`.
- Return a list sorted by `id`, lowest first.

---

## `completion_summary(tasks)`

Returns a dict summarizing progress:

```python
{"total": int, "done": int, "pending": int, "pct": float}
```

- `pct` is the percentage of tasks completed, rounded to one decimal place (0.0–100.0).
- If there are **no tasks**, `pct` should be **0.0** (not 100.0, not an error).
