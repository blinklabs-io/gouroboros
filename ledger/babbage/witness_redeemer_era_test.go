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

package babbage_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestBabbageWitnessSetRejectsObserveRedeemer(t *testing.T) {
	redeemers, err := cbor.Encode([]any{
		[]any{
			uint64(common.RedeemerTagObserve), uint64(0), uint64(0),
			[]uint64{0, 0},
		},
	})
	require.NoError(t, err)
	witnessSet, err := cbor.Encode(map[uint]any{
		5: cbor.RawMessage(redeemers),
	})
	require.NoError(t, err)

	var decoded babbage.BabbageTransactionWitnessSet
	err = decoded.UnmarshalCBOR(witnessSet)
	require.ErrorContains(t, err, "unsupported redeemer tag 6")
}
