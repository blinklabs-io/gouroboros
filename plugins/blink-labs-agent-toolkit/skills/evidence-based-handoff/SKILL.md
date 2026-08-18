---
name: evidence-based-handoff
description: Report Blink Labs work so the result can be trusted without repeating it — an evidence ledger of commands and exit codes, an explicit skipped-check list with reasons, findings ordered by severity, and no claim that unavailable live infrastructure was exercised. Use before finishing a task, opening a pull request, or handing off to a reviewer.
---

# Evidence-Based Handoff

Use this skill when about to say work is done, fixed, passing, or reviewed. The
standard is simple: every claim traces to output you actually read.

## The rule

State only what you ran. If a check was not run, it is in the skipped list with
a reason. "Should pass", "presumably fine", and "tests look correct" are not
results. A change with no executed checks is unvalidated — say that plainly
rather than describing it as complete.

Live GitHub, container registries, the Cardano devnet, conformance suites, and
Antithesis are frequently unavailable. Never describe any of them as having run
when they did not. Naming the exact skipped check and the blocking reason is a
complete, acceptable answer; implying coverage is not.

## Evidence ledger

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
