// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package localstatequery

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// eraHistoryModelEra is one era's parameters as the hard-fork combinator
// records them: slots per epoch and slot length in milliseconds.
type eraHistoryModelEra struct {
	epochSize    uint64
	slotLengthMs uint64
}

// eraHistoryModelBound is a Summary bound. Time is in picoseconds, the unit
// RelativeTime is encoded in.
type eraHistoryModelBound struct {
	time  *big.Int
	slot  uint64
	epoch uint64
}

const picosecondsPerMillisecond = 1_000_000_000

// eraHistoryModelUpper is the reference's mkUpperBound
// (HardFork/History/Summary.hs): an era ends a whole number of its own
// epochs after it starts.
func eraHistoryModelUpper(
	era eraHistoryModelEra,
	lo eraHistoryModelBound,
	hiEpoch uint64,
) eraHistoryModelBound {
	slots := (hiEpoch - lo.epoch) * era.epochSize
	elapsed := new(big.Int).Mul(
		new(big.Int).SetUint64(slots),
		new(big.Int).SetUint64(era.slotLengthMs*picosecondsPerMillisecond),
	)
	return eraHistoryModelBound{
		time:  new(big.Int).Add(lo.time, elapsed),
		slot:  lo.slot + slots,
		epoch: hiEpoch,
	}
}

// eraHistoryModelSummary is the reference's summarize for a ledger whose
// tip is tip (nil is Origin): known eras end at the next era's start epoch,
// and the last one at the epoch boundary at or after
// max(tip+1, start) + safeFromTip.
func eraHistoryModelSummary(
	eras []eraHistoryModelEra,
	starts []uint64,
	tip *uint64,
	safeFromTip uint64,
) (begins, ends []eraHistoryModelBound) {
	lo := eraHistoryModelBound{time: new(big.Int)}
	for i := range starts {
		var hi eraHistoryModelBound
		if i+1 < len(starts) {
			hi = eraHistoryModelUpper(eras[i], lo, starts[i+1])
		} else {
			from := lo.slot
			if tip != nil && *tip+1 > from {
				from = *tip + 1
			}
			n := from + safeFromTip - lo.slot
			epochs := n / eras[i].epochSize
			if n%eras[i].epochSize != 0 {
				epochs++
			}
			hi = eraHistoryModelUpper(eras[i], lo, lo.epoch+epochs)
		}
		begins = append(begins, lo)
		ends = append(ends, hi)
		lo = hi
	}
	return begins, ends
}

func eraHistoryModelBoundWire(b eraHistoryModelBound) []any {
	return []any{b.time, b.slot, b.epoch}
}

func eraHistoryTimespan(t *testing.T, value any) *big.Int {
	t.Helper()
	switch v := value.(type) {
	case uint64:
		return new(big.Int).SetUint64(v)
	case int64:
		return big.NewInt(v)
	case big.Int:
		return new(big.Int).Set(&v)
	case *big.Int:
		return new(big.Int).Set(v)
	default:
		t.Fatalf("unexpected era history timespan type %T", value)
		return nil
	}
}

// TestEraHistoryMatchesSummaryModel decodes GetEraHistory replies built by
// the specs/25-consensus-eras EraHistory model's Summary for several era
// tables, transition epochs and ledger tips. Each decoded bound must equal
// the model's, and the decoded summary must satisfy the reference's
// invariantSummary: bounds line up, no era is empty, INV-1b and INV-2b.
func TestEraHistoryMatchesSummaryModel(t *testing.T) {
	byron := eraHistoryModelEra{epochSize: 21600, slotLengthMs: 20000}
	shelley := eraHistoryModelEra{epochSize: 432000, slotLengthMs: 1000}
	small := []eraHistoryModelEra{
		{epochSize: 3, slotLengthMs: 3000},
		{epochSize: 4, slotLengthMs: 1000},
		{epochSize: 5, slotLengthMs: 200},
	}
	tip := func(slot uint64) *uint64 { return &slot }
	tests := []struct {
		name   string
		eras   []eraHistoryModelEra
		starts []uint64
		tip    *uint64
		safe   uint64
	}{
		{"origin, one era", small[:1], []uint64{0}, nil, 2},
		{"one era, tip mid epoch", small[:1], []uint64{0}, tip(4), 2},
		{"one era, tip at a boundary", small[:1], []uint64{0}, tip(5), 1},
		{"next era known, tip before it", small[:2], []uint64{0, 2}, tip(3), 2},
		{"next era known, tip in it", small[:2], []uint64{0, 2}, tip(9), 3},
		{"three eras", small, []uint64{0, 1, 3}, tip(14), 4},
		{"three eras, tip in the first", small, []uint64{0, 1, 3}, tip(0), 1},
		{
			"mainnet Byron to Shelley",
			[]eraHistoryModelEra{byron, shelley},
			[]uint64{0, 208},
			tip(4492800 + 1000),
			129600,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			begins, ends := eraHistoryModelSummary(
				test.eras,
				test.starts,
				test.tip,
				test.safe,
			)
			wire := make([]any, 0, len(begins))
			for i := range begins {
				wire = append(wire, []any{
					eraHistoryModelBoundWire(begins[i]),
					eraHistoryModelBoundWire(ends[i]),
					[]any{
						test.eras[i].epochSize,
						test.eras[i].slotLengthMs,
						[]any{uint8(0), test.safe, []any{uint8(0)}},
						uint64(36),
					},
				})
			}
			data, err := cbor.Encode(wire)
			require.NoError(t, err)

			var reply eraHistoryReply
			_, err = cbor.Decode(data, &reply)
			require.NoError(t, err)
			require.Len(t, reply, len(begins))

			for i, era := range reply {
				require.False(t, era.End.Unbounded, "era %d", i)
				require.Equal(t, begins[i].slot, uint64(era.Begin.SlotNo), "era %d start slot", i)
				require.Equal(t, begins[i].epoch, uint64(era.Begin.EpochNo), "era %d start epoch", i)
				require.Equal(t, 0, begins[i].time.Cmp(eraHistoryTimespan(t, era.Begin.Timespan)), "era %d start time", i)
				require.Equal(t, ends[i].slot, uint64(era.End.SlotNo), "era %d end slot", i)
				require.Equal(t, ends[i].epoch, uint64(era.End.EpochNo), "era %d end epoch", i)
				require.Equal(t, 0, ends[i].time.Cmp(eraHistoryTimespan(t, era.End.Timespan)), "era %d end time", i)
				require.Equal(t, test.eras[i].epochSize, uint64(era.Params.EpochLength), "era %d epoch size", i)
				require.Equal(t, test.eras[i].slotLengthMs, uint64(era.Params.SlotLength), "era %d slot length", i)

				if i > 0 {
					prev := reply[i-1].End
					require.Equal(t, prev.SlotNo, era.Begin.SlotNo, "era %d: bounds line up (slot)", i)
					require.Equal(t, prev.EpochNo, era.Begin.EpochNo, "era %d: bounds line up (epoch)", i)
					require.Equal(t, 0, eraHistoryTimespan(t, prev.Timespan).Cmp(eraHistoryTimespan(t, era.Begin.Timespan)), "era %d: bounds line up (time)", i)
				}
				require.Greater(t, era.End.EpochNo, era.Begin.EpochNo, "era %d is empty", i)
				epochs := uint64(era.End.EpochNo - era.Begin.EpochNo)
				slots := epochs * uint64(era.Params.EpochLength)
				require.Equal(t, uint64(era.Begin.SlotNo)+slots, uint64(era.End.SlotNo), "era %d: INV-1b", i)
				wantTime := new(big.Int).Add(
					eraHistoryTimespan(t, era.Begin.Timespan),
					new(big.Int).Mul(
						new(big.Int).SetUint64(slots),
						new(big.Int).SetUint64(uint64(era.Params.SlotLength)*picosecondsPerMillisecond),
					),
				)
				require.Equal(t, 0, wantTime.Cmp(eraHistoryTimespan(t, era.End.Timespan)), "era %d: INV-2b", i)
			}
		})
	}
}

// TestEraHistorySummaryModelMainnetShelleyStart pins the model's arithmetic
// to mainnet: Shelley began at epoch 208, slot 4492800, 89856000 seconds
// after the system start (208 Byron epochs of 21600 slots of 20 seconds).
func TestEraHistorySummaryModelMainnetShelleyStart(t *testing.T) {
	begins, _ := eraHistoryModelSummary(
		[]eraHistoryModelEra{
			{epochSize: 21600, slotLengthMs: 20000},
			{epochSize: 432000, slotLengthMs: 1000},
		},
		[]uint64{0, 208},
		nil,
		0,
	)
	require.Len(t, begins, 2)
	require.Equal(t, uint64(4492800), begins[1].slot)
	require.Equal(t, uint64(208), begins[1].epoch)
	want, ok := new(big.Int).SetString(fmt.Sprintf("%d000000000000", 89856000), 10)
	require.True(t, ok)
	require.Equal(t, 0, want.Cmp(begins[1].time))
}
