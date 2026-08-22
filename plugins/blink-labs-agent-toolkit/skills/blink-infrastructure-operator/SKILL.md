---
name: blink-infrastructure-operator
description: Maintain and operate Blink Labs live infrastructure in the infrastructure and vpn-infrastructure repositories using Terraform, Helmfile, Ansible, SOPS, Grafana, and runbooks. Use for live configuration edits, drift investigation, plans, diffs, approved rollouts, or post-deploy verification; use the source-maintainer skills for reusable Helm charts, Terraform modules, and Ansible roles.
---

# Blink Infrastructure Operator

Use this skill for the repositories that describe deployed Blink Labs state.
These are control planes: a syntactically valid command can replace a cluster,
restart a syncing node, expose a service, or make encrypted material visible.

Read [references/change-workflow.md](references/change-workflow.md) before
planning or executing a live change. Also use
[infrastructure-reviewer](../infrastructure-reviewer/SKILL.md) to review blast
radius and state safety. For a Dingo block producer incident or key lifecycle
operation, use
[dingo-block-producer-operator](../dingo-block-producer-operator/SKILL.md).

## Establish the target

Before a command that reads or changes remote state, name all of these:

- repository and IaC root;
- environment, cluster, host group, or Terraform state key;
- Helmfile selector, Ansible tags and limit, or Terraform resource scope;
- current Kubernetes context and cloud account/project when applicable; and
- whether the action is render-only, remote read, or mutation.

Do not infer a production target from the current shell context. Read the
repository's `docs/architecture.md`, subtree documentation, workflows, and
current configuration. Preserve dirty sibling checkouts and local values files;
use an isolated worktree when the checkout contains unrelated work.

## Keep source and deployment boundaries intact

- `terraform-modules`, `helm-charts`, and `ansible-cardano` are reusable
  sources. Fix shared behavior there, release it, then move the pinned consumer.
- `infrastructure` and `vpn-infrastructure` hold live configuration and pins.
  Do not copy a shared module or chart into them to avoid its release process.
- Private application charts belong under `infrastructure/helmfile-app/charts/`;
  do not publish them merely because public charts use similar templates.
- An operator API, CRD, or generated RBAC change starts in the operator source.
  Package the resulting manifests through the Helm chart after regeneration.

Use
[blink-helm-chart-maintainer](../blink-helm-chart-maintainer/SKILL.md),
[blink-terraform-module-maintainer](../blink-terraform-module-maintainer/SKILL.md),
or
[blink-ansible-cardano-maintainer](../blink-ansible-cardano-maintainer/SKILL.md)
for those reusable-source changes.

## Approval boundary

Local formatting and static validation are ordinary maintenance checks, as is
Helmfile `template` when it uses only non-secret local values. Get explicit
authorization immediately before any command that:

- reads live cluster, host, cloud, or Terraform state, including
  `terraform plan`, `helmfile diff`, and live-inventory Ansible check mode;
- applies, syncs, destroys, imports, taints, rolls out, restarts, drains,
  rotates, or edits a remote object; or
- decrypts a secret or loads deployment credentials.

Authorization must cover the exact environment and operation. A request to
review or prepare a change is not approval to deploy it. Never pass arbitrary
user-controlled CLI arguments through a deployment workflow without reviewing
the expanded command and target.

## Operational invariants

- Never print decrypted values, environment files, plans containing secrets,
  kubeconfigs, or cloud credentials. Keep temporary decrypted material outside
  the repository with restrictive permissions and remove it when finished.
- Inspect Terraform plans for deletes, replacements, address changes, and
  provider or backend drift. A zero-error plan is not an approval to apply.
- Render Helmfile with the exact environment and smallest useful `app` label.
  The live Helmfile defaults disable waiting and atomic rollback, so command
  success is not workload health; perform explicit rollout and service checks.
- Scope Ansible with both tags and a host limit. Check mode is evidence, not a
  guarantee of no side effects. Review handlers and service restarts before a
  live run, then verify idempotence where the role supports it.
- Cardano data is expensive to recreate. Preserve PVCs, node databases, keys,
  network selection, and shutdown budgets, and serialize stateful-node changes.

## Handoff

Report the exact target, pre-change evidence, plan/diff summary, approval that
covered any mutation, post-change health evidence, and rollback or recovery
state. If no live command ran, say so plainly.
