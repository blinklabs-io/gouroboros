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

package ledger_test

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

// bbmDecodeAligned is the reference's alignedValidFlags, restated from the
// BlockBody model (specs/23-ledger-bbody in gouroboros-tla-models).
func bbmDecodeAligned(prev, n int, xs []int) []bool {
	var ret []bool
	if len(xs) == 0 {
		for range max(n, 0) {
			ret = append(ret, true)
		}
		return ret
	}
	for range max(xs[0]-prev-1, 0) {
		ret = append(ret, true)
	}
	ret = append(ret, false)
	return append(ret, bbmDecodeAligned(xs[0], n-(xs[0]-prev), xs[1:])...)
}

// TestBlockBodyModelDecode runs the BlockBody model's decoding cells on a
// real Conway block: one witness set per body, every invalid index in
// range, and the validity flag each transaction then carries.
func TestBlockBodyModelDecode(t *testing.T) {
	headerCbor, err := hex.DecodeString(blockLimitsTestHeaderHex)
	require.NoError(t, err)
	var lists [][]int
	lists = append(lists, []int{})
	for a := 0; a <= 2; a++ {
		lists = append(lists, []int{a})
		for b := 0; b <= 2; b++ {
			lists = append(lists, []int{a, b})
		}
	}
	for n := 0; n <= 2; n++ {
		for _, witOff := range []int{-1, 0, 1} {
			if n+witOff < 0 {
				continue
			}
			for _, list := range lists {
				name := fmt.Sprintf("n=%d wits=%d invalid=%v", n, n+witOff, list)
				t.Run(name, func(t *testing.T) {
					bodies := make([]conway.ConwayTransactionBody, n)
					for i := range bodies {
						bodies[i].TxFee = uint64(i + 1) // #nosec G115
					}
					wits := make(
						[]conway.ConwayTransactionWitnessSet,
						n+witOff,
					)
					invalid := make([]uint, len(list))
					inRange := witOff == 0
					for i, idx := range list {
						invalid[i] = uint(idx) // #nosec G115
						if idx >= n {
							inRange = false
						}
					}
					blockCbor, err := cbor.Encode([]any{
						cbor.RawMessage(headerCbor),
						bodies,
						wits,
						common.TransactionMetadataSet{},
						invalid,
					})
					require.NoError(t, err)
					block, err := ledger.NewBlockFromCbor(
						ledger.BlockTypeConway,
						blockCbor,
						common.VerifyConfig{SkipBodyHashValidation: true},
					)
					if !inRange {
						require.Error(t, err)
						return
					}
					require.NoError(t, err)
					txs := block.Transactions()
					require.Len(t, txs, n)
					aligned := bbmDecodeAligned(-1, n, list)
					for i, tx := range txs {
						want := i >= len(aligned) || aligned[i]
						require.Equal(t, want, tx.IsValid(), "tx %d", i)
					}
				})
			}
		}
	}
}

// TestBlockBodyModelEnvelope runs the BlockBody model's envelope cells
// through VerifyBlock: the declared body size against the real one, the
// real size against the maximum (0 is read as no limit, a recorded drift
// from the reference), and the header's body hash.
func TestBlockBodyModelEnvelope(t *testing.T) {
	for _, txCount := range []int{0, 1} {
		bodies := make([]conway.ConwayTransactionBody, txCount)
		for i := range bodies {
			bodies[i].TxFee = uint64(i + 1) // #nosec G115
		}
		wits := make([]conway.ConwayTransactionWitnessSet, txCount)
		metadata := common.TransactionMetadataSet{}
		invalid := []uint{}
		var actual uint64
		var bodyHashes []byte
		for _, part := range []any{bodies, wits, metadata, invalid} {
			encoded, err := cbor.Encode(part)
			require.NoError(t, err)
			actual += uint64(len(encoded))
			bodyHashes = append(
				bodyHashes,
				common.Blake2b256Hash(encoded).Bytes()...,
			)
		}
		for _, sizeOff := range []int{-1, 0, 1} {
			for _, hashOk := range []bool{true, false} {
				for _, maxBody := range []uint{0, uint(actual) - 1, uint(actual)} {
					name := fmt.Sprintf(
						"txs=%d size%+d hash=%v max=%d",
						txCount, sizeOff, hashOk, maxBody,
					)
					t.Run(name, func(t *testing.T) {
						hashInput := bodyHashes
						if !hashOk {
							hashInput = append([]byte{0}, bodyHashes...)
						}
						declared := uint64(int64(actual) + int64(sizeOff)) // #nosec G115
						header := signedConwayLimitsHeader(
							t,
							declared,
							common.Blake2b256Hash(hashInput),
						)
						blockCbor, err := cbor.Encode(&conway.ConwayBlock{
							BlockHeader:            header,
							TransactionBodies:      bodies,
							TransactionWitnessSets: wits,
							TransactionMetadataSet: metadata,
							InvalidTransactions:    invalid,
						})
						require.NoError(t, err)
						block, err := ledger.NewBlockFromCbor(
							ledger.BlockTypeConway,
							blockCbor,
							common.VerifyConfig{SkipBodyHashValidation: true},
						)
						require.NoError(t, err)
						config := blockLimitsTestVerifyConfig(
							&conway.ConwayProtocolParameters{
								MaxBlockBodySize: maxBody,
							},
						)
						config.SkipBodyHashValidation = false
						valid, _, _, _, err := ledger.VerifyBlock(
							block,
							blockLimitsTestEta0Hex,
							blockLimitsTestSlotsPerKesPeriod,
							config,
						)
						want := sizeOff == 0 && hashOk &&
							(maxBody == 0 || actual <= uint64(maxBody))
						require.Equal(t, want, valid, "%v", err)
						require.Equal(t, want, err == nil, "%v", err)
					})
				}
			}
		}
	}
}
