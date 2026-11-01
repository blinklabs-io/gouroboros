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

package common_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestNativeScriptValidityBoundPresence(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		script      string
		body        string
		wantInvalid bool
	}{
		{
			name:        "after zero without lower bound",
			script:      "820400",
			body:        "a3008001800200",
			wantInvalid: true,
		},
		{
			name:   "after zero with explicit lower bound",
			script: "820400",
			body:   "a40080018002000800",
		},
		{
			name:        "before maximum without upper bound",
			script:      "82051bffffffffffffffff",
			body:        "a3008001800200",
			wantInvalid: true,
		},
		{
			name:   "before maximum with explicit upper bound",
			script: "82051bffffffffffffffff",
			body:   "a4008001800200031bffffffffffffffff",
		},
		{
			name:        "after one without lower bound",
			script:      "820401",
			body:        "a3008001800200",
			wantInvalid: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			var tx allegra.AllegraTransaction
			body := mustDecodeHex(t, testCase.body)
			_, err := cbor.Decode(body, &tx.Body)
			require.NoError(t, err)

			var nativeScript common.NativeScript
			_, err = cbor.Decode(
				mustDecodeHex(t, testCase.script),
				&nativeScript,
			)
			require.NoError(t, err)

			_, invalid := common.FirstInvalidNativeScriptIn(
				tx,
				0,
				[]common.NativeScript{nativeScript},
			)
			require.Equal(t, testCase.wantInvalid, invalid)
		})
	}
}

func TestConstructedExplicitZeroLowerBoundPresence(t *testing.T) {
	t.Parallel()

	body := allegra.AllegraTransactionBody{}
	body.SetValidityIntervalLowerBoundPresence(true)
	tx := allegra.AllegraTransaction{Body: body}

	var nativeScript common.NativeScript
	_, err := cbor.Decode(mustDecodeHex(t, "820400"), &nativeScript)
	require.NoError(t, err)

	_, invalid := common.FirstInvalidNativeScriptIn(
		tx,
		0,
		[]common.NativeScript{nativeScript},
	)
	require.False(t, invalid)
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)
	return decoded
}
