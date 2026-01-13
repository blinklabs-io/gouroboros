# Conformance Tests

This package runs Conway ledger conformance tests against test vectors from the Cardano ledger specification.

**Status**: 314/314 tests passing (100%)

## Test Vectors

**Source**: `rules-conformance.tar.gz` in this directory

Sourced from [Amaru](https://github.com/pragma-org/amaru) commit `930c14b6bdf8197bc7d9397d872949e108b28eb4`
(original path: `crates/amaru-ledger/tests/data/rules-conformance.tar.gz`)

The tarball is extracted to a temp directory at test runtime. It contains:
- 320 test vector files (CBOR binary) in `eras/conway/impl/dump/Conway/`
- 44 protocol parameter files in `pparams-by-hash/`

## Running Tests

```bash
go test -v ./internal/test/conformance/...
```

## Test Vector CBOR Structure

### Top-Level Array
```
[0] config:        array[13]  - Network/protocol configuration
[1] initial_state: array[7]   - NewEpochState before events
[2] final_state:   array[7]   - NewEpochState after events
[3] events:        array[N]   - Transaction/epoch events
[4] title:         string     - Test name/path
```

### Config Array (index 0)
The config array contains simplified network parameters, not full protocol parameters:
```
[0]  start_slot:     uint64   - Epoch start slot
[1]  slot_length:    uint64   - Slot duration (milliseconds)
[2]  epoch_length:   uint64   - Slots per epoch
[3]  security_param: uint64   - Security parameter (k)
[4]  active_slots:   uint64   - Active slots coefficient denominator
[5]  network_id:     uint64   - Network ID (0=testnet, 1=mainnet)
[6]  pool_stake:     uint64   - Pool stake (scaled)
[7]  unknown_7:      uint64   - Unknown
[8]  unknown_8:      uint64   - Unknown
[9]  max_lovelace:   uint64   - Maximum lovelace (for rational encoding)
[10] rational:       tag(30)  - Rational number [numerator, denominator]
[11] unknown_11:     uint64   - Unknown
[12] ex_units:       array    - [mem, steps, price] for script execution
```

Note: Full protocol parameters including cost models are extracted from the
initial_state via pparams hash lookup, not from this config array.

### NewEpochState Structure
```
[0] epoch_no
[3] begin_epoch_state: array[2]
    [0] account_state: [treasury, reserves]
    [1] ledger_state: array[2]
        [0] cert_state: array[5]
            [0] voting_state (dreps, committee)
        [1] utxo_state: array[4]
            [0] utxos: map[TxIn]TxOut
            [1] deposits
            [2] fees
            [3] gov_state: array[7]
                [0] proposals
                [1] committee
                [2] constitution
                [3] current_pparams_hash
```

### Event Types
Events are CBOR arrays where the first element is the variant tag:
```
Transaction: [0, tx_cbor:bytes, success:bool, slot:uint64]
PassTick:    [1, slot:uint64]
PassEpoch:   [2, epoch:uint64]
```

The `success` field in Transaction events indicates:
- `true` = Transaction should be accepted (even if IsValid=false for phase-2 failures)
- `false` = Transaction should be rejected by phase-1 validation

Note: A transaction with `IsValid=false` may still have `success=true` if it was
correctly identified as a phase-2 failure. The transaction will be included in
the block but its effects (other than collateral consumption) will be reverted.

## Key Paths

| Data | CBOR Path |
|------|-----------|
| UTxOs | `initial_state[3][1][1][0]` |
| Gov State | `initial_state[3][1][1][3]` |
| Proposals | `initial_state[3][1][1][3][0]` |
| Committee | `initial_state[3][1][1][3][1]` |
| Constitution | `initial_state[3][1][1][3][2]` |
| DReps | `initial_state[3][1][0][0][0]` |
| Reward Balances | `initial_state[3][1][0][2][0][0]` |

## Multi-Transaction Handling

Many test vectors contain multiple transactions that build on each other:
1. TX 0 creates initial UTxOs
2. TX 1+ may spend outputs from prior TXs

The test harness updates the UTxO set after each transaction using:
- `tx.Consumed()` - UTxOs removed
- `tx.Produced()` - UTxOs created

## UTxO Encoding Formats

The harness handles multiple UTxO encodings:
1. `map[UtxoId]Output` - Typed keys
2. `map[string]Output` - String keys ("txid#index")
3. `map[rawBytes]Output` - Byte-string keys
4. `[[UtxoId, Output], ...]` - Array of pairs

## Governance State Structure

### Gov State Array
The gov_state at `initial_state[3][1][1][3]` contains 7 elements:
```
[0] proposals           - Proposal tracking
[1] committee           - Constitutional committee
[2] constitution        - Current constitution anchor and policy
[3] current_pparams_hash - Hash of current protocol parameters (32 bytes)
[4] prev_pparams_hash   - Hash of previous epoch's protocol parameters (32 bytes)
[5] future_pparams      - Future protocol parameters (if any)
[6] drep_state          - DRep-related state
```

Note: The current_pparams_hash at [3] is used to look up protocol parameters
from the `pparams-by-hash/` directory.

### Proposals Array
```
proposals = [
    [0] proposals_tree,     - Map of GovActionId -> ProposalState
    [1] root_params,        - Last enacted ParameterChange (or null)
    [2] root_hard_fork,     - Last enacted HardFork (or null)
    [3] root_cc,            - Last enacted NoConfidence/UpdateCommittee (or null)
    [4] root_constitution   - Last enacted NewConstitution (or null)
]
```

### ProposalState CBOR
```
ProposalState = [
    [0] id,                 - GovActionId [txHash, index]
    [1] committee_votes,    - map[StakeCredential]Vote
    [2] dreps_votes,        - map[StakeCredential]Vote
    [3] pools_votes,        - map[PoolId]Vote
    [4] procedure,          - Proposal (contains action type)
    [5] proposed_in,        - Epoch
    [6] expires_after       - Epoch
]
```

### Vote Values
- `0` = Yes
- `1` = No
- `2` = Abstain

### Ratification at Epoch Boundaries

1. At each `PassEpoch` event, proposals are evaluated for ratification
2. A proposal is ratified if:
   - Its parent matches the current root for its governance purpose
   - It has sufficient votes (during bootstrap, thresholds are 0 for DReps)
3. Enacted proposals update the corresponding root
4. New proposals must reference the current root as their parent

### Parent Chain Validation

After a proposal is enacted, subsequent proposals of the same purpose must reference it:
- NewConstitution with empty PrevGovId fails if a constitution was already enacted
- ParameterChange must chain from the last enacted ParameterChange

## Implementation Notes

### ScriptDataHash Validation

The ScriptDataHash is computed as `Blake2b256(redeemers || datums || language_views)`:
- Redeemers: Original CBOR bytes preserved via `ConwayRedeemers.Cbor()`
- Datums: Original CBOR bytes preserved via `SetType[Datum].Cbor()` (only if non-empty)
- Language views: Encoded per Cardano spec with version-specific formats

**PlutusV1** (double-bagged for historical compatibility):
- Tag: `serialize(serialize(0))` = `0x4100` (bytestring containing 0x00)
- Params: indefinite-length list of cost model values, wrapped in bytestring

**PlutusV2/V3**:
- Tag: `serialize(version)` = `0x01` or `0x02`
- Params: definite-length list of cost model values (no bytestring wrapper)

The language views map uses "shortLex" ordering (length first, then lexicographic).

### Cost Model Handling

The Haskell test suite modifies protocol parameters in memory via `modifyPParams`,
but test vectors store the original (unmodified) pparams hash. For "No cost model"
tests, the harness clears cost models to simulate the Haskell behavior.

### Malformed Reference Scripts

Transaction outputs with reference scripts must contain well-formed Plutus bytecode.
The validation uses plutigo's `syn.Decode[syn.DeBruijn]` to verify scripts are valid UPLC.

### Witness Set CBOR Keys

Conway-era witness set uses these CBOR map keys:
- 0: VKey witnesses
- 1: Native scripts
- 2: Bootstrap witnesses
- 3: PlutusV1 scripts
- 4: Plutus data (datums)
- 5: Redeemers
- 6: PlutusV2 scripts
- 7: PlutusV3 scripts

## Common Pitfalls

1. **Reference inputs**: Resolved but never consumed
2. **Collateral**: Only consumed when IsValid=false
3. **Datum lookup**: Check witness set, inline datum, and reference inputs
4. **Cost models**: Must exist for each Plutus version used
5. **Network ID**: All vectors use Preview/Testnet (network ID 0)
6. **Proposal enactment**: Happens at epoch boundaries, not immediately
7. **Vote tracking**: Votes are stored within ProposalState, not separately
8. **Ratification timing**: Proposals ratified in epoch N are enacted in epoch N+1

## Test Categories

| Directory | Tests | Focus |
|-----------|-------|-------|
| GOV | 55 | Proposals, voting, policies |
| GOVCERT | 9 | DRep/CC certificates |
| ENACT | 16 | Proposal enactment |
| DELEG | 24 | Delegation operations |
| EPOCH | 12 | Epoch boundary logic |
| RATIFY | 46 | Ratification thresholds |
| AlonzoImpSpec | ~50 | Plutus V1/V2/V3 scripts |
| BabbageImpSpec | ~20 | Reference scripts, inline datums |
| ShelleyImpSpec | ~30 | Basic TX, witnesses, metadata |

## Key Implementation Files

| File | Purpose |
|------|---------|
| `ledger/common/script.go` | Native script evaluation, Plutus script types |
| `ledger/common/script/context.go` | Script context construction |
| `ledger/common/errors.go` | Error types (ScriptDataHashMismatch, MalformedReferenceScripts, etc.) |
| `ledger/conway/rules.go` | Conway validation rules including ScriptDataHash and malformed script validation |
| `internal/test/conformance/conformance_test.go` | Test harness, state tracking |
| `internal/test/ledger/ledger.go` | MockLedgerState implementation |
