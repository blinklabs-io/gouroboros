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

package ledger

import (
	"encoding/hex"
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Golden CBOR bytes captured from
// IntersectMBO/ouroboros-consensus/ouroboros-consensus-cardano/golden/cardano/QueryVersion3/CardanoNodeToClientVersion19/ApplyTxErr_WrongEra*
// (exampleApplyTxErrWrongEraByron / Shelley in Test.Consensus.Cardano.Examples).
//
// Wire shape per encodeEitherMismatch + encodeNS in Haskell:
//
//	[2,                              -- listLen 2 (two NS entries)
//	 [2, otherEraIndex, otherName],  -- NS: era position + name (offered era)
//	 [2, ledgerEraIndex, ledgerName] -- NS: era position + name (ledger era)
//	]
//
// Era indices are CardanoEras positions: 0=Byron, 1=Shelley, 2=Allegra,
// 3=Mary, 4=Alonzo, 5=Babbage, 6=Conway. Era names match the strings the
// Haskell node emits via singleEraName ("Byron", "Shelley", etc.).
const (
	// goldenWrongEraByron: applying a Shelley thing to a Byron ledger.
	goldenWrongEraByron = "828201675368656c6c657982006542" +
		"79726f6e"
	// goldenWrongEraShelley: applying a Byron thing to a Shelley ledger.
	goldenWrongEraShelley = "8282006542" +
		"79726f6e82016753" +
		"68656c6c6579"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

func TestEraMismatch_DecodesByronWire(t *testing.T) {
	got := &EraMismatch{}
	_, err := cbor.Decode(mustHex(t, goldenWrongEraByron), got)
	require.NoError(t, err)
	assert.Equal(t, uint8(1), got.OtherEra.Index, "other era index = Shelley(1)")
	assert.Equal(t, "Shelley", got.OtherEra.Name)
	assert.Equal(t, uint8(0), got.LedgerEra.Index, "ledger era index = Byron(0)")
	assert.Equal(t, "Byron", got.LedgerEra.Name)
}

func TestEraMismatch_DecodesShelleyWire(t *testing.T) {
	got := &EraMismatch{}
	_, err := cbor.Decode(mustHex(t, goldenWrongEraShelley), got)
	require.NoError(t, err)
	assert.Equal(t, uint8(0), got.OtherEra.Index, "other era index = Byron(0)")
	assert.Equal(t, "Byron", got.OtherEra.Name)
	assert.Equal(t, uint8(1), got.LedgerEra.Index, "ledger era index = Shelley(1)")
	assert.Equal(t, "Shelley", got.LedgerEra.Name)
}

func TestEraMismatch_EncodesByronWire(t *testing.T) {
	em := &EraMismatch{
		OtherEra:  EraInfo{Index: 1, Name: "Shelley"},
		LedgerEra: EraInfo{Index: 0, Name: "Byron"},
	}
	got, err := cbor.Encode(em)
	require.NoError(t, err)
	assert.Equal(t, mustHex(t, goldenWrongEraByron), got,
		"encoded bytes must match the Haskell golden ApplyTxErr_WrongEraByron")
}

func TestEraMismatch_EncodesShelleyWire(t *testing.T) {
	em := &EraMismatch{
		OtherEra:  EraInfo{Index: 0, Name: "Byron"},
		LedgerEra: EraInfo{Index: 1, Name: "Shelley"},
	}
	got, err := cbor.Encode(em)
	require.NoError(t, err)
	assert.Equal(t, mustHex(t, goldenWrongEraShelley), got,
		"encoded bytes must match the Haskell golden ApplyTxErr_WrongEraShelley")
}

func TestEraMismatch_RoundTrip(t *testing.T) {
	original := &EraMismatch{
		OtherEra:  EraInfo{Index: 6, Name: "Conway"},
		LedgerEra: EraInfo{Index: 5, Name: "Babbage"},
	}
	encoded, err := cbor.Encode(original)
	require.NoError(t, err)

	decoded := &EraMismatch{}
	_, err = cbor.Decode(encoded, decoded)
	require.NoError(t, err)
	assert.Equal(t, original.OtherEra, decoded.OtherEra)
	assert.Equal(t, original.LedgerEra, decoded.LedgerEra)
}

// TestNewEraMismatchErrorFromCbor_RecognizesGoldenBytes verifies the
// constructor returns *EraMismatch for canonical wire bytes (i.e. the
// dispatch in NewTxSubmitErrorFromCbor will pick this branch instead of
// falling through to GenericError).
func TestNewEraMismatchErrorFromCbor_RecognizesGoldenBytes(t *testing.T) {
	for name, hexBytes := range map[string]string{
		"WrongEraByron":   goldenWrongEraByron,
		"WrongEraShelley": goldenWrongEraShelley,
	} {
		t.Run(name, func(t *testing.T) {
			got, err := NewEraMismatchErrorFromCbor(mustHex(t, hexBytes))
			require.NoError(t, err)
			require.NotNil(t, got)
			var em *EraMismatch
			require.True(t, errors.As(got, &em),
				"constructor must return *EraMismatch")
			assert.NotEmpty(t, em.OtherEra.Name)
			assert.NotEmpty(t, em.LedgerEra.Name)
		})
	}
}

// TestNewTxSubmitErrorFromCbor_PrefersEraMismatch verifies the typed
// dispatcher picks *EraMismatch (not GenericError) for canonical wire
// bytes. This is the regression that gouroboros has carried since the
// type was introduced — the prior decoder couldn't parse real
// cardano-node responses, so this dispatch would always have fallen
// through to the GenericError fallback.
func TestNewTxSubmitErrorFromCbor_PrefersEraMismatch(t *testing.T) {
	got, err := NewTxSubmitErrorFromCbor(mustHex(t, goldenWrongEraByron))
	require.NoError(t, err)
	var em *EraMismatch
	require.True(t, errors.As(got, &em),
		"NewTxSubmitErrorFromCbor must select *EraMismatch over GenericError fallback")
	assert.Equal(t, "Shelley", em.OtherEra.Name)
	assert.Equal(t, "Byron", em.LedgerEra.Name)
}

// TestEraMismatch_ErrorString verifies the Error() message names both
// eras (it's user-facing in tx-submission rejection logs).
func TestEraMismatch_ErrorString(t *testing.T) {
	em := &EraMismatch{
		OtherEra:  EraInfo{Index: 1, Name: "Shelley"},
		LedgerEra: EraInfo{Index: 0, Name: "Byron"},
	}
	msg := em.Error()
	assert.Contains(t, msg, "Shelley")
	assert.Contains(t, msg, "Byron")
}
