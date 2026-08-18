#!/usr/bin/env python3
"""Guard `git commit` in Blink Labs repositories.

Blink Labs requires Conventional Commits and a DCO sign-off (`git commit -s`)
in every repository. This hook denies a `git commit` that would violate either
rule, and warns when a local plan or scratch file is about to be committed.

Set BLINK_SKIP_COMMIT_GUARD=1 to disable it for a session.
"""

import json
import os
import re
import shlex
import subprocess
import sys

CONVENTIONAL = re.compile(
    r"^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)"
    r"(\([^()\n]+\))?!?: .+"
)

PLAN_FILE = re.compile(
    r"(^|/)(plan|plans|PLAN|PLANS|scratch|notes)(/|\.md$)"
    r"|(^|/)[^/]*[-_](plan|plans|notes|scratch)\.md$"
    r"|(^|/)(TODO|NOTES|HANDOFF|SESSION)[^/]*\.md$"
)


def emit(payload):
    print(json.dumps(payload))
    sys.exit(0)


def deny(reason):
    emit(
        {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "deny",
                "permissionDecisionReason": reason,
            }
        }
    )


def allow_with_note(note):
    emit({"systemMessage": note})


HEREDOC = re.compile(
    r"<<-?[ \t]*['\"]?([A-Za-z_][A-Za-z0-9_]*)['\"]?[ \t]*\r?\n(.*?)\r?\n[ \t]*\1\b",
    re.S,
)

UNRESOLVED = re.compile(r"\$\(|`|\$\{")


def extract_heredocs(command):
    """Strip heredoc bodies out of a command and return them separately.

    Heredoc bodies contain arbitrary text that would otherwise be tokenized as
    shell words, so they are removed before parsing and reattached as candidate
    commit messages.
    """
    bodies = []

    def replace(match):
        bodies.append(match.group(2))
        return "<<" + match.group(1)

    return HEREDOC.sub(replace, command), bodies


def commit_segments(command):
    """Return argv lists for each `git commit` invocation in the command."""
    try:
        tokens = shlex.split(command, comments=False)
    except ValueError:
        return []
    segments, current = [], []
    for token in tokens:
        if token in ("&&", "||", ";", "|"):
            segments.append(current)
            current = []
        else:
            current.append(token)
    segments.append(current)

    found = []
    for segment in segments:
        if not segment:
            continue
        argv = segment[1:] if segment[0] == "sudo" else segment
        if not argv or os.path.basename(argv[0]) != "git":
            continue
        rest = argv[1:]
        # Skip global options like -C <dir> to find the subcommand.
        i = 0
        while i < len(rest) and rest[i].startswith("-"):
            if rest[i] in ("-C", "-c", "--git-dir", "--work-tree"):
                i += 2
            else:
                i += 1
        if i < len(rest) and rest[i] == "commit":
            found.append(rest[i + 1 :])
    return found


def parse_commit_args(argv):
    """Return (messages, flags) from a `git commit` argument list.

    Handles bundled short options (`-sm "msg"`), attached values (`-m"msg"`),
    and long options with or without `=`.
    """
    msgs, flags = [], set()
    i = 0
    while i < len(argv):
        arg = argv[i]
        if arg == "--":
            break
        if arg.startswith("--"):
            name, _, value = arg.partition("=")
            flags.add(name)
            if name in ("--message", "--file", "--reuse-message") and not value:
                if i + 1 < len(argv):
                    value = argv[i + 1]
                    i += 1
            if name == "--message" and value:
                msgs.append(value)
        elif arg.startswith("-") and len(arg) > 1:
            body = arg[1:]
            for pos, char in enumerate(body):
                if char in ("m", "F", "C", "c"):
                    value = body[pos + 1 :]
                    if not value and i + 1 < len(argv):
                        value = argv[i + 1]
                        i += 1
                    if char == "m" and value:
                        msgs.append(value)
                    flags.add("-" + char)
                    break
                flags.add("-" + char)
        i += 1
    return msgs, flags


def head_message(cwd):
    """Return the current HEAD commit message, or "" when unavailable."""
    try:
        result = subprocess.run(
            ["git", "log", "-1", "--format=%B"],
            cwd=cwd or None,
            capture_output=True,
            text=True,
            timeout=5,
        )
    except (OSError, subprocess.SubprocessError):
        return ""
    return result.stdout if result.returncode == 0 else ""


def staged_plan_files(cwd):
    try:
        result = subprocess.run(
            ["git", "diff", "--cached", "--name-only"],
            cwd=cwd or None,
            capture_output=True,
            text=True,
            timeout=5,
        )
    except (OSError, subprocess.SubprocessError):
        return []
    if result.returncode != 0:
        return []
    return [p for p in result.stdout.split("\n") if p and PLAN_FILE.search(p)]


def main():
    if os.environ.get("BLINK_SKIP_COMMIT_GUARD") == "1":
        sys.exit(0)
    try:
        event = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        sys.exit(0)

    command = (event.get("tool_input") or {}).get("command") or ""
    if "commit" not in command:
        sys.exit(0)

    stripped, heredocs = extract_heredocs(command)
    invocations = commit_segments(stripped)
    if not invocations:
        sys.exit(0)

    problems = []
    for argv in invocations:
        msgs, flags = parse_commit_args(argv)
        subjects = []
        for msg in msgs:
            if UNRESOLVED.search(msg):
                # The message comes from a substitution. Use the heredoc body
                # when there is exactly one; otherwise the text is unknowable
                # here and the subject rules are not enforced.
                if len(heredocs) == 1:
                    subjects.append(heredocs[0].split("\n", 1)[0].strip())
                continue
            subjects.append(msg.split("\n", 1)[0].strip())
        if not msgs and len(heredocs) == 1 and "-F" in flags:
            subjects.append(heredocs[0].split("\n", 1)[0].strip())
        amending = "--amend" in flags
        reusing = amending and not msgs and not any(
            flag in flags for flag in ("-F", "--file", "-C", "--reuse-message")
        )
        existing = head_message(event.get("cwd", "")) if reusing else ""
        if reusing and existing:
            # The message is inherited from HEAD; check that, not the flags.
            subjects.append(existing.split("\n", 1)[0].strip())
        signed = "-s" in flags or "--signoff" in flags
        if not signed and not any(
            "Signed-off-by:" in text for text in msgs + heredocs + [existing]
        ):
            problems.append(
                "missing DCO sign-off: add `-s` (or a Signed-off-by trailer). "
                "Every Blink Labs repository requires it."
            )
        for subject in subjects:
            if amending and not subject:
                continue
            if not CONVENTIONAL.match(subject):
                problems.append(
                    f"subject {subject!r} is not a Conventional Commit. Use "
                    "`type(scope): summary` with one of build, chore, ci, docs, "
                    "feat, fix, perf, refactor, revert, style, test."
                )
            elif len(subject) > 72:
                problems.append(
                    f"subject is {len(subject)} characters. Keep commit subjects "
                    "short and factual (72 characters or fewer)."
                )

    if problems:
        deny(
            "Blink Labs commit policy blocked this command:\n- "
            + "\n- ".join(dict.fromkeys(problems))
            + "\n\nFix the command and retry. Set BLINK_SKIP_COMMIT_GUARD=1 only "
            "when the user has explicitly asked to bypass this policy."
        )

    plans = staged_plan_files(event.get("cwd", ""))
    if plans:
        allow_with_note(
            "Blink Labs: these staged paths look like local planning artifacts, "
            "which are never committed — "
            + ", ".join(plans[:10])
            + ". Unstage them, or use a repository issue for durable tracking."
        )
    sys.exit(0)


if __name__ == "__main__":
    main()
