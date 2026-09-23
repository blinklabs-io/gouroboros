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

package conway_test

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

// conwayBlockWithVote splices a voting_procedures entry naming
// gov_action_index idx into a real preview transaction and wraps the result
// in a block. Splicing a known good transaction keeps every other decode
// check satisfied, so a block that still decodes can only have accepted the
// governance action ID.
func conwayBlockWithVote(t *testing.T, idx uint64, isValid bool) []byte {
	t.Helper()

	voter, err := cbor.Encode(
		[]any{0, bytes.Repeat([]byte{0x33}, common.Blake2b224Size)},
	)
	require.NoError(t, err)
	govActionId, err := cbor.Encode(
		[]any{bytes.Repeat([]byte{0x44}, common.Blake2b256Size), idx},
	)
	require.NoError(t, err)
	votingProcedure, err := cbor.Encode([]any{0, nil})
	require.NoError(t, err)

	// Assembled by hand: a Go map cannot carry a CBOR array as its key.
	inner := append([]byte{0xa1}, govActionId...)
	inner = append(inner, votingProcedure...)
	votingProcedures := append([]byte{0xa1}, voter...)
	votingProcedures = append(votingProcedures, inner...)

	txCbor := previewPoolRegistrationTxCbor(t)
	var txComponents []cbor.RawMessage
	_, err = cbor.Decode(txCbor, &txComponents)
	require.NoError(t, err)
	require.Len(t, txComponents, 4)

	// Initialized rather than declared nil: the map is written to by
	// index below, and a decode that yielded a nil map would panic there.
	body := map[uint64]cbor.RawMessage{}
	_, err = cbor.Decode(txComponents[0], &body)
	require.NoError(t, err)
	body[19] = votingProcedures

	bodyCbor, err := cbor.Encode(body)
	require.NoError(t, err)
	splicedTx, err := cbor.Encode([]any{
		cbor.RawMessage(bodyCbor),
		cbor.RawMessage(txComponents[1]),
		isValid,
		cbor.RawMessage(txComponents[3]),
	})
	require.NoError(t, err)
	return syntheticConwayBlockWithTransaction(t, splicedTx)
}

// A block whose voting procedures name a governance action index above 65535
// must be refused at decode: the CDDL types the field as `uint .size 2` and
// cardano-ledger holds it in a Word16, so accepting it diverges from
// consensus.
//
// The isValid=false rows matter because a phase-2-invalid transaction skips
// several validation rules. Decode is not one of them.
func TestConwayBlockRejectsOversizedGovActionIdx(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		idx     uint64
		isValid bool
	}{
		{"65536, isValid true", math.MaxUint16 + 1, true},
		{"65536, isValid false", math.MaxUint16 + 1, false},
		{"max uint32, isValid true", math.MaxUint32, true},
		{"max uint32, isValid false", math.MaxUint32, false},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := conway.NewConwayBlockFromCbor(
				conwayBlockWithVote(t, testCase.idx, testCase.isValid),
				common.VerifyConfig{SkipBodyHashValidation: true},
			)
			require.Error(
				t,
				err,
				"block decoded with gov action index %d",
				testCase.idx,
			)
		})
	}
}

// The rejection above must not come from rejecting every spliced block. 256
// and 65535 are wire-legal even though CIP-0129's bech32 form cannot carry
// them.
func TestConwayBlockAcceptsUint16GovActionIdx(t *testing.T) {
	t.Parallel()

	for _, idx := range []uint64{0, 255, 256, math.MaxUint16} {
		t.Run(fmt.Sprintf("index %d", idx), func(t *testing.T) {
			t.Parallel()
			block, err := conway.NewConwayBlockFromCbor(
				conwayBlockWithVote(t, idx, true),
				common.VerifyConfig{SkipBodyHashValidation: true},
			)
			require.NoError(t, err)
			txs := block.Transactions()
			require.Len(t, txs, 1)
			procedures := txs[0].VotingProcedures()
			require.Len(t, procedures, 1)
			for _, actions := range procedures {
				require.Len(t, actions, 1)
				for actionId := range actions {
					require.Equal(
						t,
						uint32(idx),
						actionId.GovActionIdx,
					)
				}
			}
		})
	}
}

func TestConwayBlockDecoderRejectsRequiredAndEmptyBodyFields(t *testing.T) {
	for _, test := range []struct {
		name      string
		mutate    func(map[uint]cbor.RawMessage)
		wantError string
	}{
		{
			name: "missing required input field",
			mutate: func(fields map[uint]cbor.RawMessage) {
				delete(fields, 0)
			},
			wantError: "required CBOR map field 0 is missing",
		},
		{
			name: "empty certificates",
			mutate: func(fields map[uint]cbor.RawMessage) {
				empty, err := cbor.Encode([]any{})
				require.NoError(t, err)
				fields[4] = empty
			},
			wantError: "CBOR map field 4 must not be empty",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var components []cbor.RawMessage
			_, err := cbor.Decode(previewPoolRegistrationTxCbor(t), &components)
			require.NoError(t, err)
			var bodyFields map[uint]cbor.RawMessage
			_, err = cbor.Decode(components[0], &bodyFields)
			require.NoError(t, err)
			test.mutate(bodyFields)
			components[0], err = cbor.Encode(bodyFields)
			require.NoError(t, err)
			txCbor, err := cbor.Encode(components)
			require.NoError(t, err)
			blockCbor := syntheticConwayBlockWithTransaction(t, txCbor)
			_, err = conway.NewConwayBlockFromCbor(
				blockCbor,
				common.VerifyConfig{SkipBodyHashValidation: true},
			)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}
