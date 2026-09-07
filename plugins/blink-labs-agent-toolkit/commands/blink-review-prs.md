---
description: "Sweep blinklabs-io for pull requests that are ready for review, dispatch a review shepherd for each three at a time, and aggregate the sweep's token usage into concrete efficiency findings"
argument-hint: "[how many PRs, or a repository to prefer]"
allowed-tools: ["Bash", "Glob", "Grep", "Read", "Task"]
---

# Ready-for-review sweep

Target: "$ARGUMENTS" (if empty, sweep the latest ready PRs, preferring `dingo`
and `gouroboros`).

Scour `blinklabs-io` for pull requests that are ready for review, review each
one in a `blink-review-shepherd` agent, three agents at a time, and finish by
aggregating the sweep's measured token usage into strategies that make the next
sweep cheaper.

Posting reviews to GitHub is an outward-facing action. Confirm the sweep size
and whether to post before dispatching anything, unless the invocation already
said so.

## Steps

1. **Discover.** Use the workspace scanner rather than hand-rolling a query — it
   reads review records and checks at the exact current head SHA and never
   writes:

   ```sh
   make scan-prs ARGS="--format json" > "$SCRATCH/scan.json"
   ```

   The script is not executable; invoke it through the Makefile target, or as
   `python3 scripts/scan-prs.py`. It already excludes drafts and
   `dependabot[bot]`; pass `--include-drafts` or `--include-dependabot` only
   when asked. `--owner` defaults to `blinklabs-io`, and
   `make scan-tosidrop-prs` covers `TosiDrop`.

   The result is an object, not a bare array: the pull requests are under
   `.pull_requests`, alongside `.user`, `.review_teams`, and
   `.team_lookup_error`. A **non-null** `.team_lookup_error` means team
   memberships could not be queried, so review requests made to a team are
   missing: report the scan as incomplete rather than as an empty backlog.

2. **Filter to genuinely ready.** From the scan, keep pull requests with no
   human review on the current head, then drop:
   - **drafts** (already excluded by the scanner);
   - **a pipeline still running** — any entry in `problem_checks` whose
     `status` is not `completed`. Reviewing these is what makes a sweep
     expensive: a shepherd that blocks on an in-progress job costs roughly
     double, and the disposition it reaches is provisional anyway. Re-queue
     them at the end of the sweep instead.

   ```sh
   jq -r '.pull_requests[] | select(.error==null)
      | select([.problem_checks[]? | select(.status != "completed")] | length == 0)
      | [.repository, (.number|tostring), .author, .updated_at, .title] | @tsv' \
      "$SCRATCH/scan.json"
   ```

   Count both sides before dispatching, so the deferred set is a reported
   number rather than a silent omission:

   ```sh
   jq '[.pull_requests[]|select(.error==null)]
      | {total: length,
         ready:   ([.[]|select([.problem_checks[]?|select(.status!="completed")]|length==0)]|length),
         running: ([.[]|select([.problem_checks[]?|select(.status!="completed")]|length>0)]|length)}' \
      "$SCRATCH/scan.json"
   ```

   A representative run: 114 open PRs, 96 ready, 18 deferred for a running
   pipeline. Entries with a non-null `.error` failed inspection and are neither
   ready nor deferred — report them separately.

   A PR whose `problem_checks` are all `completed` but not passing is still
   worth reviewing — a red pipeline blocks approval, not review. Keep it and
   let the shepherd attribute the failure.

3. **Rank.** Prefer the most recent `dingo` and `gouroboros` pull requests,
   then the rest by `updated_at`. Report the count and the chosen slice before
   dispatching; a full `REVIEW_REQUIRED` backlog can exceed thirty PRs.

4. **Dispatch three at a time.** One `blink-review-shepherd` per pull request,
   in Mode B. Keep exactly three in flight: when one finishes, launch the next
   rather than waiting for the whole wave, which both keeps under the rate
   limit and staggers naturally.

   Do not stagger dispatches to warm the prompt cache — siblings launched in
   the same message already read the shared prefix the first one wrote.

   The shepherd may register as `blink-review-shepherd` or as
   `blink-labs-agent-toolkit:blink-review-shepherd`, and plugin agents can
   register a little after session start. If the type will not resolve, inline
   the agent's Mode B definition into a `general-purpose` dispatch instead of
   restoring `.claude/agents/` symlinks — those duplicate the plugin's own
   registration.

5. **Write each dispatch prompt to stand alone.** A subagent sees only its
   definition and this prompt. Include the repository, PR number, author, size,
   the checkout path and an instruction to use its own worktree and an isolated
   `GOLANGCI_LINT_CACHE`, a domain note on what the change can break, and the
   authorization to post.

   Front-load everything the sweep has already learned, so each agent does not
   rediscover it: the base branch's own `gofmt`/lint noise, which bots are
   quota-blocked on this head, the sibling-PR map for the same class, and any
   finding a previous agent proved or disproved. Measured across one sweep,
   this cut later reviews from 23.3 requests to 13.6.

   Require in every prompt: check findings against the current head; prove a new
   test fails without the fix by reverting the fix in place; audit the whole
   class, not the named line; verify the PR body's issue references; never claim
   an unrun check.

6. **Track usage as it runs.** Per-agent usage lives in the session transcripts,
   not in what an agent reports about itself — self-reported tool counts were
   wrong in both directions:

   ```sh
   ~/.claude/projects/<project>/<session>/subagents/agent-*.jsonl
   ```

   Dedupe rows by `requestId` (streaming writes several per request), then
   weight `input + 1.25*cache_creation_5m + 2.0*cache_creation_1h +
   0.1*cache_read` for input-token-equivalents. Each agent's
   `.meta.json` carries its description. Log each verdict to a scratch file as
   it lands and keep the parent thread lean — the orchestrator is routinely the
   most expensive single line item in the sweep.

7. **Aggregate and report strategies.** When every review is settled, total the
   sweep, compare requests-per-review and equivalents-per-review against prior
   sweeps in the same project, and name the specific cost drivers with numbers.
   Record durable findings in the `Efficient token use` section of the
   workspace `CLAUDE.md`; do not leave them only in the transcript.

## Report

One row per pull request: URL, disposition, the single most important finding,
and whether the pipeline was green, red, or still running. Say which bots
actually reviewed each head — a green CodeRabbit or Cubic check whose body
reads "rate limited", "review skipped", or "not needed for merge commit" is not
a review, and that is common enough to expect on most heads.

Then the usage table: per agent requests, cache creation, cache read, output,
and equivalents, plus the parent's own row and the sweep total. Close with the
measured efficiency findings, each tied to the number that supports it, and the
list of PRs deferred because their pipeline was still running.
