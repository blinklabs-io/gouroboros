# cardano-ledger golden files

Copied unchanged from IntersectMBO/cardano-ledger at revision
`1210d37fe6bdef32e85179ae08b7e1e8cfd75273`:

| File here | Source path |
| --- | --- |
| `<era>-tx.cbor` | `eras/<era>/impl/golden/tx.cbor` (shelley, allegra, mary, alonzo, babbage, conway) |
| `<era>-pparams.json` | `eras/<era>/impl/golden/pparams.json` (shelley, alonzo, babbage, conway) |

Each `tx.cbor` is the reference encoding of the era's example mempool
transaction at the era's lowest protocol version (`cborAnnGoldenSpec` in
`libs/cardano-ledger-core/testlib/Test/Cardano/Ledger/Era.hs`). The reference
ships no fee, size or minimum-coin values for these files; tests derive them
from the reference formulas.
