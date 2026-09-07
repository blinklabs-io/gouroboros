# Claude Code instructions

This workspace is shared by Claude, Codex, CodeRabbit, and Cubic. Use the same
repository boundaries, review standards, and durable documentation regardless
of which agent or review bot started the work.

## Start here

Read [`AGENTS.md`](AGENTS.md) first. For Go repositories and cross-repository
reviews, read the [Go repository common-ground guide](docs/go-repository-guide.md).
Then read the target submodule's local `AGENTS.md`, `CLAUDE.md`,
`CONTRIBUTING.md`, README, and Makefile.

The shared toolkit lives in `plugins/blink-labs-agent-toolkit/` and is enabled
for this repository through `.claude/settings.json`. It provides skills, the
`/orient`, `/validate`, `/review`, `/dep-audit`, `/release-check`,
`/submodule-sync`, and `/review-prs` commands, review subagents, and three
workspace guards. The catalog is
[docs/skill-catalog.md](docs/skill-catalog.md).

Start a cross-repository task with `/orient`, or with the
[`blink-workspace-navigator`](skills/blink-workspace-navigator/SKILL.md) skill,
to establish which repository owns the change before editing anything.
For any `TosiDrop/*` task, load
[`tosidrop-repo-maintainer`](skills/tosidrop-repo-maintainer/SKILL.md) instead
of treating the repository as a Blink submodule. TosiDrop shares ownership but
has separate repository, review, and release rules.

The `skills/*/SKILL.md` files are tool-neutral and work for Claude and Codex
alike. Load the relevant `SKILL.md` directly; the adjacent `agents/openai.yaml`
file is optional Codex UI metadata and is not a Claude runtime dependency.

`skills/` is a symlink into the plugin, as are the two shared guides under
`docs/`. Edit the files under `plugins/blink-labs-agent-toolkit/`, and run
`make validate` after any toolkit change.

The parent repository is a workspace of independently versioned submodules.
Source changes belong in the relevant submodule; the parent normally records
the resulting pointer and workspace documentation. Do not modify nested
repositories or re-pin submodules incidentally.

## Planning and tracking

Plan files and planning notes are local, ephemeral working artifacts. Never
commit them. Use repository issues for durable scope, acceptance criteria, and
follow-up tracking.

## Blink Labs reviews and review bots

This section governs `blinklabs-io` repositories and clanker. For TosiDrop,
follow the target repository and `tosidrop-repo-maintainer`; do not infer these
Blink requirements from shared ownership.

CodeRabbit and Cubic findings are review inputs, not substitutes for inspecting
the current checkout. For each finding:

1. Locate the exact path and current line or symbol.
2. Check the target repository's local instructions and existing tests.
3. Reproduce or disprove the behavior with a focused test, static check, or
   direct code path analysis.
4. Classify it as a merge blocker, non-blocking recommendation, false positive,
   or already addressed finding.

Do not copy bot prose into durable documentation without verifying it. When
reviewing a change, report behavioral and security defects first, then API or
wire compatibility, missing contract-specific tests, architecture boundaries,
documentation/generated drift, and style issues.

Commit messages, PR descriptions, and review comments must be short, factual,
and scoped to the current change. If a commit body or longer description is
needed, use only concise facts directly supported by the changed code, tests,
or review. Do not include storytelling, chat transcripts, roadmaps, future
plans, or unrelated context. Put code-specific feedback in inline comments;
keep PR-level comments to concise summaries, checks, and dispositions. Useful
Cubic and CodeRabbit summaries may remain in PR descriptions when clearly
attributed and converted to plain Markdown; remove generated HTML, buttons,
hidden bot state, prompts, run IDs, stale commit metadata, and duplicate
wrappers.

The review sequence is bot review first, human review second. Run CodeRabbit
and Cubic when configured, address their actionable findings, and only then
request human review. If CodeRabbit is rate-limited, document it; a completed
Cubic review is sufficient for bot review. A human review is mandatory and may
be AI-assisted, but bot approval or silence never counts as human approval.

Do not review draft pull requests. When the user excludes Dependabot, omit
pull requests authored by `dependabot[bot]` before inspecting diffs or posting
reviews.

When asked to check for reviews or review requests, include pull requests
assigned to any team the authenticated user belongs to, not only requests made
directly to the user's account. If team memberships cannot be queried, report
the scan as incomplete instead of claiming that no requests exist.

UI changes require screenshots in the pull request. Include the affected
states at the relevant viewport or platform, with secrets and user data
redacted.

When asked to resolve an issue, carry it through implementation, PR update,
bot review responses, valid fixes, validation, and bot re-runs until no
actionable bot findings remain and the PR is ready for human review.

When a human reviewer requests changes, implement and validate the fixes,
summarize the changes on the pull request, and explicitly request another
review from that same person through GitHub. Do not assume that replying to
the review or pushing commits automatically completes the review loop.

An authorized human reviewer may dismiss another human review through GitHub
when appropriate. Record the rationale on the pull request, and do not treat
dismissal as eliminating the requirement for appropriate human review.

Only the author merges their own pull request. Whoever presses merge takes
responsibility for the code, so an approval is not ownership — hand an approved
PR back to its author rather than merging it for them. `dependabot[bot]` is the
sole exception, since it cannot merge its own.

A pull request may be squash-merged when GitHub shows human approval for the
current head SHA, required checks pass, and configured bots have no actionable
findings. Use one concise factual squash summary and preserve the DCO
`Signed-off-by:` line.

## Go and Cardano work

Use repository-native Makefile targets and inspect every nested `go.mod`.
Preserve generated-code provenance, shared `ouroboros-mock` fixtures, raw CBOR
bytes, Cardano era semantics, and conformance vectors. For Dingo, also follow
the [Dingo maintainer skill](skills/dingo-maintainer/SKILL.md) and its local
architecture/database documentation. For live incidents, long validation runs,
or review findings, also read the [Dingo agent workflow](skills/dingo-maintainer/references/dingo-agent-workflow.md):
start from current `origin/main`, use isolated worktrees and unique resources,
inspect complete background-task output, preserve live evidence, and test
negative/absence cases rather than accepting a plausible path.

Do not claim that a check ran when it required unavailable live GitHub,
registry, Cardano devnet, conformance, or Antithesis infrastructure. Record
the exact skipped check and reason in the handoff.

When Docker is available and useful for the requested check or reproduction,
use it. Prefer equivalent `blinklabs-io` images and record why any Docker
validation was skipped or an upstream image was required.

Use canonical upstream repositories and Go modules for source dependencies.
Blink Labs forks are emergency-only exceptions that require explicit approval,
an issue, and an exit plan; Apollo must use `Salvionied/apollo` under normal
circumstances.

## Commits

Use Conventional Commits and DCO sign-off (`git commit -s`). Keep workspace
documentation, skill, and submodule-pointer changes easy to distinguish. A
workspace guard denies a commit that is missing either, so fix the command
rather than working around it. Details are in the
[`commit-and-pr-hygiene`](skills/commit-and-pr-hygiene/SKILL.md) skill.

For changes made directly in `clanker`, commit and push validated work as part
of the same task unless the user says not to. Do not include unrelated existing
changes or submodule pointer moves outside the requested scope.

Before reporting work as complete, produce the evidence ledger and skipped-check
list described in
[`evidence-based-handoff`](skills/evidence-based-handoff/SKILL.md).

## Review findings

Check a finding against the branch's current head before acting on it: on an
active branch most open findings are already fixed and the thread is merely
unanswered. When one is real, fix the cause rather than the sentence — read the
consumer before changing a shape
([`cross-boundary-changes`](skills/cross-boundary-changes/SKILL.md)), decide the
contract once and hold it under push-back, and prove any new test fails without
the fix
([`regression-test-discipline`](skills/regression-test-discipline/SKILL.md)).
Before pushing, audit the class the finding belongs to rather than the single
location it named, including any member the fix creates by making a path
reachable
([`github-review-coordinator`](skills/github-review-coordinator/SKILL.md)); a
class that turns out to be complete is a result worth stating, not a null one.
Reply to every bot thread, including already-fixed and rejected ones, and expect
another bot pass after each push.

## Efficient token use

Cost is dominated by the number of API round trips, not by the size of what
each one returns. Every request re-sends the whole accumulated conversation,
so spend grows roughly quadratically with request count. Measured across a
25-agent pull-request review sweep: 3.3 MB of total tool output against 161M
tokens of context re-reads, for 25.3M input-token-equivalents overall — 69.4
requests and 845K equivalents per review.

A later 9-review sweep of the same repositories, dispatched with the batching
rules below stated in each prompt, ran at 18.7 requests and 324K equivalents
per review: 73% fewer round trips and 62% less spend, with no loss of rigor —
it still produced seven blockers, including two the review bots never saw
because they were quota-blocked. Per-request cost rose (12.2K to 17.3K
equivalents) because the requests are fatter; total spend is what matters.

Batch aggressively:

- Combine independent commands into one call. `go build ./... && go vet ./...
  && gofmt -l . && golangci-lint run ./...` is one round trip, not four.
- Read many files in one call:
  `for f in a.go b.go c.go; do echo "=== $f"; sed -n '1,250p' "$f"; done`.
- Fetch pull-request metadata, diff, review threads, check runs, and comments
  in a single `gh` call with `--jq` field selection.

Trim round trips, never rigor. Fail-before reverts, class audits, and
merged-tree checks cost little and are where findings come from. Batched
reviews averaged 31 requests and 0.55M equivalents against 75 requests and
1.07M unbatched — half the cost, and each still found a blocker both review
bots had missed.

Subagents start from a fresh context: the agent definition plus its dispatch
prompt. Clearing the parent conversation does not reduce subagent cost, and a
subagent never sees the parent's history unless it is a fork. Put everything
the agent needs into the dispatch prompt.

The parent thread is its own cost centre. One sweep's orchestrator reached a
302K-token context and 2.21M equivalents, more than any single review it
dispatched. Write durable cross-task facts to a scratch file and clear between
units of work rather than carrying the whole transcript forward.

Do not stagger a concurrent batch to warm the cache. Measured over nine
dispatches: the first agent of a simultaneous trio created the 19.5K-token
shared prefix and read none of it, and both siblings dispatched in the same
message read all 19.5K of it — identical to every later staggered agent.
Staggering bought nothing and cost wall-clock time.

Keep the cached prefix stable: switching model or effort mid-run invalidates
it, and so does editing an earlier turn.

Never wait on CI inside a review agent. The one agent that blocked ~16 minutes
on an in-progress job cost 640K equivalents against a 197K-324K sweep norm —
the single most expensive review, for a check the parent can poll once and
cheaply re-dispatch on. Read the check runs; if they are still pending, record
the disposition as withheld and return.

Front-load what earlier agents discovered. Facts the parent learns once and
injects into later prompts — the base branch's own `gofmt` noise, which bots
are quota-blocked on this head, the sibling-PR map — stop each agent from
rediscovering them. The three cold-start reviews averaged 23.3 requests; the
five later ones carrying those facts averaged 13.6, a 42% drop.

Measure round trips from the session transcript
(`~/.claude/projects/<project>/<session>/subagents/agent-*.jsonl`, deduped by
`requestId`), not from what an agent reports about itself. Self-reported counts
were wrong in both directions: one agent claimed 44 tool calls against 28 real
requests, another claimed 21 against 30.
