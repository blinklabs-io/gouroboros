#!/usr/bin/env python3
"""Brief a new session on the state of the Blink Labs clanker workspace.

Reports which repositories have uncommitted changes and which submodule
pointers have moved, so a session starts from real state instead of assuming a
clean tree. Silent outside the workspace.

Set BLINK_SKIP_WORKSPACE_BRIEF=1 to disable it.
"""

import json
import os
import subprocess
import sys
from pathlib import Path

MAX_LISTED = 12


def run(args, cwd):
    try:
        result = subprocess.run(
            args, cwd=cwd, capture_output=True, text=True, timeout=15
        )
    except (OSError, subprocess.SubprocessError):
        return None
    return result.stdout if result.returncode == 0 else None


def workspace_root(start):
    path = Path(start).resolve()
    for candidate in [path, *path.parents]:
        if (candidate / ".gitmodules").is_file() and (candidate / "repos").is_dir():
            return candidate
    return None


def main():
    if os.environ.get("BLINK_SKIP_WORKSPACE_BRIEF") == "1":
        sys.exit(0)
    try:
        event = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        event = {}

    root = workspace_root(event.get("cwd") or os.getcwd())
    if root is None:
        sys.exit(0)

    status = run(["git", "submodule", "status", "--recursive"], root)
    if status is None:
        sys.exit(0)

    uninitialized, moved = [], []
    for line in status.splitlines():
        if not line:
            continue
        marker, rest = line[0], line[1:].strip()
        name = rest.split(" ")[1] if " " in rest else rest
        if marker == "-":
            uninitialized.append(name)
        elif marker == "+":
            moved.append(name)

    dirty = []
    parent_status = run(["git", "status", "--short"], root) or ""
    for line in parent_status.splitlines():
        path = line[3:].strip()
        if path.startswith("repos/"):
            dirty.append(path.rstrip("/"))

    lines = [f"Blink Labs workspace: {root}"]
    if uninitialized:
        lines.append(
            f"Uninitialized submodules ({len(uninitialized)}): "
            + ", ".join(uninitialized[:MAX_LISTED])
            + (" ..." if len(uninitialized) > MAX_LISTED else "")
            + ". Run `git submodule update --init --recursive` before assuming a "
            "repository is missing a feature."
        )
    if moved:
        lines.append(
            f"Submodule pointers already moved ({len(moved)}): "
            + ", ".join(moved[:MAX_LISTED])
            + (" ..." if len(moved) > MAX_LISTED else "")
            + ". Do not commit these unless the task asked for them."
        )
    if dirty:
        lines.append(
            "Submodules with working-tree changes: "
            + ", ".join(sorted(set(dirty))[:MAX_LISTED])
        )
    if len(lines) == 1:
        lines.append("Submodules are initialized and pointers are unchanged.")
    lines.append(
        "Source changes belong in the owning submodule; the parent records the "
        "pointer. Conventional Commits and `git commit -s` are required."
    )

    print(
        json.dumps(
            {
                "hookSpecificOutput": {
                    "hookEventName": "SessionStart",
                    "additionalContext": "\n".join(lines),
                }
            }
        )
    )
    sys.exit(0)


if __name__ == "__main__":
    main()
