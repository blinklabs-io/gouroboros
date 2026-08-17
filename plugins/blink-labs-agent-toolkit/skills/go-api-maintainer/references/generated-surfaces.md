# Generated API surfaces

| Surface | Source of truth | Typical owner/check |
| --- | --- | --- |
| OpenAPI | `openapi/api/openapi.yaml`, root `openapi.sh` or `make swagger` | Adder, Bursa, Cardano Node API, Tx Submit API; regenerate, test nested module |
| Protobuf/ConnectRPC | `proto/`, `buf.yaml`, `buf.gen.yaml` | Bark and Dingo; run Buf format/lint/generate and Go tests |
| SQLC | SQL queries/schema, `sqlc.yaml` | Dingo; run `make sql`, then `make sql-check` |
| Go interface | Handwritten interface and all implementations/callers | API/service package; compile and test every implementation |

Review generated diffs for:

- public names, JSON/CBOR tags, field optionality, and error contracts;
- backwards-compatible RPC paths, method numbers, wire types, and status
  behavior;
- accidental dependency or Go-version changes in nested modules;
- documentation and examples that describe the old surface;
- generated files changed without their source or generator changing.

Generated files are evidence of a source change, not a substitute for it.
