#!/usr/bin/env python3
"""Remind the session about submodule boundaries when editing inside repos/.

The clanker workspace is a parent of independently versioned repositories. A
file under repos/<project>/ belongs to that project's history, tooling, and
review process. This hook never blocks; it emits one notice per project per
session so the boundary is stated before the first edit lands.

Set BLINK_SKIP_BOUNDARY_NOTICE=1 to disable it.
"""

import hashlib
import json
import os
import sys
import tempfile
from pathlib import Path


def state_path(session_id, project):
    key = hashlib.sha256(f"{session_id}:{project}".encode()).hexdigest()[:16]
    return Path(tempfile.gettempdir()) / f"blink-boundary-{key}"


def owning_project(file_path):
    """Return (project, workspace_root) when the path is inside repos/<project>."""
    try:
        resolved = Path(file_path).resolve()
    except (OSError, ValueError):
        return None, None
    parts = resolved.parts
    for index in range(len(parts) - 1, 0, -1):
        if parts[index] == "repos" and index + 1 < len(parts):
            workspace = Path(*parts[:index])
            if (workspace / ".gitmodules").is_file():
                return parts[index + 1], workspace
    return None, None


def main():
    if os.environ.get("BLINK_SKIP_BOUNDARY_NOTICE") == "1":
        sys.exit(0)
    try:
        event = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        sys.exit(0)

    tool_input = event.get("tool_input") or {}
    file_path = tool_input.get("file_path") or tool_input.get("notebook_path") or ""
    if not file_path:
        sys.exit(0)

    project, workspace = owning_project(file_path)
    if not project:
        sys.exit(0)

    marker = state_path(event.get("session_id", ""), project)
    if marker.exists():
        sys.exit(0)
    try:
        marker.touch()
    except OSError:
        pass

    local_files = [
        name
        for name in ("AGENTS.md", "CLAUDE.md", "CONTRIBUTING.md", "Makefile")
        if (workspace / "repos" / project / name).is_file()
    ]
    hint = (
        " Read " + ", ".join(local_files) + " in that repository first."
        if local_files
        else ""
    )

    print(
        json.dumps(
            {
                "systemMessage": (
                    f"Blink Labs: editing inside the `{project}` submodule. Commit "
                    f"and validate there with its own tooling; the clanker parent "
                    f"should record only the resulting pointer.{hint}"
                )
            }
        )
    )
    sys.exit(0)


if __name__ == "__main__":
    main()
