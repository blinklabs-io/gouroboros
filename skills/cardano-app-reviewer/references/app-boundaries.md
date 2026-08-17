# Cardano application boundaries

| Area | Primary pointers | Review questions |
| --- | --- | --- |
| Wallet/key | Bursa README, wallet/key packages, seed-file handling | Is key material isolated, zeroed or protected where appropriate, and never logged? |
| Transactions | Apollo upstream, Bursa transaction code, fixed backends | Are network, fee, validity, collateral, reference-input, datum, and signer rules correct? |
| Indexing | Adder, Shai/Bluefin indexers and cursor storage | Are rollback, duplicates, restart, and cursor recovery safe? |
| DEX | Shai/Bluefin profile and parser packages | Are on-chain identifiers and profile parameters validated per network? |
| Node/API | Dingo, Cardano Node API, Tx Submit API, Bark | Are protocol errors, timeouts, auth, and submission semantics preserved? |
| Shared tests | Ouroboros mock ledger, fixtures, conformance | Is the scenario deterministic and reusable instead of locally duplicated? |

Apollo is external upstream at `github.com/Salvionied/apollo/v2`; do not add a
Blink fork or local submodule for ordinary work. Emergency fork use requires
explicit approval, an issue, and an exit plan.
