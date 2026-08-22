# Terraform module contract checklist

## Change surface

Review each layer that can change a consumer:

| Layer | Compatibility questions |
|---|---|
| Variables | Did a name, type, default, validation, null behavior, or object attribute change? |
| Outputs | Did a name, type, sensitivity flag, or plan-time availability change? |
| Providers | Do source, aliases, constraints, and root-provider expectations still compose? |
| Resources | Did an address, key, name, region, lifecycle rule, or replacement trigger change? |
| External modules | Is the source immutable, canonical, licensed, and compatible with existing state? |
| Documentation | Do examples use a real released tag and every required input? |

Complex object variables are APIs. Adding an `optional()` attribute is usually
additive; changing an attribute's type or using a new value in a resource name
may not be. Preserve unknown and null semantics rather than coercing them merely
to satisfy validation.

## State-risk review

Look explicitly for:

- resource block renames and `count` to `for_each` conversions;
- `for_each` keys derived from mutable display names or unordered values;
- cloud names, zones, regions, account/project IDs, or certificate domains that
  force replacement;
- storage, KMS, IAM, DNS, load balancer, cluster, and network resources whose
  replacement has durable effects;
- removed lifecycle protections or newly broad IAM principals/actions;
- provider major-version changes that alter defaults, identifiers, or import
  formats; and
- output changes that feed downstream resource identity.

Use `moved` blocks for representable address changes. Do not add
`prevent_destroy`, `ignore_changes`, or state surgery as a reflex; each can hide
real drift and needs a documented operational reason.

## Provider-family checks

### AWS

Check account and region assumptions, IAM trust and policy scope, KMS key
administrators/users, S3 retention/encryption, VPC/subnet identity, EKS addon
and node-group replacement, and ACM validation records.

### GCP

Check project ownership, APIs, service accounts, Workload Identity bindings,
KMS protection and location, GKE release channels/node pools, and provider
alias requirements.

### Cloudflare

Check whether resources are account- or zone-scoped, DNS proxy behavior,
ruleset phase/order, tunnel credentials, Pages environment variables, load
balancer health monitors, and provider v4/v5 schema differences.

### Grafana

Check stable dashboard/alert identifiers, folder and data-source references,
JSON normalization, contact points, evaluation intervals, and whether exported
configuration must be converted before use.

## Validation evidence

Record separately:

1. recursive formatting result;
2. `init -backend=false` and `validate` for every changed module;
3. README/example review;
4. consumer search results, including missing checkouts;
5. authorized consumer plans and all replacements/deletes; and
6. skipped provider or live integration checks with the concrete reason.

A successful module `validate` proves only configuration consistency. It is not
evidence that an upgrade preserves live resources.

## Release tags and consumers

The release workflow derives each module's next semantic version from merged
pull-request content and creates a module-scoped tag such as
`aws_eks/v0.2.3`. Keep a change scoped enough that its release signal is clear.
Do not edit tags or fabricate a tag in a consumer; verify the repository shows
the released tag and that it contains the intended module tree.

Consumer updates should name the old and new tags, the module contract change,
the plan result, and any required migration. Keep the module release and each
live consumer rollout independently reviewable.
