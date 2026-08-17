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

## Special-purpose repositories

- `adder-mobile`: Flutter/mobile application with app-specific PR and release
  workflows.
- `cardano-compose-stacks`: Docker Compose integration environment; its
  workflow focuses on upstream version checks and it has local contribution and
  ownership files.
- `cardano-up-packages`: declarative package definitions consumed by
  `cardano-up`; version checks and package validation are its primary CI.
- `docs`: Next.js/Nextra public documentation site. Prefer concise MDX pages,
  preserve navigation metadata, and validate the package manager/build scripts
  before changing the site.
- `kb`: long-form engineering knowledge base organized into numbered books.
  Preserve its source-map, glossary, and pinned-public-source conventions when
  extending training material.

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
