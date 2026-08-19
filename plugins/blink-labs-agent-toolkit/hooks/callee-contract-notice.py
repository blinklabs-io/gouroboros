#!/usr/bin/env python3
"""Surface the documented constraints of functions a staged change calls.

Three review rounds in a row on one Blink Labs pull request found a defect whose
cause was already written down in the function the change called or wrapped: that
callers add the component prefix, that the sibling path clears its state when
exhausted, that the function releases its mutex around a network request. None of
it needed inferring. It needed reading.

So before a commit lands, this reprints the constraint sentences from the doc
comments of the functions the staged diff newly calls. It never blocks; a notice
that fires on every commit has to stay cheap to skim.

Set BLINK_SKIP_CALLEE_NOTICE=1 to disable it for a session.
"""

import json
import os
import re
import subprocess
import sys

# Words that mark a sentence as stating an obligation rather than describing
# behaviour. Kept deliberately narrow: a notice nobody reads is worse than none.
CONSTRAINT = re.compile(
    r"\b(must|never|do not|don't|caller|callers|releases|release[sd]? the lock"
    r"|only|requires|before|after|deadlock|assumes|invariant|unsafe)\b",
    re.IGNORECASE,
)

# Identifiers in added lines that look like calls. Method calls keep only the
# final selector, which is what a Go func declaration will name.
CALL = re.compile(r"(?:\.|\b)([A-Za-z_]\w*)\s*\(")

# Calls too generic to be worth resolving, plus common builtins and stdlib.
IGNORE = {
    "if", "for", "switch", "return", "func", "go", "defer", "range", "case",
    "make", "len", "cap", "append", "copy", "delete", "new", "panic", "recover",
    "print", "println", "close", "string", "int", "int64", "uint64", "byte",
    "bool", "error", "errors", "fmt", "Errorf", "New", "Sprintf", "Printf",
    "require", "assert", "t", "T", "NoError", "Equal", "True", "False", "Nil",
    "NotNil", "Empty", "Len", "Error", "Fatal", "Fatalf", "Run", "Lock",
    "Unlock", "RLock", "RUnlock", "Load", "Store", "Add", "Done", "Wait",
    "String", "Bytes", "Now", "Since", "Sprint", "Warn", "Info", "Debug",
}

MAX_FUNCS = 6
MAX_SENTENCES = 2


def run(args, cwd):
    try:
        result = subprocess.run(
            args, cwd=cwd or None, capture_output=True, text=True, timeout=8
        )
    except (OSError, subprocess.SubprocessError):
        return ""
    return result.stdout if result.returncode == 0 else ""


def staged_added_lines(cwd):
    diff = run(["git", "diff", "--cached", "-U0"], cwd)
    return [
        line[1:]
        for line in diff.split("\n")
        if line.startswith("+") and not line.startswith("+++")
    ]


def funcs_defined_in_diff(added_lines):
    """Names this change itself declares.

    Their doc comments are the change's own prose, so echoing them back says
    nothing. What matters is the contract of the code being called into.
    """
    names = set()
    for line in added_lines:
        match = re.match(r"func\s+(?:\([^)]*\)\s*)?([A-Za-z_]\w*)", line)
        if match:
            names.add(match.group(1))
    return names


def staged_go_dirs(cwd):
    names = run(["git", "diff", "--cached", "--name-only"], cwd)
    dirs = set()
    for name in names.split("\n"):
        if name.endswith(".go"):
            dirs.add(os.path.dirname(name) or ".")
    return sorted(dirs)


DELEGATE = re.compile(r"^\s*return\s+\w+\.([A-Za-z_]\w*)\(")


def contract_text(path, func_name, cwd, hop=True):
    """Return the constraint prose attached to func_name, and its delegate.

    Reads three places, because this codebase puts obligations in all of them:
    the doc comment above the declaration, the leading comment block inside the
    body ("The caller owns chainsyncBlockfetchMutex..."), and -- for a thin
    wrapper whose body is a single delegating return -- the same two places on
    the function it delegates to. Without that last hop the notice misses
    exactly the case it was written for: startQueuedBlockfetchLocked carries no
    comment of its own, while the delegate it forwards to is where "never hold
    it across the network request" is written down.
    """
    try:
        with open(os.path.join(cwd or ".", path), encoding="utf-8") as handle:
            lines = handle.read().split("\n")
    except OSError:
        return ""
    pattern = re.compile(
        r"^func\s+(\([^)]*\)\s*)?" + re.escape(func_name) + r"\s*[(\[]"
    )
    for index, line in enumerate(lines):
        if not pattern.match(line):
            continue
        above = []
        cursor = index - 1
        while cursor >= 0 and lines[cursor].startswith("//"):
            above.append(lines[cursor][2:].strip())
            cursor -= 1
        # Walk to the end of the signature, then take the leading body comment.
        body = index
        while body < len(lines) and not lines[body].rstrip().endswith("{"):
            body += 1
        inside, delegate = [], ""
        cursor = body + 1
        while cursor < len(lines) and lines[cursor].strip().startswith("//"):
            inside.append(lines[cursor].strip()[2:].strip())
            cursor += 1
        if cursor < len(lines):
            match = DELEGATE.match(lines[cursor])
            if match:
                delegate = match.group(1)
        text = " ".join(reversed(above)) + " " + " ".join(inside)
        if hop and delegate and delegate != func_name:
            text += " " + contract_text(path, delegate, cwd, hop=False)
        return text.strip()
    return ""


def constraint_sentences(comment):
    out = []
    for sentence in re.split(r"(?<=[.;])\s+", comment):
        sentence = sentence.strip()
        # A bare mention is not a constraint; require some substance.
        if len(sentence) > 24 and CONSTRAINT.search(sentence):
            out.append(sentence)
    return out[:MAX_SENTENCES]


def main():
    if os.environ.get("BLINK_SKIP_CALLEE_NOTICE") == "1":
        sys.exit(0)
    try:
        event = json.load(sys.stdin)
    except (json.JSONDecodeError, ValueError):
        sys.exit(0)

    command = (event.get("tool_input") or {}).get("command") or ""
    if not re.search(r"\bgit\b.*\bcommit\b", command):
        sys.exit(0)

    cwd = event.get("cwd", "")
    dirs = staged_go_dirs(cwd)
    if not dirs:
        sys.exit(0)

    added = staged_added_lines(cwd)
    own = funcs_defined_in_diff(added)
    called = []
    for line in added:
        if line.lstrip().startswith("//"):
            continue
        for name in CALL.findall(line):
            if name in IGNORE or name in own or name in called:
                continue
            called.append(name)
    if not called:
        sys.exit(0)

    # Resolve only against the packages the change already touches. That is
    # where a wrapped or cited neighbour lives, and it keeps this to one grep.
    listing = run(["git", "ls-files", "--", *dirs], cwd)
    go_files = [f for f in listing.split("\n") if f.endswith(".go")]
    if not go_files:
        sys.exit(0)

    findings = []
    for name in called:
        if len(findings) >= MAX_FUNCS:
            break
        for path in go_files:
            comment = contract_text(path, name, cwd)
            if not comment:
                continue
            sentences = constraint_sentences(comment)
            if sentences:
                findings.append((name, sentences))
            break

    if not findings:
        sys.exit(0)

    lines = [
        "Blink Labs: functions this change calls document constraints. Confirm "
        "the change honors them — three review rounds on dingo#3214/#3219/#3221 "
        "each found a defect already stated here:",
    ]
    for name, sentences in findings:
        lines.append(f"- {name}: " + " ".join(sentences))
    print(json.dumps({"systemMessage": "\n".join(lines)}))
    sys.exit(0)


if __name__ == "__main__":
    main()
