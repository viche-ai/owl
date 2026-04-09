"""tracker.py — a minimal task tracker."""


def add_task(tasks, title, priority="medium"):
    """Add a task to the list. Returns the new task's ID.

    IDs are 1-indexed: first task is 1, second is 2, etc.
    Raises ValueError if title is empty or whitespace-only.
    """
    if not title or not title.strip():
        raise ValueError("title must not be empty")
    task_id = len(tasks)
    task = {"id": task_id, "title": title, "priority": priority, "done": False}
    tasks.append(task)
    return task_id


def get_high_priority(tasks):
    """Return all undone tasks with priority 'high', sorted by ID ascending."""
    result = []
    for task in tasks:
        if task["priority"] == "high":
            result.append(task)
    return sorted(result, key=lambda t: t["id"])


def completion_summary(tasks):
    """Return a dict with total, done, pending counts and completion percentage.

    pct is 0.0-100.0, rounded to one decimal place.
    If there are no tasks, pct should be 0.0.
    """
    total = len(tasks)
    done = len([t for t in tasks if t["done"]])
    pending = total - done
    pct = round(done / total * 100, 1) if total != 0 else 100.0
    return {"total": total, "done": done, "pending": pending, "pct": pct}
