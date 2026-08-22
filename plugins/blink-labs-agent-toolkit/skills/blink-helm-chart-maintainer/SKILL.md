---
name: blink-helm-chart-maintainer
description: Maintain and review Blink Labs Helm charts in helm-charts and private charts under infrastructure/helmfile-app/charts, including values/template contracts, CRD and RBAC sync, chart versions, OCI release inputs, and consumer pins. Use for chart code, chart reviews, or operator-to-chart manifest changes; use blink-infrastructure-operator for deploying a released chart.
---

# Blink Helm Chart Maintainer

Use this skill for chart source and packaging. Read
[references/chart-contract.md](references/chart-contract.md) for the detailed
change and validation matrix, and use
[infrastructure-reviewer](../infrastructure-reviewer/SKILL.md) for state and
blast-radius review.

## Locate the chart boundary

- Public OCI charts live in `helm-charts/charts/<name>/` and publish to GHCR.
- Private application charts live under
  `infrastructure/helmfile-app/charts/` and ship with that live repository.
- Operator CRDs and RBAC originate in the operator repository. Regenerate them
  there, then synchronize the chart packaging and values that expose them.
- Live values and chart pins belong in `infrastructure` or
  `vpn-infrastructure`; deploying them uses
  [blink-infrastructure-operator](../blink-infrastructure-operator/SKILL.md).

Inspect the chart's `Chart.yaml`, `values.yaml`, templates, README, publish
workflow, image-update configuration, source application, and all live
consumers before editing. A value is a public API even when Helm has no schema
file.

## Preserve the chart contract

1. Trace every changed value through template output and documentation.
   Preserve zero, `false`, empty-list, and omitted-value semantics deliberately.
2. Keep selectors and immutable fields stable. Never put mutable version,
   network, or service-mode labels into a StatefulSet or Deployment selector.
3. Treat StatefulSet names, service names, `volumeClaimTemplates`, storage
   classes, and PVC retention as persisted interfaces.
4. Guard required key material and incompatible modes with actionable template
   failures. Never render Cardano cold keys or secrets into logs or environment
   variables.
5. Scope service accounts, Roles, ClusterRoles, and NetworkPolicies to the
   controller or workload behavior that actually needs them.
6. Keep `Chart.yaml` `version`, `appVersion`, default image tag, and automation
   metadata consistent. Every published chart content change needs a chart
   version bump; `appVersion` changes only with the packaged application.

When two charts intentionally mirror one another, or the request names a
baseline chart, compare the complete relevant template path. Apply a confirmed
shared defect to each affected chart, but do not spread chart-specific behavior
merely for cosmetic consistency.

## Validate behavior, not only YAML syntax

Render a matrix containing defaults, every changed feature enabled, relevant
feature combinations, and invalid inputs that must fail. At minimum run:

```sh
helm lint --strict charts/<name>
helm template <release> charts/<name> -f <representative-values>
helm package charts/<name> -d <temporary-directory>
```

Use `ct lint` for the changed-chart set. Use `ct install` or a disposable kind
cluster when Kubernetes admission or runtime behavior matters; do not install
into an existing cluster without explicit authorization. Parse embedded JSON,
TOML, or application configuration with its native parser after rendering.

For a review, test the PR merged with current `origin/main` in an isolated
worktree when the primary checkout is dirty. Validate negative guards as well
as successful renders; a default render cannot exercise optional topology,
keys, metrics, RBAC, or service paths.

## Release and consumer handoff

The image-update workflow treats registry tags as authoritative and updates the
default image tag, `appVersion`, and chart patch version together. Verify the
exact image exists and, for multi-architecture workloads, that required
architectures exist. Do not publish a chart locally without authorization.

After a public chart is released, update live consumers in a separate change,
render their real values, and review the resulting rollout. Never point a live
consumer at an unpublished version, branch, or mutable tag.
