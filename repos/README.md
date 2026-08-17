# Blink Labs repositories

This directory contains Blink Labs repositories tracked as Git submodules.
Each submodule keeps its own history, issues, releases, license, contribution
guidance, and development tooling. This repository records the specific commit
that belongs in a workspace checkout.

## Current repositories

| Path | Repository | Purpose |
| --- | --- | --- |
| [`.github`](.github) | [`blinklabs-io/.github`](https://github.com/blinklabs-io/.github) | Organization-wide contribution, security, and community defaults |
| [`actions`](actions) | [`blinklabs-io/actions`](https://github.com/blinklabs-io/actions) | Reusable GitHub Actions workflows and repository governance engine |
| [`docs`](docs) | [`blinklabs-io/docs-site`](https://github.com/blinklabs-io/docs-site) | Public Blink Labs documentation site |
| [`kb`](kb) | [`blinklabs-io/kb`](https://github.com/blinklabs-io/kb) | Long-form engineering training and reference books |
| [`adder`](adder) | [`blinklabs-io/adder`](https://github.com/blinklabs-io/adder) | Cardano chain-sync event tailer |
| [`adder-mobile`](adder-mobile) | [`blinklabs-io/adder-mobile`](https://github.com/blinklabs-io/adder-mobile) | Mobile app for Adder notifications |
| [`ansible-cardano`](ansible-cardano) | [`blinklabs-io/ansible-cardano`](https://github.com/blinklabs-io/ansible-cardano) | Ansible collection for Cardano services |
| [`bark`](bark) | [`blinklabs-io/bark`](https://github.com/blinklabs-io/bark) | gRPC protocol for Dingo operations |
| [`bluefin`](bluefin) | [`blinklabs-io/bluefin`](https://github.com/blinklabs-io/bluefin) | Cardano TUNA miner |
| [`bursa`](bursa) | [`blinklabs-io/bursa`](https://github.com/blinklabs-io/bursa) | Programmatic Cardano wallet |
| [`cardano-compose-stacks`](cardano-compose-stacks) | [`blinklabs-io/cardano-compose-stacks`](https://github.com/blinklabs-io/cardano-compose-stacks) | Cardano service Docker Compose environment |
| [`cardano-models`](cardano-models) | [`blinklabs-io/cardano-models`](https://github.com/blinklabs-io/cardano-models) | Go models for Cardano on-chain data structures |
| [`cardano-node-api`](cardano-node-api) | [`blinklabs-io/cardano-node-api`](https://github.com/blinklabs-io/cardano-node-api) | Cardano node API service |
| [`cardano-up`](cardano-up) | [`blinklabs-io/cardano-up`](https://github.com/blinklabs-io/cardano-up) | Cardano package and service manager |
| [`cardano-up-packages`](cardano-up-packages) | [`blinklabs-io/cardano-up-packages`](https://github.com/blinklabs-io/cardano-up-packages) | Package definitions consumed by `cardano-up` |
| [`dingo`](dingo) | [`blinklabs-io/dingo`](https://github.com/blinklabs-io/dingo) | Cardano node implementation in Go |
| [`docker-amaru`](docker-amaru) | [`blinklabs-io/docker-amaru`](https://github.com/blinklabs-io/docker-amaru) | Amaru Docker image |
| [`docker-cardano-cli`](docker-cardano-cli) | [`blinklabs-io/docker-cardano-cli`](https://github.com/blinklabs-io/docker-cardano-cli) | Cardano CLI Docker image |
| [`docker-cardano-configs`](docker-cardano-configs) | [`blinklabs-io/docker-cardano-configs`](https://github.com/blinklabs-io/docker-cardano-configs) | Cardano configuration Docker image |
| [`docker-cardano-db-sync`](docker-cardano-db-sync) | [`blinklabs-io/docker-cardano-db-sync`](https://github.com/blinklabs-io/docker-cardano-db-sync) | Cardano DB-Sync Docker image |
| [`docker-cardano-node`](docker-cardano-node) | [`blinklabs-io/docker-cardano-node`](https://github.com/blinklabs-io/docker-cardano-node) | Cardano node Docker image |
| [`docker-cardano-wallet`](docker-cardano-wallet) | [`blinklabs-io/docker-cardano-wallet`](https://github.com/blinklabs-io/docker-cardano-wallet) | Cardano wallet Docker image |
| [`docker-go`](docker-go) | [`blinklabs-io/docker-go`](https://github.com/blinklabs-io/docker-go) | Go toolchain Docker image |
| [`docker-haskell`](docker-haskell) | [`blinklabs-io/docker-haskell`](https://github.com/blinklabs-io/docker-haskell) | Haskell toolchain Docker image |
| [`docker-hydra-node`](docker-hydra-node) | [`blinklabs-io/docker-hydra-node`](https://github.com/blinklabs-io/docker-hydra-node) | Hydra node Docker image |
| [`docker-kupo`](docker-kupo) | [`blinklabs-io/docker-kupo`](https://github.com/blinklabs-io/docker-kupo) | Kupo Docker image |
| [`docker-minio`](docker-minio) | [`blinklabs-io/docker-minio`](https://github.com/blinklabs-io/docker-minio) | MinIO Docker image |
| [`docker-mithril-client`](docker-mithril-client) | [`blinklabs-io/docker-mithril-client`](https://github.com/blinklabs-io/docker-mithril-client) | Mithril client Docker image |
| [`docker-mithril-signer`](docker-mithril-signer) | [`blinklabs-io/docker-mithril-signer`](https://github.com/blinklabs-io/docker-mithril-signer) | Mithril signer Docker image |
| [`docker-ogmios`](docker-ogmios) | [`blinklabs-io/docker-ogmios`](https://github.com/blinklabs-io/docker-ogmios) | Ogmios Docker image |
| [`docker-openvpn`](docker-openvpn) | [`blinklabs-io/docker-openvpn`](https://github.com/blinklabs-io/docker-openvpn) | OpenVPN Docker image |
| [`docker-parity-subkey`](docker-parity-subkey) | [`blinklabs-io/docker-parity-subkey`](https://github.com/blinklabs-io/docker-parity-subkey) | Parity Subkey Docker image |
| [`docker-wireguard`](docker-wireguard) | [`blinklabs-io/docker-wireguard`](https://github.com/blinklabs-io/docker-wireguard) | WireGuard VPN service and Docker image |
| [`go-bip39`](go-bip39) | [`blinklabs-io/go-bip39`](https://github.com/blinklabs-io/go-bip39) | Go BIP-0039 mnemonic library |
| [`go-scls`](go-scls) | [`blinklabs-io/go-scls`](https://github.com/blinklabs-io/go-scls) | Go Standard Canonical Ledger State library |
| [`gouroboros`](gouroboros) | [`blinklabs-io/gouroboros`](https://github.com/blinklabs-io/gouroboros) | Go Ouroboros/Cardano protocol library |
| [`helm-charts`](helm-charts) | [`blinklabs-io/helm-charts`](https://github.com/blinklabs-io/helm-charts) | Blink Labs Kubernetes Helm charts |
| [`issues`](issues) | [`blinklabs-io/issues`](https://github.com/blinklabs-io/issues) | Shared repository for uncategorized issues |
| [`merkle-patricia-forestry`](merkle-patricia-forestry) | [`blinklabs-io/merkle-patricia-forestry`](https://github.com/blinklabs-io/merkle-patricia-forestry) | Merkle Patricia Forestry data structure library used by Bluefin |
| [`nview`](nview) | [`blinklabs-io/nview`](https://github.com/blinklabs-io/nview) | Cardano node monitoring TUI |
| [`ouroboros-mock`](ouroboros-mock) | [`blinklabs-io/ouroboros-mock`](https://github.com/blinklabs-io/ouroboros-mock) | Go Ouroboros protocol mock library |
| [`plutigo`](plutigo) | [`blinklabs-io/plutigo`](https://github.com/blinklabs-io/plutigo) | Pure-Go Plutus interpreter |
| [`shai`](shai) | [`blinklabs-io/shai`](https://github.com/blinklabs-io/shai) | Cardano Multi-DEX matcher and oracle |
| [`terraform-modules`](terraform-modules) | [`blinklabs-io/terraform-modules`](https://github.com/blinklabs-io/terraform-modules) | Reusable cloud and infrastructure Terraform modules |
| [`tx-submit-api`](tx-submit-api) | [`blinklabs-io/tx-submit-api`](https://github.com/blinklabs-io/tx-submit-api) | Cardano transaction submission API |
| [`tx-submit-api-mirror`](tx-submit-api-mirror) | [`blinklabs-io/tx-submit-api-mirror`](https://github.com/blinklabs-io/tx-submit-api-mirror) | Cardano transaction submission API mirror |
| [`txtop`](txtop) | [`blinklabs-io/txtop`](https://github.com/blinklabs-io/txtop) | Cardano mempool inspector |

Keep this table synchronized with `.gitmodules` and add a purpose or category
when a new project repository is introduced.

## Actions and workspace workflows

[`actions`](actions) is shared infrastructure for the Blink Labs organization,
not a normal application dependency. It provides reusable `workflow_call`
workflows for testing, linting, publishing, version checks, and Conventional
Commit validation.

It also contains the governance engine and its `repos-config.yaml`. That file
is the operational source of truth for the repositories it manages: the sync
workflow reconciles repository settings, collaborators, branch protection, and
generated workflow wrappers, writing changes directly to each target's default
branch. The `actions` repository manages other repositories and is intentionally
not part of its own managed set.

The submodule here gives this workspace a reproducible checkout for inspecting
and developing the shared workflows and governance code. It does not pin the
workflow version consumed by downstream repositories: those workflow wrappers
currently reference paths such as
`blinklabs-io/actions/.github/workflows/reuseable-go-test.yml@main`. Changes to
the `actions` repository can therefore affect consumers independently of this
monorepo's submodule pointer.

The governance engine currently manages the 27 project repositories listed in
`repos/actions/repos-config.yaml`. The other ecosystem repositories in this
workspace are outside that set for now; some have their own CI while `docs` and
`kb` do not currently contain repository-local workflows. Define an appropriate
profile and checks before adding any of them to the governance set.

The scan findings and validation patterns are recorded in
[`docs/repository-patterns.md`](../docs/repository-patterns.md).

## Adding a repository

From the monorepo root, add a repository at a descriptive path:

```sh
git submodule add git@github.com:blinklabs-io/<repository>.git repos/<repository>
git submodule update --init --recursive
```

Before adding a repository, confirm its canonical URL and intended local path.
Read its `README.md`, `AGENTS.md`, `CONTRIBUTING.md`, and other repository-level
guidance before making changes inside it.

## Updating repositories

To initialize or restore all repositories at the revisions recorded by the
monorepo:

```sh
git submodule update --init --recursive
```

To update one repository intentionally, enter its directory, fetch or check out
the desired commit, then review and commit the changed submodule pointer in the
parent repository:

```sh
cd repos/<repository>
git fetch origin
git checkout <commit-or-ref>
cd ../..
git diff --submodule=log -- repos/<repository>
git add repos/<repository>
git commit -s -m "build: update <repository> submodule"
```

Use a Conventional Commit type that describes the reason for the update when a
more specific type is appropriate. Keep the parent pointer update separate from
source changes made in the nested repository.

## Working across repository boundaries

Changes to a project's source belong in that project's repository. The parent
monorepo should contain the pinned submodule reference and shared workspace
documentation or automation. Validate a project using its own documented
commands before updating its pointer here.

For organization-wide policies and community defaults, see
[`repos/.github/CONTRIBUTING.md`](.github/CONTRIBUTING.md) and
[`repos/.github/SECURITY.md`](.github/SECURITY.md).
