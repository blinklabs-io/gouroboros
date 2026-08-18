# Blink repository families

Use this reference after reading the target repository's own instructions.
The lists reflect the repositories currently tracked by `clanker`; update this
reference when the inventory or shared workflow profiles change.

## Shared Docker profile

These repositories use the `docker-standard` profile in
`repos/actions/repos-config.yaml` and normally receive three generated wrappers:
Conventional Commit validation, native multi-architecture Docker CI, and
publishing.

- `docker-amaru`
- `docker-cardano-cli`
- `docker-cardano-configs`
- `docker-cardano-db-sync`
- `docker-cardano-node`
- `docker-cardano-wallet`
- `docker-go`
- `docker-haskell`
- `docker-hydra-node`
- `docker-kupo`
- `docker-minio`
- `docker-mithril-client`
- `docker-mithril-signer`
- `docker-ogmios`
- `docker-openvpn`
- `docker-parity-subkey`

For these projects, check Docker build arguments, upstream version/tag
selection, architecture support, and manifest publication behavior before
changing a Dockerfile or publish workflow.

## Go and service repositories

These projects generally combine Conventional Commit validation, Go tests,
golangci-lint, NilAway, Docker CI, and publishing. Some omit or customize a
workflow based on their scope.

- `adder`
- `cdnsd`, `dingo-operator`, `dingoctl`, `handshake-node`, and `vpn-indexer`
- `cardano-node-api`
- `cardano-up`
- `shai`
- `tx-submit-api`
- `tx-submit-api-mirror`
- `txtop`
- `docker-wireguard`

Check for `Makefile` targets and nested modules. Generated OpenAPI code and
Cardano protocol adapters are common sources of compile, lint, and NilAway
failures; validate generated packages independently when they have their own
module or test commands.

## Go protocol and library repositories

These repositories are reusable building blocks rather than deployable
services. Preserve their API and generated-code contracts while using their
native tests and specialized checks:

- `bark` — protobuf/ConnectRPC protocol definitions; use Buf formatting,
  generation, and linting alongside Go tests.
- `bluefin`, `bursa`, `dingo`, and `nview` — Cardano applications or services;
  inspect Docker, runtime configuration, and integration expectations.
- `cardano-models`, `go-bip39`, `go-scls`, `gouroboros`,
  `merkle-patricia-forestry`, `ouroboros-mock`, and `plutigo` — protocol,
  cryptographic, serialization, or interpreter libraries; preserve
  conformance, fuzz, benchmark, and nested-module checks.

`gouroboros` contains a nested submodule; use recursive submodule commands and
keep its nested repository boundary intact.

`dingo` has stricter node-level validation than a normal Go service. Read its
local `AGENTS.md` and `CLAUDE.md`, work in an isolated worktree, preserve
existing agent state, and use the Dingo maintainer skill for architecture
boundaries, EventBus usage, plugin composition, shared `ouroboros-mock`
fixtures, race-enabled tests, conformance/devnet testing, and the
`DATABASE.md`/`ARCHITECTURE.md` documentation bar.

## Special-purpose repositories

- `adder-mobile`: Flutter/mobile application with app-specific PR and release
  workflows.
- `cardano-compose-stacks`: Docker Compose integration environment; its
  workflow focuses on upstream version checks and it has local contribution and
  ownership files.
- `cardano-up-packages`: declarative package definitions consumed by
  `cardano-up`; version checks and package validation are its primary CI.
- `ansible-cardano`: Ansible Galaxy collection. Validate affected roles with
  `ansible-test` and `ansible-lint` before release changes.
- `helm-charts`: chart collection with chart-specific publishing workflows.
  Use `helm lint`, `helm template`, and chart-testing for affected charts.
- `terraform-modules`: reusable cloud modules. Run formatting and validation
  in each affected module and respect provider/version constraints.
- `issues`: content-only issue repository; review links and Markdown without
  inventing a code build.
- `docs-site`: active `blinklabs-io/docs` Astro/Starlight public documentation
  site. Prefer concise Markdown pages under `src/content/docs/`, preserve
  navigation metadata, and validate with `npm ci`, `npm run check`, and
  `npm run build` when the package manager is available. The archived
  `blinklabs-io/docs-site` repository is not the workspace docs checkout.
- `kb`: long-form engineering knowledge base organized into numbered books.
  Preserve its source-map, glossary, and pinned-public-source conventions when
  extending training material. It currently has no package manifest or CI
  workflow, so Markdown structure and link integrity are the practical checks.

## Historical patterns worth checking

Prior Codex work repeatedly encountered these patterns:

- Run `actionlint` after workflow changes, especially around Docker publish
  manifests and tag-triggered `latest` behavior.
- Use `docker build --check .` before an expensive image build. Haskell/Cardano
  images may take a long time to compile and can expose pre-existing undefined
  environment variables.
- Treat existing NilAway findings as a baseline only after confirming their
  location and ownership; do not silently broaden a cleanup task.
- For Go/OpenAPI changes, make sure generated code compiles and its nested
  module tests pass before interpreting root-module lint results.
- During reviews, distinguish merge blockers from non-blocking documentation or
  validation gaps, and report unrun live registry or integration checks.
