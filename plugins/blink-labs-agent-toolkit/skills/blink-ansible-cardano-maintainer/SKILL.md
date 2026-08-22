---
name: blink-ansible-cardano-maintainer
description: Maintain Blink Labs Cardano Ansible roles in ansible-cardano and their pinned use by infrastructure/ansible, including role variable contracts, Docker service lifecycle, templates, idempotence, collection validation, Galaxy releases, and consumer upgrades. Use for Ansible collection changes, reviews, role releases, or live collection pin updates; use blink-infrastructure-operator for host execution.
---

# Blink Ansible Cardano Maintainer

Use this skill for the reusable `blinklabs.cardano` collection and its
consumer contract. Read
[references/collection-contract.md](references/collection-contract.md), the
role README, defaults, tasks, templates, metadata, and the live variables that
consume the role.

Use [infrastructure-reviewer](../infrastructure-reviewer/SKILL.md) for
deployment-risk review. Any command against real inventory or hosts uses
[blink-infrastructure-operator](../blink-infrastructure-operator/SKILL.md) and
requires explicit authorization.

## Keep collection and live configuration separate

- Reusable Cardano installation behavior belongs in `ansible-cardano/roles/`.
- Host groups, host variables, enabled services, credentials, and deployment
  sequencing belong in `infrastructure/ansible/`.
- The live repository pins `blinklabs.cardano` in
  `ansible/requirements.yml`. Release the collection before moving that pin.
- A one-host workaround is not a collection default unless it is safe and
  correct for every consumer of the role.

Role variables are public APIs. Preserve names, types, defaults, paths, and
boolean behavior, or document a migration. Keep defaults free of secrets and
use `no_log` narrowly when a task must handle sensitive data.

## Role invariants

1. Tasks are idempotent and use fully qualified module names. Commands and
   shell fragments need explicit change detection and quoting.
2. Templates, copy tasks, and configuration changes notify a handler only when
   a restart or reload is required. Review handler ordering and avoid restarting
   multiple Cardano services together without an availability plan.
3. Use pinned `ghcr.io/blinklabs-io` images when they exist. Confirm the role's
   image tag and documented application version agree.
4. Preserve data, IPC, config, topology, and key directory ownership and mount
   semantics. Block-producer cold keys never belong in automation; hot keys and
   opcerts require restrictive file modes and explicit scope.
5. Check mode must not be represented as side-effect-free unless the involved
   modules and tasks support it. A role that shells out to Docker or downloads
   remote state may need a disposable integration target for proof.

## Validate and release

Run the collection's native sanity gate and add lint or role-scoped checks when
available:

```sh
ansible-test sanity
ansible-lint
ansible-galaxy collection build --force --output-path <temporary-directory>
```

There are currently no repository integration or unit suites, so do not imply
that sanity testing proves idempotence or service health. Test meaningful role
behavior on a disposable target when possible. Access to live Blink hosts is a
separate, explicitly authorized operation.

Releases use repository tags `vX.Y.Z`; the release workflow replaces the
placeholder `galaxy.yml` version while packaging. Keep the tracked placeholder
intact. After the tag and Galaxy artifact exist, update the live collection
pin separately, run a narrowly scoped authorized check-mode pass, review
container recreation and handlers, then perform and verify any approved host
rollout.
