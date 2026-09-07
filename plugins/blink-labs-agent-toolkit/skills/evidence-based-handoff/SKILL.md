---
name: evidence-based-handoff
description: Report Blink Labs work to the requester so the result can be trusted without repeating it — an evidence ledger of commands and exit codes, an explicit skipped-check list with reasons, findings ordered by severity, and no claim that unavailable live infrastructure was exercised. Use before finishing a task or handing work back; do not use the ledger as a pull-request template.
---

# Evidence-Based Handoff

Use this skill when about to say work is done, fixed, passing, or reviewed. The
standard is simple: every claim traces to output you actually read.

The evidence ledger is private task-handoff material. Never paste it, local
paths, machine or account identifiers, private logs, tool transcripts, or
environment values into GitHub. Public records get sanitized technical facts
and repository-relative paths only.

## The rule

State only what you ran. If a check was not run, it is in the skipped list with
a reason. "Should pass", "presumably fine", and "tests look correct" are not
results. A change with no executed checks is unvalidated — say that plainly
rather than describing it as complete.

Live GitHub, container registries, the Cardano devnet, conformance suites, and
Antithesis are frequently unavailable. Never describe any of them as having run
when they did not. Naming the exact skipped check and the blocking reason is a
complete, acceptable answer; implying coverage is not.

### Try a required check before declaring it skipped

The skipped list is for checks you established you cannot run — not for checks
you decided not to attempt. When a repository's guidance says a change of this
class requires a suite, attempt the suite. Only after it actually fails to start
do you get to record it, and then the reason must name the missing thing
("no Docker daemon", "registry unreachable"), not the category it belongs to.

"Owed", "flagged as still needed", or "left for CI" is not a disposition for a
required check. It reads as diligence while shifting the work to the reviewer,
and it is wrong whenever the check would in fact have run. Assume it would have:
suites that sound heavyweight often are not. Dingo's conformance vectors default
to an in-memory SQLite backend and need no setup at all — the whole suite is
`go test ./internal/test/conformance/`, about six seconds. Deferring that one as
infrastructure-dependent was simply a failure to read its README.

Check what a suite needs before judging its cost: a README, a `docker-compose.yml`,
or a `--help` on the runner script settles it in one command. If a required suite
genuinely is too slow to finish locally, say so with the measured or documented
duration and let the user decide — that is their call, not a default.

### Ask for network before writing up the gap

The same rule applies to a fact the sandbox blocks. When an investigation stalls
on something that needs the network, a live service, or a public relay, ask for
the permission and get the fact. Do not write the gap up as an open question and
hand it back.

A comment saying "blocked on preprod blocks around slot 86400" is worth nothing
next to the blocks themselves. `dangerouslyDisableSandbox: true` on Bash prompts
the user, which is the point — one prompt beats a paragraph of hedging. An
earlier sandboxed attempt dying on `socket: operation not permitted` is a signal
to ask, not a finding to report.

For Cardano facts specifically:

| Need | Source |
|---|---|
| Raw block CBOR, authoritative and reusable as a fixture | A small gouroboros blockfetch client against a public relay: `preprod-node.play.dev.cardano.org:3001`, `preview-node.play.dev.cardano.org:3001`, `backbone.cardano.iog.io:3001` |
| Locating a block by `abs_slot` | Koios, `{net}.koios.rest/api/v1` — but it omits Byron EBBs and gates some `tx_info` fields, so use it to find points and the relay to read bytes |
| Reference-implementation behavior | `raw.githubusercontent.com/IntersectMBO/cardano-ledger`, fetching the era's own file, since a later era may shadow the function |

Pair every absence claim with a control proving the method would have shown the
thing had it been there.

## Evidence ledger

The ledger is task-handoff material for the requester. Do not copy it, its
command list, or its skipped-check list into a pull request description or
review comment. A PR is for the problem, code, and resulting behavior. Include
an individual validation fact there only when it materially affects the code
review or merge-risk decision.

Record for every check:

| Field | Why |
|---|---|
| Command | Exactly as run, including flags |
| Directory | Which repository and module — nested modules are separate |
| Exit code | The verdict, not your reading of the log |
| Result | pass / fail / pre-existing failure / skipped |
| Evidence | Log path or the decisive output lines |

A root `go test ./...` does not cover nested modules; list each module you
actually tested. A green single-architecture Docker build says nothing about the
manifest. A backgrounded suite that exits zero without running anything is a
skip, not a pass.

Read the log, not the notification. A task-completion notice can report success
for a run whose own output ends in `FAIL` and a non-zero exit — the notification
describes the process, the log describes the tests. Open the log, find the exit
code, and confirm the package you changed appears with `ok`.

The same rule applies to a verification you perform on yourself: a regression
check that failed to compile did not check anything, and a test run that
reported a build failure is not evidence that the test would have failed.

## Findings order

Report in this order, so the reader hits the blockers first:

1. behavioral and security defects;
2. API and wire compatibility;
3. missing contract-specific tests, including the negative case;
4. architecture and boundary violations;
5. documentation and generated-code drift;
6. style.

For each finding: path and current line or symbol, the input or state that
triggers it, the observable wrong behavior, and how you confirmed it.
Distinguish confirmed defects from suspicions you could not resolve, and
distinguish merge blockers from non-blocking recommendations.

## Handoff structure

1. **What changed** — repositories, paths, and whether the parent workspace
   records only a submodule pointer.
2. **Evidence ledger** — the table above.
3. **Skipped checks** — each with the concrete blocking reason.
4. **Findings** — in severity order, with classification.
5. **Change-bar documents** — for Dingo, whether `DATABASE.md` and
   `ARCHITECTURE.md` were updated or checked and unaffected; for API changes,
   generated output and downstream callers.
6. **Open risks and follow-ups** — what belongs in an issue, in which
   repository. Plans and scratch notes stay local and uncommitted.

Keep it short and factual. A handoff is a record, not a narrative: no
storytelling, no transcript, no roadmap, no speculation about future work
beyond the named follow-ups.
