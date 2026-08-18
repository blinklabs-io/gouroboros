---
name: go-api-maintainer
description: Maintain and review Blink Labs Go API surfaces, including OpenAPI-generated clients, protobuf/Buf and ConnectRPC contracts, sqlc output, nested modules, and public compatibility. Use when changing Adder, Bursa, Cardano Node API, Tx Submit API, Bark, Dingo APIs, or generated Go code.
---

# Go API Maintainer

Use this skill whenever a change crosses a public Go, HTTP, protobuf, RPC, or
database-generated interface. Read the [generated surfaces reference](references/generated-surfaces.md)
after reading local repository instructions.

## Workflow

1. Find every affected `go.mod`; root tests do not cover nested OpenAPI, UI,
   examples, or Antithesis modules automatically.
2. Identify the source of truth: OpenAPI YAML/spec, `.proto` files and Buf
   config, SQL/schema/query files, or handwritten interface definitions.
3. Inspect callers, implementations, generated artifacts, docs, and workflow
   generation commands before editing. Do not hand-edit generated output unless
   the repository explicitly requires it.
4. Regenerate only when the source changed, review the complete generated diff,
   and check for accidental dependency or public-name changes.
5. Run the repository-native generation/check target, root tests, nested-module
   tests, race tests where configured, and compatibility-focused tests.
6. For API removals or wire changes, document migration impact and verify
   clients, examples, fixtures, and downstream users in the workspace.
7. Read the consumer before changing a response shape. An HTTP surface in this
   workspace usually ends at a TypeScript client helper and then a screen: a
   field the shared helper does not parse is unreachable no matter what the
   handler sends, and a client that hardcodes a value the server also asserts
   will mask the server's answer. See
   [cross-boundary-changes](../cross-boundary-changes/SKILL.md).

Keep generated output, API documentation, and runtime behavior synchronized.
Treat stale generated files as a correctness issue, not a formatting detail.
