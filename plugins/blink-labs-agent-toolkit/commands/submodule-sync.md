---
description: "Safely inspect, update, and record Blink Labs submodule pointers without crossing repository boundaries"
argument-hint: "[submodule name or 'status']"
allowed-tools: ["Bash", "Glob", "Grep", "Read"]
---

# Submodule pointer workflow

Target: "$ARGUMENTS" (if empty, report status for the whole workspace).

The `clanker` parent is a workspace. Source changes belong in the submodule; the
parent normally records only the resulting pointer.

## Steps

1. Report current state: `git submodule status --recursive` and
   `git diff --submodule=log` for anything already moved. Distinguish an
   intentional pointer change from an incidental one caused by a stray checkout.
2. For each submodule in scope, show what the pointer move actually contains —
   the commit range, not just the SHA. Never advance a pointer whose range you
   have not looked at.
3. Keep source changes and pointer updates in separate commits unless the user
   asked otherwise. Never commit inside a submodule from the parent repository
   as a side effect.
4. Leave untouched: unrelated submodules, untracked files, local agent state,
   and any nested worktree. Do not re-pin submodules the task did not name.
5. Verify the result: the parent diff should show only `Subproject commit` lines
   for the intended submodules plus any intended workspace documentation.

## Constraints

Do not create a parent-repository commit unless the user explicitly asked for
one. Use Conventional Commits with `git commit -s`. Keep the commit subject
short and factual; a body, if any, states only what moved and why.

## Report

List each submodule in scope with old SHA, new SHA, the commit range summary,
and whether the move is intended. Flag every unintended pointer change
separately so it can be reverted rather than committed.
