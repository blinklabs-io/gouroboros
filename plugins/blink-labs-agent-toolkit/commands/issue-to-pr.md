---
description: "Take assigned blinklabs-io issues from triage through implementation to reviewed pull requests, running the developer-to-reviewer handoff as a deterministic workflow so it cannot be dropped"
argument-hint: "[issue numbers, a project view URL, or empty for your assigned issues] [--publish]"
allowed-tools: ["Bash", "Glob", "Grep", "Read", "Workflow"]
---

# Issue to pull request

Target: "$ARGUMENTS" (if empty, take every issue assigned to the authenticated
user that is not Done or Archived).

**This command instructs you to call the `Workflow` tool.** That is the opt-in
into multi-agent orchestration; you do not need any further confirmation to
launch it, only the confirmations named below.

The workflow is `issue-to-pr`, and it exists because the handoff between
`blink-tdd-developer` and `blink-review-shepherd` used to be a reminder. A
subagent has no dispatch tool, so the developer cannot start the reviewer
itself, and a `SubagentStop` hook cannot reach the parent's context to ask it
to. Encoding the handoff as a pipeline stage is what makes it undroppable.

## 1. Triage before you launch anything

Do this inline. A workflow fans out over a work-list; you have to find the list
first.

Query the project or the issues named in `$ARGUMENTS`, then **drop every issue
that already has an open pull request**. Check the linked pull requests, not
just the issue body: an issue is covered when a PR closes it, and it may also be
covered by a PR filed against a *different* issue for the same root cause. Say
which issues you dropped and why.

Batch the triage: one paginated GraphQL call for the project items, one batched
call with per-issue aliases for the linked pull requests.

## 2. Confirm scope

Report the issue list you are about to work and the publication setting, then
launch. Confirm first only when the invocation did not already settle it:

- **Publication is outward-facing.** Pass `publish: true` only when the
  invocation said so (`--publish`) or the user has authorized publishing in this
  session. Without it, each shepherd reviews and fixes locally but stops before
  `git push` and `gh pr create`, and returns what it would have published.
- More than a handful of issues, or an issue whose acceptance criteria need live
  infrastructure, is worth naming before you spend on it.

Mark each issue you are about to work as **In Progress** on its project board
before launching, so a parallel session does not pick it up.

## 3. Front-load what you already know

Each subagent starts from a fresh context: the agent definition plus its
dispatch prompt. Facts you learned during triage are free to inject and
expensive for the agent to rediscover. Measured across an earlier sweep, agents
carrying front-loaded facts averaged 13.6 requests against 23.3 for cold-start
ones — a 42% drop.

Put into each issue's entry:

- `body` — the issue text verbatim, so the agent does not fetch it
- `base` — the repository's current `origin/main` SHA, fetched once by you
- `notes` — sibling pull requests touching the same code, prior art that already
  shipped, which bots are quota-blocked on this head, known base-branch lint noise
- `priorBranch` — any unpushed local branch holding earlier work on this issue
- `scope` — for an epic, the one case this run may land, and what is out of bounds

## 4. Launch

```
Workflow({
  name: "issue-to-pr",
  args: {
    publish: false,
    sessionUrl: "<this session's URL, for the commit and PR trailers>",
    issues: [
      {
        repo: "blinklabs-io/dingo",
        num: 1234,
        title: "...",
        url: "https://github.com/blinklabs-io/dingo/issues/1234",
        body: "<the issue text, verbatim>",
        gitDir: "/abs/path/to/repos/dingo",
        base: "<origin/main SHA>",
        worktreeRoot: "<scratchpad dir the agents may create worktrees under>",
        notes: "<sibling PRs, prior art, environment facts>",
        priorBranch: "<branch and path, or omit>",
        scope: "<for an epic: the one case this run may land, or omit>"
      }
    ]
  }
})
```

If the name does not resolve, run the shipped script directly with
`Workflow({scriptPath: "<plugin root>/workflows/issue-to-pr.js", args: {...}})`.

The workflow runs in the background and returns a task ID; `/workflows` shows
live progress. Do not poll it.

## 5. Report what came back

The workflow returns `published`, `reviewedNotPublished`, `noChange`,
`needsAttention` and `skippedChecks`. Relay all five — the agents' own reports
are not shown to the user.

- `noChange` is a **success** outcome, not a failure: an issue whose defect a
  merged change already fixed has nothing to review. Verify the evidence it
  gives, and offer to close that issue as a duplicate or as already fixed rather
  than closing it unasked. Set its board status accordingly.
- `needsAttention` means a branch exists that nothing reviewed. Say so plainly
  and name the branch; do not let it read as finished work.
- Carry `skippedChecks` into your summary verbatim. Unavailable live
  infrastructure is a real gap in the evidence, not a formality.

Then reconcile the board: `published` issues stay In Progress until merged,
`noChange` issues move to Done once closed.
