#!/usr/bin/env python3
"""Tell the parent session to dispatch the reviewer when the developer stops.

blink-tdd-developer stops at a signed local commit and has no way to dispatch
anything: a subagent has no Agent tool, so the developer-to-reviewer handoff is
otherwise only a request in its report that the parent may or may not act on.
This hook closes that gap from the harness side, where it does not depend on
the parent model remembering.

Which payload field names the stopping agent is not under this repository's
control, so every plausible key is checked and the agent's own marker line is
used as a fallback. The notice never denies or interrupts a stop; it adds
context and exits zero on anything it does not understand.

Set BLINK_SKIP_HANDOFF_NOTICE=1 to disable it. Set BLINK_HANDOFF_HOOK_DEBUG to
a file path to append each payload verbatim when confirming field names against
a real harness.
"""

import json
import os
import sys

DEVELOPER = "blink-tdd-developer"
REVIEWER = "blink-review-shepherd"
MARKER = "HANDOFF:"

# The harness may name the stopping agent under any of these, and may namespace
# the value with its plugin ("blink-labs-agent-toolkit:blink-tdd-developer").
IDENTITY_KEYS = (
    "agent_type",
    "subagent_type",
    "agent_name",
    "subagent_name",
    "agent",
    "subagent",
)

NOTICE = (
    f"Blink Labs: {DEVELOPER} finished at a signed local commit. Dispatch "
    f"{REVIEWER} next with that handoff report — pre-publication review runs "
    f"before any push or `gh pr create`, and publication still needs explicit "
    f"authorization."
)


def debug(raw):
    path = os.environ.get("BLINK_HANDOFF_HOOK_DEBUG")
    if not path:
        return
    try:
        with open(path, "a", encoding="utf-8") as handle:
            handle.write(raw.strip() + "\n")
    except OSError:
        pass


def stopping_agent(event):
    """Return the lowercased identity of the agent that stopped, or ''."""
    for key in IDENTITY_KEYS:
        value = event.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip().lower()
    return ""


def final_assistant_text(path):
    """Return the text of the last assistant message in a JSONL transcript."""
    try:
        with open(path, encoding="utf-8") as handle:
            lines = handle.readlines()
    except (OSError, ValueError):
        return ""
    for line in reversed(lines):
        try:
            entry = json.loads(line)
        except (json.JSONDecodeError, ValueError):
            continue
        if not isinstance(entry, dict) or entry.get("type") != "assistant":
            continue
        message = entry.get("message")
        content = message.get("content") if isinstance(message, dict) else None
        if isinstance(content, str):
            return content
        if isinstance(content, list):
            return "\n".join(
                block.get("text", "")
                for block in content
                if isinstance(block, dict) and block.get("type") == "text"
            )
    return ""


def marker_present(text):
    """True when a line of its own hands off to the reviewer.

    Line-anchored on purpose: an agent quoting the instruction in prose is
    describing the handoff, not performing one.
    """
    for line in text.splitlines():
        stripped = line.strip().lstrip("*_` ")
        if stripped.upper().startswith(MARKER) and REVIEWER in stripped.lower():
            return True
    return False


def should_notify(event):
    agent = stopping_agent(event)
    if agent:
        # An identified agent settles it either way; the reviewer's own stop
        # must not dispatch another reviewer.
        return DEVELOPER in agent
    transcript = event.get("transcript_path")
    if isinstance(transcript, str) and transcript:
        return marker_present(final_assistant_text(transcript))
    return False


def main():
    if os.environ.get("BLINK_SKIP_HANDOFF_NOTICE") == "1":
        sys.exit(0)
    raw = sys.stdin.read()
    debug(raw)
    try:
        event = json.loads(raw)
    except (json.JSONDecodeError, ValueError):
        sys.exit(0)
    if not isinstance(event, dict) or not should_notify(event):
        sys.exit(0)

    print(
        json.dumps(
            {
                "systemMessage": NOTICE,
                "hookSpecificOutput": {
                    "hookEventName": "SubagentStop",
                    "additionalContext": NOTICE,
                },
            }
        )
    )
    sys.exit(0)


if __name__ == "__main__":
    main()
