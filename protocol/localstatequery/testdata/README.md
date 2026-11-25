# Reference test data

`ouroboros-consensus/` holds golden files copied unchanged from
[IntersectMBO/ouroboros-consensus](https://github.com/IntersectMBO/ouroboros-consensus)
at revision `ec239d57f618d698b6350b48bf017b02c0c96a5f`:

- `ouroboros-consensus-cardano/golden/cardano/QueryVersion3/CardanoNodeToClientVersion19/`:
  the `Query_*`, `Result_*` and `TopLevelQuery_*` files.
- `ouroboros-consensus-cardano/golden/shelley/QueryVersion3/ShelleyNodeToClientVersion15/`:
  the `Query_*`, `Result_*` and `TopLevelQuery_*` files.

They are written by `Test.Util.Serialisation.Golden` from the examples in
`ouroboros-consensus-cardano/src/unstable-shelley-testlib/Test/Consensus/Shelley/Examples.hs`
and `src/unstable-cardano-testlib/Test/Consensus/Cardano/Examples.hs`.
At that revision node-to-client version 23 is the only supported version.
`reference_golden_test.go` uses them.
`ouroboros-consensus.sha256` lists each file's SHA-256 as computed from that revision's checkout.
