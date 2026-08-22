---
name: blink-terraform-module-maintainer
description: Maintain Blink Labs reusable Terraform modules in terraform-modules, including input and output contracts, provider constraints, resource-address stability, module-scoped release tags, and pinned consumers in infrastructure and vpn-infrastructure. Use for module changes, module reviews, provider upgrades, releases, or consumer pin updates; use blink-infrastructure-operator for live plans and applies.
---

# Blink Terraform Module Maintainer

Use this skill for reusable module source, not live state. Read
[references/module-contract.md](references/module-contract.md), then inspect the
affected module's README, variables, outputs, providers, resources, recent
module-scoped tags, and all consumers.

Use [infrastructure-reviewer](../infrastructure-reviewer/SKILL.md) for blast
radius and destructive-plan review. Live plans or applies in consuming
repositories use
[blink-infrastructure-operator](../blink-infrastructure-operator/SKILL.md).

## Treat modules as versioned APIs

- Inputs include names, types, defaults, validation, nullability, and the keys
  used for `for_each` or resource names.
- Outputs include names, types, sensitivity, and whether their values remain
  known at plan time.
- Provider source and version constraints are part of the compatibility
  contract with every root module.
- Resource addresses are persisted in state. Renaming a resource, changing
  `count`/`for_each`, or changing map keys can destroy and recreate real
  infrastructure even when the cloud object arguments look equivalent.

Prefer additive optional inputs and outputs. For an intentional address move,
provide a `moved` block when Terraform can represent it. For an unavoidable
breaking change, document the migration and use the repository's module-scoped
semantic-release signal; do not hide it inside a consumer pin bump.

## Find consumers before editing

Search independent Blink checkouts and the `clanker` workspace for both the
module directory and its tag form:

```sh
rg 'terraform-modules(\.git)?\?ref=<module>/|source\s*=.*<module>' \
  /path/to/blink /path/to/clanker
```

At minimum inspect `infrastructure`, `vpn-infrastructure`, module README
examples, and any application-specific infrastructure repository found by the
search. Record missing or stale checkouts; absence from one workspace is not
proof that a public module has no consumers.

## Implementation and validation

Match the module's existing provider and collection patterns. Pin external
module versions and constrain providers compatibly; do not introduce a Blink
fork without explicit approval, issue tracking, and an exit plan.

Run the same broad checks as CI, plus focused checks in the changed module:

```sh
terraform fmt -check -recursive -diff
cd <module>
terraform init -backend=false
terraform validate
```

Initialization downloads providers and modules and writes `.terraform/`; use a
clean or isolated checkout and request network access when needed. Validate
README examples and update them with interface changes. Static validation does
not show resource replacement; an authorized plan in each material consumer is
the evidence for state behavior.

## Release and consumer sequence

The repository releases modules independently with tags shaped
`<module>/vX.Y.Z`. Merge and verify the module tag before changing a live
consumer to it. Consumer sources stay pinned to an immutable module tag, never
`main`, a feature branch, or an unpublished ref.

Update live consumer pins separately, run init with the new source, and inspect
a fresh plan for deletes and replacements. Do not commit incidental lock-file
or provider upgrades unless they are part of the requested change.
