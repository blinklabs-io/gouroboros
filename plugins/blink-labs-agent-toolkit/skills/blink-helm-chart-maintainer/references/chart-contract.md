# Blink Helm chart contract

## Source and consumer map

| Change | Source of truth | Follow-up |
|---|---|---|
| Public workload chart | `helm-charts/charts/<name>/` | Release OCI chart, then move live Helmfile pins |
| Private application chart | `infrastructure/helmfile-app/charts/<name>/` | Render and review in the live repository |
| Operator CRD or RBAC | Operator API markers and generated `config/` | Regenerate in source, then synchronize chart `crds/` and RBAC |
| Application flags, ports, probes, or config | Application repository | Update chart values/templates and live consumers together as needed |
| Default container version | Published container registry tag | Update image tag, `appVersion`, chart version, and updater metadata |

Search both the independent checkouts and `clanker/repos/`; a missing or dirty
checkout must not be mistaken for absence of a consumer.

## Template review checklist

### Object identity and selectors

- Workload `spec.selector.matchLabels`, pod-template labels, and Service
  selectors agree.
- Service selectors include the intended component identity and exclude
  CronJob pods or sibling controllers that share the chart's common labels.
- The chart renders exactly the intended controller set; a Deployment and
  StatefulSet must not accidentally run the same application behind one
  Service.
- Immutable selectors contain only identity labels, not image, chart, network,
  role, mode, or version labels that may change on upgrade.
- Names stay inside Kubernetes length limits after release and instance names
  are expanded, including resource-specific suffixes.
- Namespace assumptions are explicit for cross-namespace RBAC, NetworkPolicy,
  and service discovery.

### Stateful workloads

- `volumeClaimTemplates` name, access modes, storage class, size, and retention
  changes are treated as migration decisions, not ordinary rollout fields.
- A config checksum or image change restarts only the intended pods.
- Update strategy, disruption budget, anti-affinity, termination grace, and
  probe budgets match long Cardano startup, replay, and shutdown times.
- Existing PVC identity and chain data survive ordinary upgrades and pod
  replacement.
- When init-container ownership, UID/GID, `fsGroup`, or file-mode behavior
  changes, recreate a pod against the same PVC in a disposable cluster and
  verify the second boot; a successful first mount does not cover remounts.

### Values and generated configuration

- Each documented value is read at the documented path and type.
- Omitted, empty, `false`, and zero values render intentionally; `default` and
  truthiness do not erase legitimate values.
- Required secrets or block-producer key files fail rendering before a pod can
  start with partial material.
- Commas, quoting, and indentation remain valid in embedded JSON, TOML, shell,
  and YAML after optional blocks are toggled.
- User-provided strings are quoted and cannot become template code or unsafe
  shell fragments.
- Application listeners, container ports, Service target ports, and probes
  stay aligned when users override addresses or ports.

### Security and operators

- Cold signing keys never enter the chart or cluster. Hot keys and opcerts are
  file-mounted with appropriate ownership and modes.
- RBAC has no unexplained wildcard resources or verbs. Namespaced permissions
  remain namespaced when cluster scope is unnecessary.
- CRD schema, served/storage versions, conversion, and generated deepcopy
  behavior agree with operator source.
- The chart Deployment includes labels, ports, and permissions required by the
  controller's NetworkPolicies and event APIs.
- NetworkPolicy includes DNS and control-plane paths the workload genuinely
  needs without opening unrelated node ports.

## Render matrix

Choose cases from the changed contract rather than copying a fixed list:

| Case | What it proves |
|---|---|
| Default values | Installable baseline and default object identities |
| Each changed feature enabled alone | The path is reachable and templates are complete |
| Interacting features enabled together | Conditions and generated config compose |
| Explicit `false`, zero, and empty values | Values are not erased by truthiness/defaulting |
| Required input omitted | The chart fails early with accurate guidance |
| Upgrade-relevant old and new values | Selectors, names, PVCs, and service identity remain compatible |

Inspect the rendered objects directly. For JSON-bearing ConfigMaps or Secrets,
extract and run `jq`; for scripts use `bash -n` or the correct shell; for
application config use the application's parser when available.

## Repository-native release checks

- `ct list-changed --target-branch <base>` identifies the chart set used by CI.
- `ct lint --target-branch <base>` covers chart metadata and lint conventions.
- The per-chart publish workflow must include the chart path and its own
  workflow path in the trigger, use minimal package permissions, package the
  intended directory, and push to the expected OCI namespace.
- `scripts/upstream-versions.json` is required only for charts managed by the
  automated registry-version workflow. Its tag regex must exclude floating,
  prerelease, or incompatible tags deliberately.
- Container registries are authoritative for installable image versions. The
  resolver must reject equal or decreasing versions, including packaging
  revisions that do not appear in repository release tags.
- `scripts/update-chart-version.sh` updates the primary image tag,
  `appVersion`, and chart patch version. Review its diff; nested images are not
  changed automatically.
- Verify the token used to open automated pull requests actually triggers the
  repository's required `pull_request` checks; a PR created with the default
  `GITHUB_TOKEN` does not trigger another workflow run.

`ct install`, live `helm install/upgrade`, registry push, and Helmfile
`apply/sync` are external mutations and require explicit authorization.
