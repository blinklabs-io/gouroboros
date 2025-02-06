// Copyright 2025 Blink Labs Software
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

package shelley_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"

	"github.com/stretchr/testify/assert"
)

type testLedgerState struct {
	utxos []common.Utxo
}

func (ls testLedgerState) UtxosById(_ []common.TransactionInput) ([]common.Utxo, error) {
	return ls.utxos, nil
}

type testTipState struct {
	tip pcommon.Tip
}

func (ts testTipState) Tip() (pcommon.Tip, error) {
	return ts.tip, nil
}

func TestUtxoValidateTimeToLive(t *testing.T) {
	var testSlot uint64 = 555666777
	var testZeroSlot uint64 = 0
	testTx := &shelley.ShelleyTransaction{
		Body: shelley.ShelleyTransactionBody{
			Ttl: testSlot,
		},
	}
	testLedgerState := testLedgerState{}
	testProtocolParams := &shelley.ShelleyProtocolParameters{}
	var testBeforeSlot uint64 = 555666700
	var testAfterSlot uint64 = 555666799
	// Test helper function
	testRun := func(t *testing.T, name string, testTipSlot uint64, validateFunc func(*testing.T, error)) {
		t.Run(
			name,
			func(t *testing.T) {
				testTipState := testTipState{
					tip: pcommon.Tip{
						Point: pcommon.Point{
							Slot: testTipSlot,
						},
					},
				}
				err := shelley.UtxoValidateTimeToLive(
					testTx,
					testLedgerState,
					testTipState,
					testProtocolParams,
				)
				validateFunc(t, err)
			},
		)
	}
	// Slot before TTL
	testRun(
		t,
		"slot before TTL",
		testBeforeSlot,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateTimeToLive should succeed when provided a tip slot (%d) before the specified TTL (%d)\n  got error: %v",
					testBeforeSlot,
					testTx.TTL(),
					err,
				)
			}
		},
	)
	// Slot equal to TTL
	testRun(
		t,
		"slot equal to TTL",
		testSlot,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateTimeToLive should succeed when provided a tip slot (%d) equal to the specified TTL (%d)\n  got error: %v",
					testSlot,
					testTx.TTL(),
					err,
				)
			}
		},
	)
	// Slot after TTL
	testRun(
		t,
		"slot after TTL",
		testAfterSlot,
		func(t *testing.T, err error) {
			if err == nil {
				t.Errorf(
					"UtxoValidateTimeToLive should fail when provided a tip slot (%d) after the specified TTL (%d)",
					testAfterSlot,
					testTx.TTL(),
				)
				return
			}
			testErrType := shelley.ExpiredUtxoError{}
			assert.IsType(
				t,
				testErrType,
				err,
				"did not get expected error type: got %T, wanted %T",
				err,
				testErrType,
			)
		},
	)
	// Zero TTL
	testTx.Body.Ttl = testZeroSlot
	testRun(
		t,
		"zero TTL",
		testZeroSlot,
		func(t *testing.T, err error) {
			if err != nil {
				t.Errorf(
					"UtxoValidateTimeToLive should succeed when provided a zero TTL\n  got error: %v",
					err,
				)
			}
		},
	)
}
