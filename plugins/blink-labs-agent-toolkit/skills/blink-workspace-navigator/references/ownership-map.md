# Workspace ownership map

A topic-to-repository index for the `clanker` workspace. It answers "which
repository owns this?" — the per-family validation profile lives in
[repository-families.md](../../blink-repo-maintainer/references/repository-families.md).
Verify against the current checkout; the inventory changes.

## Dependency spine

```
gouroboros ──┬─> dingo ──> dingoctl, dingo-operator, docker images
             ├─> adder ──> adder-mobile, vpn-indexer, cdnsd
             ├─> cardano-node-api, tx-submit-api, tx-submit-api-mirror
             ├─> bursa, shai, bluefin, nview, txtop
             └─> cardano-models
ouroboros-mock ─> shared protocol fixtures for all of the above
plutigo ───────> Plutus evaluation
Salvionied/apollo (external, upstream-only) ─> transaction construction
repos/actions ─> generated workflows and repository settings for every project
```

`gouroboros` contains a nested submodule; use recursive submodule commands and
keep that boundary intact.

## By topic

| Topic | Owning repository |
|---|---|
| Ouroboros mini-protocols, CBOR, ledger eras, NtN/NtC | `gouroboros` |
| Shared protocol test fixtures and mocks | `ouroboros-mock` |
| Plutus / Untyped Plutus Core evaluation | `plutigo` |
| Cardano data model types | `cardano-models` |
| Merkle Patricia Forestry | `merkle-patricia-forestry` |
| BIP-39 mnemonics | `go-bip39` |
| Cardano node implementation | `dingo` |
| Dingo CLI and Kubernetes operator | `dingoctl`, `dingo-operator` |
| Chain event pipeline / indexer framework | `adder` |
| Adder mobile client (Flutter) | `adder-mobile` |
| Wallet and key management service | `bursa` |
| DEX / trading application | `shai` |
| Mining / block-production application | `bluefin` |
| Node HTTP+gRPC API | `cardano-node-api` |
| Transaction submission API and mirror | `tx-submit-api`, `tx-submit-api-mirror` |
| Node terminal dashboards | `nview`, `txtop` |
| protobuf / ConnectRPC contracts | `bark` |
| DNS-on-chain services and UI | `cdnsd`, `dns-cli`, `dns-frontend` |
| Handshake integration | `handshake-node` |
| VPN indexer and frontend | `vpn-indexer`, `vpn-frontend` |
| Cardano component installer | `cardano-up`, `cardano-up-packages` |
| Container images | `docker-*` |
| Compose integration environments, Antithesis stacks | `cardano-compose-stacks` |
| Reusable workflows, governance engine, repo settings | `actions` |
| Ansible collection for Cardano hosts | `ansible-cardano` |
| Helm charts | `helm-charts` (private app charts in `infrastructure/helmfile-app/charts/`) |
| Terraform modules (AWS, Cloudflare, …) | `terraform-modules` |
| Live deployment configuration, helmfile, Grafana, ops runbooks | `infrastructure` |
| Corporate website, partner pages, and customer marketing content | `www` |
| Public product documentation (Astro/Starlight) | `docs-site` (the active `blinklabs-io/docs`) |
| Long-form engineering knowledge base | `kb` |
| Organization defaults, CONTRIBUTING, SECURITY | `.github` |
| Cross-repository issue tracking | `issues` |
| Workshop and example code | `buidler-fest-2024-workshop`, `utxorpc-example` |

## Disambiguation traps

- `docker-cardano-node` builds an image of the Haskell `cardano-node`; `dingo`
  is the Go node. They are not alternatives within one repository.
- `docs-site` is the checkout of `blinklabs-io/docs`. The archived
  `blinklabs-io/docs-site` repository is not the workspace docs site.
- `helm-charts` holds public charts; private application charts live under
  `infrastructure/helmfile-app/charts/`.
- `actions` writes generated workflows into other repositories' default
  branches. A wrapper workflow in another repository is usually an output, not
  a source.
- `adder` is a library and a service. Check whether the change belongs to the
  pipeline framework, an input/output plugin, or the deployable binary.

## Where a change belongs

| Situation | Change belongs in |
|---|---|
| Protocol decoding wrong in an app | the owning library (`gouroboros`, `plutigo`) |
| Fixture needed by more than one repository | `ouroboros-mock` |
| Generated workflow wrapper is wrong | `actions` profile or reusable workflow |
| Generated Go client is stale | the OpenAPI/protobuf source, then regenerate |
| Image tag or manifest wrong | the `docker-*` repository, and its `actions` profile |
| Product install/config docs | `docs-site` |
| Durable technical explanation, architecture, testing lore | `kb` |
| Cross-repository tracking with no code change | `issues` |
| Submodule pointer only | `clanker` (this workspace) |
