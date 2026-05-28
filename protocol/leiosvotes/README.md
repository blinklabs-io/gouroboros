# LeiosVotes Protocol

The LeiosVotes protocol diffuses votes on Leios endorser blocks. It is part
of the experimental CIP-0164 Linear Leios protocol suite and follows the
current upstream design where vote diffusion is separate from EB announce and
fetch traffic.

## Protocol Identifiers

| Field | Value |
|-------|-------|
| Protocol Name | `leios-votes` |
| Protocol ID | `20` |
| Mode | Node-to-Node |

## Messages

| Message | ID | Direction | Purpose |
|---------|----|-----------|---------|
| `VotesRequestNext` | 0 | Client -> Server | Request `N` votes |
| `Vote` | 1 | Server -> Client | Deliver one vote |
| `Done` | 2 | Client -> Server | End protocol |

`VotesRequestNext` carries a positive `Count`. The implementation currently
caps this at `MaxRequestNextCount` to keep the receive buffer commitment
bounded while CIP-0164 remains experimental.

`Vote` carries the stake-based committee vote shape from the current
CIP-0164 design:

| Field | Type | Purpose |
|-------|------|---------|
| `SlotNo` | `uint64` | Voting round / announcing RB slot |
| `EndorserBlockHash` | `common.Blake2b256` | EB hash being endorsed |
| `VoterId` | `uint64` | Index in the epoch committee |
| `VoteSignature` | `[]byte` | 48-byte BLS12-381 MinSig signature |

## State Machine

```text
Idle --VotesRequestNext(N)--> Busy(tokens=N)
Busy --Vote, tokens > 1--> Busy(tokens-1)
Busy --Vote, tokens = 1--> Idle
Idle --Done--> Done
```

## States

| State | ID | Agency | Description |
|-------|----|--------|-------------|
| **Idle** | 1 | Client | Waiting for a vote request or termination |
| **Busy** | 2 | Server | Delivering the requested vote batch |
| **Done** | 3 | None | Terminal state |

## Usage

```go
cfg := leiosvotes.NewConfig(
    leiosvotes.WithRequestNextFunc(func(
        ctx leiosvotes.CallbackContext,
        count uint64,
    ) ([]leiosvotes.Vote, error) {
        return nextVotes(count)
    }),
    leiosvotes.WithVoteFunc(func(
        ctx leiosvotes.CallbackContext,
        vote leiosvotes.Vote,
    ) error {
        return handleVote(vote)
    }),
)
```
