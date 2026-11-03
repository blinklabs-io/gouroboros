// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dijkstra

import (
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

func TestConwayFeatureTransactionUsesSelectedBodyLowerBound(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		topPresent bool
		subPresent bool
	}{
		{
			name:       "top present and sub absent",
			topPresent: true,
		},
		{
			name:       "top absent and sub present",
			subPresent: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			topBody := DijkstraTransactionBody{}
			topBody.SetValidityIntervalLowerBoundPresence(testCase.topPresent)
			subBody := DijkstraSubTransactionBody{}
			subBody.SetValidityIntervalLowerBoundPresence(testCase.subPresent)

			tx := dijkstraConwayFeatureTransaction{
				Transaction: DijkstraTransaction{Body: topBody},
				body:        &subBody,
			}
			lowerBound, present := common.TransactionValidityIntervalLowerBound(tx)
			require.Zero(t, lowerBound)
			require.Equal(t, testCase.subPresent, present)
		})
	}
}

func TestUtxoValidateNativeScriptsRequiresTimelockBounds(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		script      common.NativeScriptInvalidBefore
		hereafter   *common.NativeScriptInvalidHereafter
		setBound    func(*DijkstraTransactionBody)
		wantFailure bool
	}{
		{
			name:        "after zero without lower bound",
			script:      common.NativeScriptInvalidBefore{Type: 4, Slot: 0},
			wantFailure: true,
		},
		{
			name:   "after zero with explicit lower bound",
			script: common.NativeScriptInvalidBefore{Type: 4, Slot: 0},
			setBound: func(body *DijkstraTransactionBody) {
				body.SetValidityIntervalLowerBoundPresence(true)
			},
		},
		{
			name: "before maximum without upper bound",
			hereafter: &common.NativeScriptInvalidHereafter{
				Type: 5,
				Slot: math.MaxUint64,
			},
			wantFailure: true,
		},
		{
			name: "before maximum with explicit upper bound",
			hereafter: &common.NativeScriptInvalidHereafter{
				Type: 5,
				Slot: math.MaxUint64,
			},
			setBound: func(body *DijkstraTransactionBody) {
				body.SetValidityIntervalUpperBound(math.MaxUint64)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var encoded []byte
			var err error
			if testCase.hereafter != nil {
				encoded, err = cbor.Encode(testCase.hereafter)
			} else {
				encoded, err = cbor.Encode(&testCase.script)
			}
			require.NoError(t, err)
			var nativeScript common.NativeScript
			require.NoError(t, nativeScript.UnmarshalCBOR(encoded))

			// A guard credential makes the witnessed script required.
			tx := &DijkstraTransaction{
				Body: DijkstraTransactionBody{
					TxGuards: &DijkstraGuards{
						Credentials: []common.Credential{{
							CredType:   common.CredentialTypeScriptHash,
							Credential: nativeScript.Hash(),
						}},
					},
				},
				WitnessSet: DijkstraTransactionWitnessSet{
					WsNativeScripts: cbor.NewSetType(
						[]common.NativeScript{nativeScript},
						false,
					),
				},
				TxIsValid: true,
			}
			if testCase.setBound != nil {
				testCase.setBound(&tx.Body)
			}

			err = UtxoValidateNativeScripts(tx, 0, nil, nil)
			if !testCase.wantFailure {
				require.NoError(t, err)
				return
			}
			var scriptErr conway.NativeScriptFailedError
			require.ErrorAs(t, err, &scriptErr)
			require.Equal(t, nativeScript.Hash(), scriptErr.ScriptHash)
		})
	}
}
