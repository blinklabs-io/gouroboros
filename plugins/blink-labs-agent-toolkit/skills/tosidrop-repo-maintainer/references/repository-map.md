# TosiDrop repository map

This map covers the current organization shape. Confirm it with the target's
Git remote, `gh repo list TosiDrop`, local files, and live repository settings;
TosiDrop repositories are not currently `clanker` submodules.

## Dependency and deployment spine

```text
Cardano wallet / stake address
  -> TosiDrop/web browser UI
  -> Cloudflare Pages Functions
  -> vm-sdk client and VM API
  -> claims, rewards, pools, tokens, and history

TosiDrop/infrastructure
  -> Cloudflare Pages projects for web and vm-frontend
  -> production/preview variables and bindings
  -> D1, KV, R2, DNS, tunnels, and remote Terraform state
```

`web` and `vm-frontend` are separate applications with different default
branches, runtimes, API layers, and deployment projects. Do not copy a fix from
one into the other without confirming which application and domain owns the
observed behavior.

## Active repositories and validation

| Repository | Role and boundaries | Native validation |
| --- | --- | --- |
| `web` | React/Vite claim UI plus Cloudflare Pages Functions, D1 migrations, KV/R2-backed features, and VM API proxies. Node 22 is configured for its Pages deployment. | `npm ci`, `npm test`, `npm run lint`, `npm run build`. Run focused `functions/` or `workers/` Vitest files for backend changes. Preview deployments can lack or differ in VM/binding configuration; distinguish a preview 500 from a code regression. |
| `vm-sdk` | Published TypeScript client and types for the VM API. Public type changes affect both frontends and external npm consumers. CI uses Node 24. | `npm ci`, `npm test`, `npm run lint`, `npm run build`, and `npm pack --dry-run` for package-surface or release changes. Release tags must be exact `vX.Y.Z` matches for `package.json`; GitHub Actions publishes with OIDC. |
| `vm-frontend` | Older combined React client and Express/Cardano server on default branch `master`; still active and independently deployed. Client and server have separate lockfiles and scripts. | Run `npm ci`, build, and tests in both `client/` and `server/` using the CI Node matrix. The root and server `test` scripts are no-ops, so their zero exit does not prove coverage. Run the Docker build for Docker/runtime changes. |
| `infrastructure` | Private Terraform ownership of Cloudflare Pages, deployment variables and bindings, DNS, tunnels, load balancers, D1, KV, R2, and the R2 Terraform backend. | From `terraform/`: `terraform fmt -check -recursive`, `terraform init -input=false`, `terraform validate`, then `terraform plan -input=false` only with authorized credentials and remote-state access. Never apply or dispatch `run-terraform` without explicit approval. |
| `docs` | Docusaurus 2 beta documentation site with a Yarn lockfile and GitHub Pages workflows. | Use the lockfile's package manager, then run the documented install and `yarn build`; check changed links and rendered navigation. |
| `plsk` | Static HTML, token images, and Koios API specifications. | Validate the changed HTML or OpenAPI artifact directly; inspect image paths and public links. There is no shared package build to substitute. |
| `takedown-io` and other private active repositories | Project-specific behavior not documented by the public organization profile. | Read local instructions and workflows before selecting any command; do not infer a profile from the repository name or language label. |

Archived repositories include `airdrop-cardano`, `airdrop-ergo`,
`ergo-explorer-frontend`, `neta-staking-fe`, `token-drops`,
`token-registry-api`, and `www`. Do not modernize, unarchive, release, or migrate
them unless the request explicitly names that work.

## Contract checks

- `getRewardBreakdown` rows answer where rewards or promises came from. They are
  not an authoritative current-delegation endpoint. Use Cardano account or
  ledger state for current delegation and keep empty, stale, and error states
  distinct.
- Pool and whitelist payloads originate outside the browser. Validate map keys
  as well as embedded IDs, tolerate unknown records in read-only views, and do
  not turn a failed fetch into "not delegated" or "not whitelisted."
- `web` Pages Functions select VM base URL and network from deployment
  configuration. Cache keys, wallet network checks, and production/preview
  bindings must agree before a result is labeled mainnet or preview.
- `vm-sdk` has historically exposed weakly typed response members. Confirm the
  live or server-side payload and every consumer before narrowing a type or
  deleting defensive normalization.
- `TosiDrop/infrastructure/config.yaml`, environment files, Terraform state,
  and plans can contain sensitive deployment values. Report keys and affected
  resources, never values. Do not inspect or include tracked state in an
  unrelated diff.

## GitHub policy discovery

The main public application repositories currently favor squash merges and
protected branches with stale approvals dismissed, while less active projects
and private repositories differ. Treat that as orientation, not a permanent
rule: re-read the target branch's protection, enabled merge methods, required
checks, requested reviewers, and current-head approval immediately before a
review or merge decision.

TosiDrop does not publish organization-wide contribution or security rules in
`.github` today. That absence does not import Blink Labs policy. Preserve the
target's established commit style and follow explicit user direction; require
neither DCO sign-off nor screenshots unless the target or request does.
