---
description: "Sweep blinklabs-io for pull requests that are ready for review, dispatch a review shepherd for each three at a time, then aggregate the sweep's token usage and update the workspace CLAUDE.md efficiency findings with what it measured"
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
   - **anything the authenticated user authored.** A self-approval does not
     satisfy the human-review requirement, so reviewing your own pull request
     cannot unblock it — it needs a teammate. GitHub refuses the approval
     outright, so a shepherd dispatched at one will do the whole review and then
     fail to post it. Compare against `.user` from the scan, not a hardcoded
     login.
   - **anything already assigned to someone else.** An assignee is how this
     organization signals that a review is taken; duplicating it wastes both the
     sweep's tokens and the other reviewer's time. An empty assignee list is
     unclaimed and fair game.
   - **a pipeline still running** — any entry in `problem_checks` whose
     `status` is not `completed`. Reviewing these is what makes a sweep
     expensive: a shepherd that blocks on an in-progress job costs roughly
     double, and the disposition it reaches is provisional anyway. Re-queue
     them at the end of the sweep instead.

   ```sh
   jq -r --arg me "$(jq -r .user "$SCRATCH/scan.json")" '
      .pull_requests[] | select(.error==null)
      | select([.problem_checks[]? | select(.status != "completed")] | length == 0)
      | select(.author != $me)
      | select([.assignees[]? | select(. != $me)] | length == 0)
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
   ready nor deferred — report them separately. Report the self-authored and
   already-assigned counts too, so the skipped set is a stated number rather
   than a silent omission.

   A PR whose `problem_checks` are all `completed` but not passing is still
   worth reviewing — a red pipeline blocks approval, not review. Keep it and
   let the shepherd attribute the failure.

3. **Rank.** Prefer the most recent `dingo` and `gouroboros` pull requests,
   then the rest by `updated_at`. Report the count and the chosen slice before
   dispatching; a full `REVIEW_REQUIRED` backlog can exceed thirty PRs.

   **Then claim the slice by self-assigning it, before the first dispatch:**

   ```sh
   gh pr edit "$number" --repo "$repository" --add-assignee @me
   ```

   Assign every pull request the sweep will review, not just the one in flight.
   The point is to tell the rest of the team that these are taken while the
   sweep runs, so it has to happen up front — assigning as each shepherd starts
   leaves the tail of the slice looking unclaimed for the length of the sweep.
   Assignment is an outward-facing action and is covered by the same
   confirmation as posting.

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

   State that authorization explicitly. `blink-labs-agent-toolkit:review`
   forbids posting to GitHub unless told to, so a shepherd whose prompt omits
   it will correctly review and then publish nothing. Carry through whatever
   the user answered in the confirmation above — if they chose a local report
   only, say that instead, and no review is posted.

   Front-load everything the sweep has already learned, so each agent does not
   rediscover it: the base branch's own `gofmt`/lint noise, which bots are
   quota-blocked on this head, the sibling-PR map for the same class, and any
   finding a previous agent proved or disproved. Measured across one sweep,
   this cut later reviews from 23.3 requests to 13.6.

   Tell each shepherd to run the toolkit's own review workflow rather than
   improvising one: invoke the `blink-labs-agent-toolkit:review` command and
   follow its steps. That keeps every review in the sweep on the same rules —
   owner and boundary, bot reconciliation, cause over symptom, negative-case
   coverage, the repository's change bar, and finding order — instead of
   depending on how well each dispatch prompt was written.

   The shepherd has no `Task` tool, so it does the domain analysis itself
   rather than dispatching the auditor subagents step 2 of
   `blink-labs-agent-toolkit:review` lists.
   That is deliberate: nested fan-out is the most expensive thing a review
   can do, and it is what a sweep must not pay per PR.

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

7. **Aggregate, then update `CLAUDE.md`.** When every review is settled, total
   the sweep and compare it against the figures already recorded in the
   `Efficient token use` section of the workspace `CLAUDE.md`: requests per
   review, input-token-equivalents per review, and the cost of the parent
   thread against the agents it dispatched.

   Then edit that section — this step is part of the command, not an optional
   follow-up. A finding earns its place only if a number in this sweep supports
   it, and it must say which:

   - **Confirm** an existing claim by adding the new measurement beside it.
   - **Correct** a claim this sweep contradicts, and say what was measured
     instead. One sweep overturned the advice to stagger concurrent dispatches
     this way.
   - **Add** a driver the section does not name yet, with the number that
     exposed it.
   - **Drop** nothing on a single sweep's evidence; note the disagreement
     instead and let the next sweep settle it.

   Keep the section's terse style, run `make validate`, and commit the edit
   with the sweep's own numbers in the commit body. Do not record a finding
   from an agent's self-report — only from the transcript totals.

## Report

One row per pull request: URL, disposition, the single most important finding,
and whether the pipeline was green, red, or still running. Say which bots
actually reviewed each head — a green CodeRabbit or Cubic check whose body
reads "rate limited", "review skipped", or "not needed for merge commit" is not
a review, and that is common enough to expect on most heads.

Then the usage table: per agent requests, cache creation, cache read, output,
and equivalents, plus the parent's own row and the sweep total. Close with the
measured efficiency findings, each tied to the number that supports it, the
`CLAUDE.md` edit those findings produced, and the list of PRs deferred because
their pipeline was still running.
