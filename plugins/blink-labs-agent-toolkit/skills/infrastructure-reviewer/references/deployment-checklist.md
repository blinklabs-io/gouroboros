# Deployment review checklist

## Repository roles

| Repository | Role | Native checks |
|---|---|---|
| `terraform-modules` | Reusable cloud modules (AWS, Cloudflare, …) | `terraform fmt -check`, `terraform init -backend=false`, `terraform validate` per module |
| `helm-charts` | Public chart collection | `helm lint`, `helm template`, chart-testing |
| `infrastructure` | Live configuration: helmfile, Ansible, Grafana, ops runbooks, private app charts | Depends on the subtree; read `infrastructure/docs` first |
| `ansible-cardano` | Ansible Galaxy collection for Cardano hosts | `ansible-lint`, `ansible-test sanity`, role-scoped molecule where present |
| `dingo-operator` | Kubernetes operator for Dingo | Go tests plus CRD/manifest generation targets |
| `cardano-compose-stacks` | Compose integration and Antithesis environments | `docker compose config`, upstream version checks |

## Change-specific checks

**Terraform**
- Read the module's variables and outputs as an API: a renamed or retyped
  variable is a breaking change for every consumer.
- Look for `force_new`/replacement triggers on stateful resources, buckets,
  volumes, DNS records, and certificates.
- Check provider and module version constraints; an unpinned provider makes the
  plan non-reproducible.
- Confirm backend and workspace configuration was not altered incidentally.

**Helm**
- Render with the real values files, not just defaults.
- Check StatefulSet update strategy, PVC retention, and whether the change
  triggers a pod restart on a syncing node.
- Check liveness/readiness/startup probe budgets against genuine Cardano
  startup times — a node replaying the ledger is not unhealthy.
- Bump `Chart.yaml` version for a chart change; consumers pin it.

**Ansible**
- Roles must stay idempotent; a second run should change nothing.
- Check handler ordering around service restarts of a running node.
- Never place secrets in plain vars; verify vault or external secret usage.

**Kubernetes and operators**
- CRD changes are API changes: check backward compatibility, stored versions,
  and conversion.
- Verify RBAC is scoped, not cluster-admin by default.
- Check resource requests and limits against real node memory and disk profiles.

**Compose stacks**
- Pin image tags; confirm volume mounts do not discard chain state between runs.
- Keep Antithesis and integration environments isolated from ordinary local CI.

## Hard constraints

- No credentials, tokens, kubeconfigs, private hostnames, or cluster endpoints
  committed to a repository.
- No apply, deploy, destroy, rollout, or secret rotation during a review.
- Prefer `blinklabs-io` images; document and pin any exception.
- A green lint is not evidence of a safe rollout — say what you could not verify.
