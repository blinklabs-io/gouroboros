# Ansible Cardano collection contract

## Role anatomy

Each public role may expose:

- `defaults/main.yml`: the public variable surface and default image/version;
- `tasks/main.yml`: validation and install-method routing;
- `tasks/docker.yml`: directories, configuration, container definition, and
  lifecycle;
- `templates/`: topology, configuration, or helper scripts;
- `meta/main.yml`: Galaxy role metadata and dependencies; and
- `README.md`: consumer-facing variables and examples.

Read all affected pieces. Updating only a default while leaving Docker tasks,
templates, and documentation on the old assumption creates a valid-looking but
broken collection release.

## Consumer mapping

The primary live consumer is `infrastructure/ansible/`:

- `requirements.yml` pins the collection release;
- `node.yml` selects roles by tags and `when: <service>.enabled`;
- `group_vars/` and `host_vars/` supply environment-specific values; and
- `scripts/run_ansible.sh` is the live playbook entry point.

Search other Blink checkouts for `blinklabs.cardano.<role>` and the variable
prefix before changing a public input. Do not read or reproduce untracked
inventory, token, wallet, or event files encountered during the search.

## Idempotence and lifecycle checklist

- Use declarative modules rather than unconditional command or shell tasks.
- When a command is necessary, set `creates`, `removes`, `changed_when`, and
  `failed_when` from observable behavior.
- Quote Jinja values in YAML and command arguments; do not construct shell from
  untrusted inventory values.
- Assign owner, group, and mode explicitly for data directories, configs,
  sockets, topology, and key files.
- Keep persistent data mounts stable across image upgrades.
- Notify handlers from changed configuration instead of restarting on every
  play. Flush handlers only where later tasks require the new state.
- Distinguish service reload, container recreation, and data migration in the
  role and handoff.
- Treat check-mode skips and unsupported modules as gaps, not passes.

For Cardano nodes, validate network configuration, topology, port exposure,
snapshot behavior, probe or health logic, shutdown time, and storage retention.
For block producers, serialize changes and apply the dedicated Dingo operator
guidance when Dingo forging, KES, opcert, or HA behavior is involved.

## Validation ladder

1. YAML and Jinja syntax for every changed file.
2. `ansible-test sanity` using the repository-supported Ansible line.
3. `ansible-lint` with findings classified as introduced or pre-existing.
4. `ansible-galaxy collection build` into a temporary directory; inspect the
   artifact name and contents without publishing it.
5. Role execution against a disposable target, twice, proving the second run
   is unchanged where the role promises idempotence.
6. Authorized live consumer `--check --diff --limit ... --tags ...`, followed
   by an approved rollout and service-specific health verification.

The repository CI currently runs sanity only; steps 3–6 must be reported
separately and never inferred from the CI job.

## Release handoff

The collection uses repository-wide `vX.Y.Z` tags. The tracked
`galaxy.yml` version is a placeholder replaced during the tagged release; do
not bump it in source. Before updating `infrastructure/ansible/requirements.yml`:

- verify the tag points to the reviewed commit;
- verify the Galaxy artifact exists;
- name changed role variables or lifecycle behavior;
- keep the consumer pin update independently reviewable; and
- render or check the exact hosts and tags that will consume the new version.
