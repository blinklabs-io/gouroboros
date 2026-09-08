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
`/submodule-sync`, `/review-prs`, and `/issue-to-pr` commands, the
`issue-to-pr` workflow, review subagents, and three
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

`blink-tdd-developer` stops at a signed local commit and has no dispatch tool,
so it cannot start `blink-review-shepherd` itself, and no `SubagentStop` hook
output reaches the orchestrator's context. **Dispatching the shepherd after a
developer commits is the orchestrating session's job, and it has been dropped
before.** For more than one issue, run the `issue-to-pr` workflow instead of
dispatching by hand: the handoff is a pipeline stage there, so it cannot be
forgotten, and the developer's worktree, branch, base and commit SHA reach the
reviewer as typed data rather than as re-narrated prose. A developer that
returns no commit — the defect was already fixed upstream — has nothing to hand
over, and that is a success, not a skipped review.

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

Costs here are quoted in **input-token-equivalents**: `input + 1.25 *
cache_creation_5m + 2.0 * cache_creation_1h + 0.1 * cache_read`, summed over
requests deduped by `requestId`. The weights are the API's cache pricing
multipliers, so the figure is a single price-weighted number comparable across
agents and sweeps.

**The one lever is round trips.** Every request re-sends the whole accumulated
conversation, so spend grows roughly quadratically with request count. Cache
reads are 64% of a sweep's cost (33.4M of 39.9M input tokens in a nine-review
dingo sweep; cache creation 24%, output 5%), and the only way to shrink them is
to make fewer requests. Round trips predict cost at r=0.955 over nine reviews;
diff size only at r=0.759. Control round trips; never trade away rigor to do it.

### 1. Batch every call

1. Chain independent commands: `go build ./... && go vet ./... && gofmt -l . &&
   golangci-lint run ./...` is one trip, not four.
2. Read files in a loop: `for f in a.go b.go c.go; do echo "=== $f"; sed -n
   '1,250p' "$f"; done`.
3. Pull pull-request metadata, diff, review threads, check runs and comments in
   a single `gh` call with `--jq` field selection.
4. Keep the fail-before reverts, class audits and merged-tree checks. They cost
   little and are where findings come from.
5. Measured: 18.7 requests and 324K equivalents per review batched, against 69.4
   and 845K unbatched — 73% fewer trips, 62% less spend, with seven blockers
   still found including two the review bots never saw.

### 2. Subagent discipline

1. Put everything the agent needs into the dispatch prompt. A subagent starts
   from a fresh context — the agent definition plus its prompt — and never sees
   the parent's history unless it is a fork. Clearing the parent conversation
   does not reduce subagent cost.
2. **Never wait on CI inside a review agent.** Subagents run on the 5-minute
   prompt-cache TTL, not the parent's 1-hour one: measured across six shepherds,
   608K `ephemeral_5m_input_tokens` and zero at the 1-hour TTL, and a
   nine-shepherd sweep split identically. Any stall over five minutes re-pays
   the entire prefix. One CI-blocked agent cost 640K against a 197K-324K norm.
   Read the check runs; if they are pending, record the disposition as withheld
   and return, and let the parent poll and re-dispatch.
3. Dispatch a concurrent batch in one message. Staggering to warm the cache buys
   nothing — measured over nine dispatches, the first agent of a simultaneous
   trio created the 19.5K shared prefix and both siblings read all of it,
   identical to every later staggered agent — and it costs wall-clock time.
4. Keep the cached prefix stable. Switching model or effort mid-run invalidates
   it, and so does editing an earlier turn.
5. Scope `golangci-lint` to the packages the change touches
   (`golangci-lint run ./ledger/...`). It takes a global lock that an isolated
   `GOLANGCI_LINT_CACHE` does not prevent — that isolation solves stale
   cross-worktree findings, a different problem. Treat exit 3, "parallel
   golangci-lint is running", as contention to retry, never as a lint failure.

### 3. Orchestrator discipline

1. Log each verdict to a scratch file as it lands and keep findings out of the
   parent thread. A disciplined parent held 649K equivalents over 42 requests
   against the 2.21M an undisciplined one spent on a comparable run.
2. Write durable cross-task facts to that scratch file and clear between units
   of work rather than carrying the whole transcript forward.
3. Budget the parent at about a sixth of a nine-review sweep: 16.5% of 3.93M and
   16.6% of 5.19M across two independent sweeps. A materially higher share is a
   signal that findings are being carried in the parent thread.
4. Orchestrator cost is roughly fixed (536K-861K), so its share scales inversely
   with sweep size — 25.5% of a six-review sweep at the same absolute spend.
   Amortize it over more reviews, and do not read a high share on a small sweep
   as indiscipline.

### 4. Get these queries right the first time

1. Filter the backlog on `current_reviews`, excluding bots **and the pull
   request's own author**. An author answering a reviewer posts a `COMMENTED`
   review record that a bots-only filter reads as coverage: measured on a 147-PR
   blinklabs-io backlog, 78 ready by the bots-only predicate against 96 with the
   author also excluded — 18 PRs, 19% of the real backlog, invisible, and biased
   toward the ones most worth reviewing. Do not use `human_reviews`: it requires
   a non-empty body by design, so a bare `gh pr review --approve` is absent from
   it, and one sweep re-reviewed two already-approved PRs for 417K equivalents.

   ```sh
   . as $pr | select([$pr.current_reviews | to_entries[]
     | select(.key | test("\\[bot\\]") | not)
     | select(.key != $pr.author)] | length == 0)
   ```

2. Test for the presence of any human key, not for `state == "APPROVED"`.
   `current_reviews` keeps only the latest review per reviewer, so an `APPROVED`
   followed by a `COMMENTED` reads as `COMMENTED`. It also carries no
   `commit_id` and is scoped to the current head, so it hides earlier reviews:
   one PR showed an empty `current_reviews` while carrying two prior
   `CHANGES_REQUESTED`. Say "no review at this head", and read the reviews API
   when history matters or `reviewDecision` when the prior decision does — one
   PR read `COMMENTED`, `CHANGES_REQUESTED then COMMENTED`, and
   `CHANGES_REQUESTED` by those three sources respectively.
3. Verify bot coverage by the `commit_id` stamped on each review record, never
   by check colour and not merely by `output.summary`. Green routinely means
   "this commit only merges another branch into the PR branch", "could not start
   the incremental review", "Review limit reached", or "auto incremental reviews
   are disabled". Coverage is per-head and erratic — measured across three
   sweeps, Cubic genuinely reviewed 2 of 9, 4 of 9 and 2 of 6 heads; CodeRabbit
   1, 1 and 1 — so scope the claim to the head you measured. An empty review
   body is not proof that no review happened: CodeRabbit's empty-bodied records
   have been containers for substantive inline comments, so reconcile
   `pulls/<n>/comments` against review records before calling a head unreviewed.
   A `cubic` check reporting "0 issues found" with no review record is a check,
   not a review. Most blockers land on heads no bot reviewed.
4. Attribute a red lint check by running the linter at head and at base rather
   than reading CI logs — one `golangci-lint run ./ledger/` at each end settled
   attribution in one extra command. A known cause: `.golangci.yml` sets
   `run.tests: false`, so rewriting the last production call site of a helper
   leaves it referenced only from `_test.go` and trips `unused`. Also check
   whether a red run was superseded by a later passing run on the same SHA.
5. Re-read the reviews API at dispatch time rather than trusting the scan. The
   scan is a snapshot and a long sweep outruns it: one sweep took 7h41m from
   scan to last verdict, and a teammate reviewed two of its pull requests at
   7h11m, after dispatch. Both agents were told as established fact that no
   human had reviewed, and both found it false.

### 5. Budgeting and measuring

1. Budget by expected verification actions, not by diff lines. Below roughly
   2000 lines diff size stops predicting cost: equivalents-per-line swung 15x in
   two separate sweeps, and a 204-line review that ran `make docs-parity`,
   `make import-boundaries`, race tests across 19 packages, a merged-tree check
   and two fail-before reverts cost 293K against a 2046-line review's 285K.
2. Assume a floor near 300K equivalents per review — the three smallest reviews
   of a nine-review sweep (136-210 lines) cost 284K, 366K and 372K. Above ~3000
   lines diff size does dominate: two changes of 3741 and 1484 lines cost 45.5
   requests and 707K each, 43% of subagent spend for 22% of the pull requests.
3. Expect a deep sweep to cost more and to be worth it. One at 481K per review
   and 29.6 requests ran in-tree probe binaries, in-place fail-before reverts,
   head-versus-base lint attribution, merged-tree builds and `-race -count=4`,
   and returned seven blockers in nine reviews. A cheaper sweep is available; it
   is a different product.
4. Measure round trips from the session transcript
   (`~/.claude/projects/<project>/<session>/subagents/agent-*.jsonl`, deduped by
   `requestId`), never from what an agent reports about itself. Self-reported
   counts were wrong in both directions: 44 claimed against 28 real, 21 against
   30.
5. Front-load earlier agents' findings for finding quality, not to cut requests.
   Two sweeps point opposite ways and both are confounded by diff size, so
   settle it with a front-loaded wave of comparable size rather than a third
   confounded measurement. Front-loaded facts decay: mark each with its
   provenance ("no human review **at scan time**", "`v0.204.0` observed
   sweep-wide, confirm per worktree with `go list -m`") rather than as an
   invariant. Three such facts were stale by the time an agent checked, and the
   agents caught all three.
