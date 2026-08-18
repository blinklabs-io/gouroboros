---
name: infrastructure-reviewer
description: Review Blink Labs deployment and infrastructure changes across Ansible collections, Helm charts, Terraform modules, helmfile application configuration, Kubernetes operators, and compose stacks. Use for repos/infrastructure, helm-charts, terraform-modules, ansible-cardano, dingo-operator, cardano-compose-stacks, or any change that alters how a Cardano service is deployed or operated.
---

# Infrastructure Reviewer

Use this skill for changes that alter deployed state. The failure mode here is
different from application code: a change can be syntactically valid, pass every
lint, and still take a node down, expose a secret, or destroy a volume.

Read [deployment-checklist.md](references/deployment-checklist.md), then the
target repository's own instructions.

## Method

1. **Identify the blast radius.** Which environments, clusters, hosts, or
   networks does this change reach? A shared module or chart change reaches
   every consumer of it — enumerate them before reviewing the diff.
2. **Separate the layers.** `terraform-modules` and `helm-charts` are reusable
   sources; `infrastructure` is the live configuration that consumes them.
   A fix usually belongs in the source module, with the consumer pinned forward
   afterwards. Private application charts live in
   `infrastructure/helmfile-app/charts/` and are not public chart material.
3. **Check state safety.** For Terraform, read the plan for replacements and
   destroys, not just additions; look for changes that force recreation of
   stateful resources, storage, or DNS. For Helm, check volume claims, stateful
   set update strategies, and whether a template change causes a rollout of a
   syncing Cardano node.
4. **Check secrets and permissions.** No credentials, tokens, kubeconfigs, or
   private endpoints in the repository. Verify how secrets are injected, which
   service accounts and IAM roles are granted, and whether a permission was
   widened without a stated reason.
5. **Check Cardano operational reality.** Node containers carry large chain
   state and long sync times. Review resource requests and limits, probe
   timeouts against real startup time, persistent volume retention, network
   magic and configuration selection, and image tags — a floating tag on a node
   image is an availability risk.
6. **Prefer Blink images.** When a `blinklabs-io` image exists, use it. Record
   why for any upstream or third-party image, and how its tag or digest is
   pinned.

## Validation

Use native tooling per repository and say which ran:

```sh
ansible-lint                     # ansible-cardano, infrastructure/ansible
ansible-test sanity              # collection changes
helm lint <chart>                # helm-charts, helmfile-app charts
helm template <chart> -f <values># render before trusting a values change
terraform fmt -check             # per module, not repository-wide only
terraform validate               # per module, after init
kubectl apply --dry-run=server   # only against a cluster you are authorized for
docker compose config            # cardano-compose-stacks
actionlint                       # changed workflows
```

`terraform plan`, `helmfile apply`, cluster access, and anything that mutates
live infrastructure require explicit authorization. Never apply, deploy,
destroy, or rotate as part of a review. If a plan could not be produced, say so
rather than reasoning about what it probably would have shown.

## Report

Cover blast radius, state-destroying operations, secret and permission changes,
resource and probe realism for Cardano workloads, image and version pinning,
drift between a shared module and its consumers, and the checks you could not
run.
