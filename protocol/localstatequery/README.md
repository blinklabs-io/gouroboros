# LocalStateQuery Protocol

The LocalStateQuery protocol queries the local node's ledger state at specific chain points. It is used for wallet queries, stake information, protocol parameters, and other state queries.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `local-state-query` |
| Protocol ID | `7` |
| Mode | Node-to-Client |

## State Machine

```text
┌──────┐     Acquire/AcquireVolatileTip/     ┌───────────┐
│ Idle │     AcquireImmutableTip             │ Acquiring │
└──┬───┘ ─────────────────────────────────►  └─────┬─────┘
   │                                               │
   │ Done                             Failure      │ Acquired
   │                                    │          │
   ▼                                    ▼          ▼
┌──────┐                             ┌──────┐  ┌──────────┐
│ Done │◄────────────────────────────│ Idle │◄─│ Acquired │
└──────┘                             └──────┘  └────┬─────┘
                                                    │
                        ┌───────────────────────────┤
                        │                           │
           Query        │    Reacquire/             │ Release
                        │    ReacquireVolatileTip/  │
                        │    ReacquireImmutableTip  │
                        ▼                           │
                 ┌──────────┐                       │
                 │ Querying │                       │
                 └────┬─────┘                       │
                      │                             │
                      │ Result                      │
                      ▼                             ▼
                 ┌──────────┐                  ┌──────┐
                 │ Acquired │                  │ Idle │
                 └──────────┘                  └──────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | No state acquired |
| **Acquiring** | 2 | Server | Processing state acquisition |
| **Acquired** | 3 | Client | State acquired, ready for queries |
| **Querying** | 4 | Server | Processing query |
| **Done** | 5 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `Acquire` | 0 | Client → Server | Acquire state at specific point |
| `Acquired` | 1 | Server → Client | State successfully acquired |
| `Failure` | 2 | Server → Client | State acquisition failed |
| `Query` | 3 | Client → Server | Execute query on acquired state |
| `Result` | 4 | Server → Client | Query result |
| `Release` | 5 | Client → Server | Release acquired state |
| `Reacquire` | 6 | Client → Server | Reacquire at different point |
| `Done` | 7 | Client → Server | Terminate protocol |
| `AcquireVolatileTip` | 8 | Client → Server | Acquire state at volatile tip |
| `ReacquireVolatileTip` | 9 | Client → Server | Reacquire at volatile tip |
| `AcquireImmutableTip` | 10 | Client → Server | Acquire state at immutable tip |
| `ReacquireImmutableTip` | 11 | Client → Server | Reacquire at immutable tip |

## State Transitions

### From Idle (Client Agency)
| Message | New State |
|---------|-----------|
| `Acquire` | Acquiring |
| `AcquireVolatileTip` | Acquiring |
| `AcquireImmutableTip` | Acquiring |
| `Done` | Done |

### From Acquiring (Server Agency)
| Message | New State |
|---------|-----------|
| `Failure` | Idle |
| `Acquired` | Acquired |

### From Acquired (Client Agency)
| Message | New State |
|---------|-----------|
| `Query` | Querying |
| `Reacquire` | Acquiring |
| `ReacquireVolatileTip` | Acquiring |
| `ReacquireImmutableTip` | Acquiring |
| `Release` | Idle |

### From Querying (Server Agency)
| Message | New State |
|---------|-----------|
| `Result` | Acquired |

## Timeouts

| Timeout | Default | Description |
|---------|---------|-------------|
| Acquire Timeout | 5 seconds | Time to acquire state |
| Query Timeout | 180 seconds | Time to execute query |

## Acquire Targets

| Target | Description |
|--------|-------------|
| Specific Point | Acquire state at a specific slot/hash |
| Volatile Tip | Acquire state at the current volatile tip |
| Immutable Tip | Acquire state at the current immutable tip |

## Configuration Options

```go
localstatequery.NewConfig(
    localstatequery.WithAcquireFunc(acquireCallback),
    localstatequery.WithQueryFunc(queryCallback),
    localstatequery.WithReleaseFunc(releaseCallback),
    localstatequery.WithAcquireTimeout(5 * time.Second),
    localstatequery.WithQueryTimeout(180 * time.Second),
)
```

## Usage Example

```go
// Acquire state at tip
client.AcquireVolatileTip()

// Wait for Acquired message

// Query UTxOs for an address
result, err := client.Query(utxosByAddressQuery)

// Release state when done
client.Release()
```

## Common Queries

- **GetCurrentPParams**: Protocol parameters
- **GetStakeDistribution**: Stake pool distribution
- **GetUTxOByAddress**: UTxOs at specific addresses
- **GetUTxOByTxIn**: UTxOs by transaction inputs
- **GetEpochNo**: Current epoch number
- **GetGenesisConfig**: Genesis configuration
- **GetRewardInfoPools**: Pool reward information
- **GetRewardProvenance**: Reward calculation details
- **GetStakePools**: Registered stake pools
- **GetStakePoolParams**: Stake pool parameters

## Notes

- State must be acquired before queries can be executed
- Multiple queries can be executed on the same acquired state
- The acquired state is a snapshot; chain may advance during queries
- Use Reacquire to get a fresh state snapshot
- Release state when done to free server resources
