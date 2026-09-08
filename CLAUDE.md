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

Filter the backlog on `.current_reviews`, not `.human_reviews`. `scan-prs.py`
requires a non-empty review body for `human_reviews`, deliberately, so that
blocking prose in a `COMMENTED` review surfaces. The side effect is that a
*bodiless* `APPROVED` review — what a bare `gh pr review --approve` produces —
is absent from that field entirely. A third sweep used it as the
"has a human reviewed this head?" predicate and re-reviewed two already-approved
pull requests: 417K equivalents, 10.6% of the sweep, spent on duplicates. The
backlog was 75 by that predicate and 64 by the correct one, so 15% of the
apparent queue was already reviewed. Use:

```sh
select([.current_reviews|to_entries[]|select(.key|test("\\[bot\\]")|not)]|length==0)
```

`current_reviews` keeps only the latest review per reviewer, so an `APPROVED`
followed by a `COMMENTED` reads as `COMMENTED` — test for the presence of any
human key, not for `state == "APPROVED"` — and it carries no `commit_id`, so
cross-check the reviews API when "on this head" actually matters. Measured on
one PR: `current_reviews` read `COMMENTED`, the reviews API read
`CHANGES_REQUESTED then COMMENTED`, and `reviewDecision` read
`CHANGES_REQUESTED`. Use `reviewDecision` when the prior decision matters.

**Exclude the pull request's own author from that predicate too.** An author
answering a reviewer posts a `COMMENTED` review record, which enters
`current_reviews` indistinguishable from someone else's review, so a bots-only
filter reads the author's own reply as coverage and hides the PR. Measured on a
147-PR blinklabs-io backlog: 78 ready by the bots-only predicate against 96 with
the author also excluded — **18 PRs, 19% of the real backlog, invisible**, and
biased toward the ones most worth reviewing, since an author replies precisely
when they have just pushed a fix. `scan-prs.py` already excludes the author from
`human_reviews`; `current_reviews` does not. Use:

```sh
. as $pr | select([$pr.current_reviews | to_entries[]
  | select(.key | test("\\[bot\\]") | not)
  | select(.key != $pr.author)] | length == 0)
```

Diff size, not repository or subject, is the dominant per-review cost driver.
In that same sweep the two largest changes (3741 and 1484 lines) cost 45.5
requests and 707K equivalents each — 43% of all subagent spend for 22% of the
pull requests — while the other seven averaged 23.3 requests and 267K. Budget a
sweep by summed diff size, and expect a 3000-line review to cost roughly three
small ones.

Verify bot coverage by the `commit_id` stamped on each review record, never by
check colour and not merely by `output.summary`. Measured over nine heads:
Cubic genuinely reviewed 2, CodeRabbit 1. The other green checks read "This
commit only merges another branch into the PR branch, so cubic completed this
check without running an AI review", conclusion `neutral` with "could not start
the incremental review", state `skipping`, "Review limit reached", or "auto
incremental reviews are disabled". Coverage is per-head and erratic: CodeRabbit
reviewed one head while rate-limited on another in the same sweep, so scope the
claim to the head you measured. A second nine-head dingo sweep measured Cubic 4
and CodeRabbit 1, with 4 heads carrying no bot review at all.

**An empty review body is not proof that no review happened**, correcting the
earlier reading. On one head CodeRabbit's two empty-bodied review records were
containers for a substantive Major inline comment, while the cubic check on the
same head read `skipping`. Fetch `pulls/<n>/comments` and reconcile inline
comments against review records before calling a head unreviewed; a Cubic "No
issues found" record that carries zero inline comments is the genuinely empty
case. Three of that sweep's seven blockers were on heads no bot had reviewed,
and two more were on heads where a bot had reported "No issues found".

Attribute a red check by running the linter at both head and base rather than
reading CI logs. One `golangci-lint run ./ledger/` at each end gave 1 issue at
head and 0 at base, which settled attribution in one extra command. Repeated on
a `connmanager` change: 1 at head, 0 at base `86c0be79`. The cause there is worth
knowing — `.golangci.yml` sets `run.tests: false`, so rewriting the last
production call site of a helper leaves it referenced only from `_test.go` and
trips `unused`, failing the required `lint` check. Also check whether a red run
was superseded: one PR's `cancelled` Windows job at 12:04 was followed by a
passing run on the same SHA at 13:55.

Orchestrator discipline is worth roughly 1.5M equivalents. Logging each verdict
to a scratch file and keeping findings out of the parent thread held the
orchestrator to 649K equivalents over 42 requests — 16.5% of a 3.93M sweep,
against the 2.21M an earlier orchestrator spent on a comparable run.

Below roughly 2000 diff lines, diff size stops predicting review cost. A
six-review dingo sweep spanning 63 to 2046 lines cost 261K equivalents per
review at 21.2 requests, and the ranking barely tracked size: 204 lines cost
293K while 2046 lines cost 285K, and equivalents-per-line swung 15x, from 139
to 2043. What actually drove cost was the number of verification actions — the
204-line review ran `make docs-parity`, `make import-boundaries`, race tests
across 19 packages, a merged-tree check and two separate fail-before reverts.
The earlier "diff size dominates" finding was measured against 3741- and
1484-line changes; it holds at that range, not below it. Budget a small-PR
sweep by verification depth and a fixed floor near 130K, not by summed lines.

**Round trips predict cost far better than diff size does.** Over nine dingo
reviews spanning 136 to 4428 lines, requests correlate with equivalents at
r=0.955; diff size only at r=0.759. Equivalents-per-line swung 15x again, 176 at
4428 lines to 2671 at 137. The floor is also higher than 130K: the three
smallest reviews (136-210 lines) still cost 284K, 366K and 372K. Budget by
expected verification actions and count round trips as the thing to control;
diff size is a weak proxy for both.

That sweep cost 481K equivalents per review at 29.6 requests, well above the
261K-324K band recorded above, and the reason is verification depth rather than
size. These reviews ran in-tree probe binaries to reproduce defects, fail-before
reverts performed in place, head-versus-base lint attribution, merged-tree
builds, and `-race -count=4` runs. It bought seven blockers in nine reviews,
including a chain-halt, a silent-forging-stop, a vote-stealing path and a
red-lint attribution. A cheaper sweep is available; it is a different product.

Front-loading earlier agents' discoveries did not reduce request count in that
sweep, and the effect may not generalize. The three cold-start agents averaged
17.3 requests; the three carrying a baseline, a bot-coverage map, a sibling-PR
map and two prior findings averaged 25.0, at 12% higher per-request cost
(12.9K vs 11.5K equivalents) because the prompts were fatter. The later agents
also reviewed the harder changes and found the two worst blockers, so this is
not evidence that front-loading is wasteful — only that it does not reliably
buy back round trips, and should be justified by finding quality rather than by
a predicted request-count drop.

A later nine-review sweep disagrees, and the disagreement is unresolved because
both measurements are confounded. Its three cold-start agents averaged 39.3
requests and 666K equivalents; the six carrying accumulated findings averaged
24.7 and 388K — 37% fewer round trips and 42% less spend. But the cold-start
wave was deliberately the three largest changes (2413 lines average against
675), so size and front-loading move together and this sweep cannot separate
them any more than the previous one could. Two sweeps now point opposite ways.
Settle it by front-loading a wave of *comparable* size to the cold-start wave
rather than by adding a third confounded measurement.

A second nine-review dingo sweep put the orchestrator at 861K equivalents over
53 requests — 16.6% of a 5.19M sweep, against 16.5% of a 3.93M one. Two
independent nine-review sweeps landing within 0.1 percentage points makes the
parent share predictable enough to budget: assume the orchestrator costs about a
sixth of a nine-review sweep, and treat a materially higher share as a signal
that findings are being carried in the parent thread.

Orchestrator cost is roughly fixed, so its share scales inversely with sweep
size. A disciplined parent that logged every verdict to a scratch file and kept
findings out of the thread spent 536K equivalents over 36 requests — close to
the 649K/42 of a nine-review sweep, but 25.5% of this six-review sweep's 2.11M
against that one's 16.5%. Amortize the orchestrator over more reviews, and do
not read a high parent share on a small sweep as indiscipline.

Cache *reads* are where a sweep's money goes, and the share is growing. In a
nine-review dingo sweep, 33.4M of the 39.9M total input tokens were cache reads:
weighted, 3.34M of 5.19M equivalents, **64%** — against the 51% measured earlier.
Cache creation was 1.24M (24%) and output only 244K (5%). Trimming what each
agent re-reads therefore beats trimming what it writes, and the lever on cache
reads is round trips, since every one re-reads the whole accumulated prefix.

Subagents run on the 5-minute prompt-cache TTL, not the parent's 1-hour one.
Measured across six shepherds: 608K tokens of `ephemeral_5m_input_tokens` and
**zero** `ephemeral_1h_input_tokens`, while the parent alone created 110K at the
1-hour TTL. A nine-shepherd sweep split identically with no exceptions: 1.24M at
the 5-minute TTL across the nine agents and zero at the 1-hour, against the
parent's 147K at the 1-hour TTL and zero at the 5-minute. An agent that stalls more than five minutes — waiting on CI, a long
sync, a slow test binary — re-pays its entire prefix. This is the mechanism
behind "never wait on CI inside a review agent", and it also means a subagent's
cache read, not its cache creation, is where the spend accumulates: cache reads
were 8.08M tokens, 51% of subagent equivalents.

`golangci-lint` takes a global lock, and an isolated `GOLANGCI_LINT_CACHE` does
not prevent contention. A concurrent agent hit exit 3, "parallel golangci-lint
is running", despite a correctly isolated cache; the isolated cache prevents
stale cross-worktree findings, which is a different problem. With three
shepherds in flight, scope each run to the packages the change touches
(`golangci-lint run ./ledger/...`), and treat exit 3 as contention to retry,
never as a lint failure to report.

**The scan is a snapshot, and a long sweep outruns it.** A nine-review sweep took
7h41m from scan to last verdict, and a teammate reviewed two of its pull requests
at 7h11m — after dispatch, before the shepherd posted. Both agents were told "no
human has reviewed this PR" as established fact and both found it false; one
produced a redundant second approval. Re-read the reviews API at dispatch time
rather than trusting the scan, and write front-loaded review state as "none at
scan time" rather than as a fact. Do not misattribute this to the
`human_reviews` predicate: one of those reviews was a bodiless `APPROVED`, which
that predicate does drop, but the timestamps show it did not exist when the scan
ran.

Front-loaded facts decay in general, so mark them as observations rather than
invariants. Of this sweep's front-loaded claims, three were wrong by the time an
agent checked: the empty-body bot reading above, the stale review state, and a
pinned dependency version asserted sweep-wide when it was per-branch
(`v0.204.0` front-loaded, `v0.202.10` actually pinned on one branch — resolve
with `go list -m` per worktree). Agents caught all three, which is the argument
for telling them what a fact's provenance is instead of only its value.

`scan-prs.py`'s `current_reviews` is scoped to the current head, so "no human
review" there means "none on this head" and hides earlier ones. One sweep PR
showed an empty `current_reviews` while carrying two prior human
`CHANGES_REQUESTED` at older commits, both already addressed. Say "no review at
this head" in a dispatch prompt, and have the agent check the reviews API when
the review history matters. Posting also perturbs the field: a decision review
followed by bodiless `COMMENTED` records carrying inline comments leaves
`current_reviews` reading `COMMENTED`, which is why the backlog predicate must
test for the presence of any human key rather than for `state == "APPROVED"`.

Bot coverage stayed erratic and mostly absent. Across six dingo heads, verified
by the `commit_id` on each review record: CodeRabbit genuinely reviewed one,
Cubic two. The rest were green checks that were not reviews — "cubic could not
start the incremental review", "This commit only merges another branch into the
PR branch", "Review rate limit", a review record stamped at an older commit, and
CodeRabbit issue comments reading "No actionable comments were generated" with
no review record at all. A `cubic` check reporting "0 issues found across 1
file" with no corresponding review record is a check, not a review. Where both
bots did review, both posted the same finding and both were false positives.
Four of the six blockers this sweep found were on heads no bot had reviewed.
