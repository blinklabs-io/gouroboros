---
name: cardano-app-reviewer
description: Review Blink Labs Cardano applications and services for wallet/key safety, transaction construction, DEX and indexer behavior, node integration, configuration, and compatibility. Use for Bursa, Shai, Bluefin, Adder, Dingo integrations, transaction APIs, or external Apollo usage.
---

# Cardano Application Reviewer

Use this skill for application behavior built on the protocol libraries. Read
the [application boundaries reference](references/app-boundaries.md), then the
target repository's local guidance and the relevant upstream library docs.

## Workflow

1. Map the data flow from node input or API request through parsing, state,
   transaction construction, signing, submission, and persistence.
2. Identify trust boundaries and sensitive material: mnemonics, seed files,
   private keys, signing interfaces, network selection, profile parameters,
   wallet storage, and external provider responses.
3. Verify transaction validity, network IDs, fees, validity intervals,
   collateral, reference inputs, datum/script handling, and error propagation.
4. For DEX/indexer work, test rollback, duplicate events, cursor recovery,
   mempool observations, profile configuration, and idempotent persistence.
5. Use deterministic fixed backends and shared `ouroboros-mock` fixtures. Use
   the canonical upstream `Salvionied/apollo` module; Blink forks are emergency
   exceptions only.
6. Run focused tests, race tests for concurrent pipelines, and integration
   checks appropriate to the changed node or provider boundary. Never include
   secrets or live credentials in fixtures, logs, or review artifacts.

Review user-visible failure behavior as carefully as the happy path. A change
that builds and submits a transaction can still be unsafe if it selects the
wrong network, signs the wrong body, or loses rollback state.
