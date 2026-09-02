#!/usr/bin/env python3
"""Reject generic squash-merge subjects in GitHub merge commands."""

import json
import os
import shlex
import sys

GENERIC_SUBJECTS = {
    "merge approved changes",
    "merge pr",
    "squash merge",
    "approved changes",
}


def deny(reason):
    print(
        json.dumps(
            {
                "hookSpecificOutput": {
                    "hookEventName": "PreToolUse",
                    "permissionDecision": "deny",
                    "permissionDecisionReason": reason,
                }
            }
        )
    )


def main():
    if os.environ.get("BLINK_SKIP_MERGE_GUARD") == "1":
        return
    try:
        event = json.load(sys.stdin)
        command = (event.get("tool_input") or {}).get("command") or ""
        tokens = shlex.split(command, comments=False)
    except (json.JSONDecodeError, ValueError):
        return

    for index, token in enumerate(tokens):
        if token != "merge" or index < 1 or tokens[index - 1] != "pr":
            continue
        merge_args = tokens[index + 1 :]
        for option_index, option in enumerate(merge_args):
            if option == "--":
                break
            if option.startswith("--subject="):
                subject = option.partition("=")[2]
            elif option == "--subject":
                subject = ""
                if option_index + 1 < len(merge_args):
                    subject = merge_args[option_index + 1]
            else:
                continue
            if subject.strip().casefold() in GENERIC_SUBJECTS:
                deny(
                    "generic squash subject blocked: use the exact PR title as "
                    "the subject and a short factual body. Set "
                    "BLINK_SKIP_MERGE_GUARD=1 only with explicit owner approval."
                )
            break


if __name__ == "__main__":
    main()
