# Live infrastructure change workflow

Use this reference after the repository and environment are known. Command
examples show repository conventions; confirm current workflow versions and
environment names before running them.

## Repository map

| Repository or path | Owns | Reusable source |
|---|---|---|
| `infrastructure/terraform/` | Live Cloudflare, GCP, AWS, and Grafana state | `terraform-modules` where a shared module is consumed |
| `infrastructure/helmfile-app/` | Workloads on the Demeter cluster | `helm-charts`, except private charts under `helmfile-app/charts/` |
| `infrastructure/ansible/` | Bare-metal and cloud-host inventory and playbooks | `ansible-cardano` for the Cardano roles |
| `vpn-infrastructure/terraform/` | Live AWS VPN infrastructure | `terraform-modules` |
| `vpn-infrastructure/helmfile-app/` | VPN-cluster workloads | `helm-charts` |
| `infrastructure/grafana/` | Dashboard and alert inputs applied by Terraform | Terraform configuration in the same repository |
| `infrastructure/docs/` and `ops/` | Architecture and operational runbooks | None; keep them current with changed operation |

The Terraform roots use remote Cloudflare R2 backends with different state
keys. Never assume that matching backend hosts mean matching state.

## Read-only preparation

1. Inspect `git status --short`, current branch, and `origin/main`. Do not
   switch a dirty primary checkout in place.
2. Read `docs/architecture.md`, the affected subtree README, the relevant
   workflow, and the current pin or values layers.
3. Identify consumers of a module, chart, role, secret, DNS name, service
   account, or dashboard before editing it.
4. Run the smallest local checks that do not need credentials.

### Terraform

From the affected Terraform root, use the repository's configured Terraform or
OpenTofu version:

```sh
terraform fmt -check -diff
terraform init -backend=false
terraform validate
```

`init -backend=false` and `validate` check configuration, not live state or
provider behavior. The repository wrappers load `.env` and credentials; do not
use them for a check that does not need those values merely for convenience.

For an authorized live plan:

```sh
terraform init -input=false
terraform plan -input=false -out=/tmp/<unique-plan-name>
terraform show -no-color /tmp/<unique-plan-name>
```

Keep plan files out of the repository. Review every `-/+`, `destroy`, moved
address, backend or provider change, and unknown value affecting identity,
networking, storage, IAM, certificates, or DNS. Recreate the plan immediately
before an approved apply; do not apply a stale artifact from another head.

### Helmfile

The live repositories use layered values: defaults, cluster-type defaults,
environment values, and SOPS-encrypted secrets. Render the exact target:

```sh
cd helmfile-app
helmfile -e <environment> -l app=<application> template
```

Validate rendered YAML and any embedded JSON or configuration. An authorized
remote comparison adds:

```sh
kubectl config current-context
helmfile -e <environment> -l app=<application> diff
```

Treat a chart pin change as a source-to-consumer boundary: confirm that the OCI
chart version exists, render the live values against it, and inspect StatefulSet,
PVC, Service, RBAC, NetworkPolicy, and probe changes.

### Ansible

Install the pinned collections in an isolated cache, then syntax-check or run
the narrow live-inventory check authorized for the target:

```sh
scripts/run_ansible.sh --syntax-check
scripts/run_ansible.sh --check --diff --limit <host-or-group> --tags <role>
```

Read the role defaults and tasks plus the live group/host variables. Confirm
which handlers fire and whether a container recreation or service restart is
expected. A broad `all` run is not a substitute for selecting the intended
host and tag.

## Mutation gate

Immediately before mutation, record:

- exact commit and clean scoped diff;
- exact account/project, kube context, environment, state, selector, tags, and
  limit;
- fresh plan or diff and its creation time;
- expected creates, updates, restarts, replacements, and deletes;
- workload-specific availability and recovery criteria; and
- the explicit authorization for this target and command.

Prefer the repository's manual workflows for live execution when they supply
the intended credentials and audit trail. Inspect their input expansion first:
the current workflows accept CLI argument strings and some defaults are
mutating.

## Post-change verification

Do not stop at a zero exit code. The Helmfile configuration intentionally sets
`wait: false` and `atomic: false`, and Ansible/Terraform success says nothing
about application-level Cardano health.

- Terraform: refresh or plan again and confirm the intended delta is gone;
  inspect resource identity and outputs without exposing sensitive values.
- Kubernetes: observe rollout completion, pod restarts, PVC identity, Events,
  service endpoints, probes, logs, metrics, and the affected application's
  functional health.
- Ansible: confirm service/container state and, when safe, rerun the same scope
  to establish idempotence.
- Cardano: distinguish process readiness from sync and forging health; use the
  dedicated block-producer skill for KES/opcert or HA-sensitive checks.

If the verification fails, stop broadening the rollout. Preserve diagnostics,
state the last known-good revision and data state, and use the prepared recovery
path rather than improvising a destructive rollback.
