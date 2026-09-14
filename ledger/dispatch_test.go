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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/stretchr/testify/require"
)

func TestShelleyTxValidationErrorDecodesNestedUtxoFailureByEra(t *testing.T) {
	tests := []struct {
		name string
		wire string
	}{
		{"Shelley", "81820181820082048103"},
		{"Allegra", "81820281820082048103"},
		{"Mary", "81820381820082048103"},
		{"Alonzo", "818204818200820082048103"},
		{"Babbage", "818205818200820282018103"},
		{"Conway", "81820681820182008104"},
		// Dijkstra: MEMPOOL LedgerFailure -> LEDGER UtxowFailure.
		{"Dijkstra", "818207818201820182008104"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire, err := hex.DecodeString(tt.wire)
			require.NoError(t, err)

			decoded, err := NewTxSubmitErrorFromCbor(wire)
			require.NoError(t, err)
			require.True(
				t, containsInputSetEmpty(decoded),
				"decoded error was %T: %v", decoded, decoded,
			)

			decoded, err = NewShelleyTxValidationErrorFromCbor(wire)
			require.NoError(t, err)
			require.True(t, containsInputSetEmpty(decoded))
		})
	}
}

func containsInputSetEmpty(err error) bool {
	switch err := err.(type) {
	case *InputSetEmptyUtxo:
		return true
	case *ShelleyTxValidationError:
		return containsInputSetEmpty(&err.Err)
	case *ApplyTxError:
		for _, failure := range err.Failures {
			if containsInputSetEmpty(failure) {
				return true
			}
		}
	case *UtxowFailure:
		return containsInputSetEmpty(err.Err)
	case *UtxoFailure:
		return containsInputSetEmpty(err.Err)
	case *ShelleyUtxowFailure:
		return containsInputSetEmpty(err.Err)
	case *AlonzoUtxowFailure:
		return containsInputSetEmpty(err.Err)
	case *BabbageUtxoFailure:
		return containsInputSetEmpty(err.Err)
	}
	return false
}

func TestNestedUtxoFailureMalformedUnknownAndDijkstra(t *testing.T) {
	for _, tt := range []struct {
		name string
		wire string
	}{
		{"Shelley singleton UTXOW", "8182018182008104"},
		{"Alonzo singleton UTXOW", "8182048182008100"},
		{"Babbage singleton Alonzo wrapper", "8182058182008101"},
		{"Babbage singleton UTXO wrapper", "8182058182008102"},
		{"Conway singleton UTXOW", "8182068182018100"},
		{"Dijkstra singleton UTXOW", "81820781820182018100"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wire, err := hex.DecodeString(tt.wire)
			require.NoError(t, err)
			require.NotPanics(t, func() {
				_, err = NewShelleyTxValidationErrorFromCbor(wire)
			})
			require.ErrorContains(t, err, "UtxowFailure")
		})
	}

	t.Run("Conway malformed payload", func(t *testing.T) {
		data, err := cbor.Encode([]any{ConwayUtxowUtxoFailure, "malformed"})
		require.NoError(t, err)
		decoded := UtxowFailure{era: EraIdConway}
		require.Error(t, decoded.UnmarshalCBOR(data))
	})

	t.Run("Conway unknown constructor", func(t *testing.T) {
		data, err := cbor.Encode(
			[]any{ConwayUtxowUtxoFailure, []any{uint(250)}},
		)
		require.NoError(t, err)
		decoded := UtxowFailure{era: EraIdConway}
		require.NoError(t, decoded.UnmarshalCBOR(data))
		utxo, ok := decoded.Err.(*UtxoFailure)
		require.True(t, ok)
		unknown, ok := utxo.Err.(*UnknownUtxoFailureError)
		require.True(t, ok)
		require.Equal(t, uint8(EraIdConway), unknown.Era)
		require.Equal(t, 250, unknown.FailureType)
		require.Equal(t, []byte{0x81, 0x18, 0xfa}, []byte(unknown.Cbor))
	})

	t.Run("Dijkstra input set empty", func(t *testing.T) {
		data, err := cbor.Encode([]any{ConwayUtxowUtxoFailure, []any{uint(4)}})
		require.NoError(t, err)
		decoded := UtxowFailure{era: EraIdDijkstra}
		require.NoError(t, decoded.UnmarshalCBOR(data))
		require.True(t, containsInputSetEmpty(decoded.Err))
	})

	t.Run("Babbage nested Alonzo", func(t *testing.T) {
		data, err := cbor.Encode(
			[]any{BabbageUtxoAlonzoInBabbage, []any{uint(3)}},
		)
		require.NoError(t, err)
		decoded := BabbageUtxoFailure{}
		require.NoError(t, decoded.UnmarshalCBOR(data))
		require.True(t, containsInputSetEmpty(decoded.Err))
	})

	t.Run("Babbage UTXOW nested Alonzo", func(t *testing.T) {
		data, err := cbor.Encode([]any{[]any{
			uint8(EraIdBabbage),
			[]any{[]any{
				uint(ApplyTxErrorUtxowFailure),
				[]any{
					uint(BabbageUtxowAlonzoInBabbage),
					[]any{
						uint(AlonzoUtxowShelleyInAlonzo),
						[]any{
							uint(ShelleyUtxowUtxoFailure),
							[]any{
								uint(BabbageUtxoAlonzoInBabbage),
								[]any{uint(3)},
							},
						},
					},
				},
			}},
		}})
		require.NoError(t, err)
		require.Equal(t,
			"81820581820082018200820482018103",
			hex.EncodeToString(data),
		)
		decoded, err := NewShelleyTxValidationErrorFromCbor(data)
		require.NoError(t, err)
		require.True(
			t,
			containsInputSetEmpty(decoded),
			"%T: %v",
			decoded,
			decoded,
		)
	})

	t.Run("direct Shelley UTXOW in Babbage", func(t *testing.T) {
		data, err := cbor.Encode([]any{
			uint(ShelleyUtxowUtxoFailure),
			[]any{uint(BabbageUtxoAlonzoInBabbage), []any{uint(3)}},
		})
		require.NoError(t, err)
		decoded := &ShelleyUtxowFailure{}
		require.NoError(t, decoded.unmarshalCBORWithEra(data, EraIdBabbage))
		require.IsType(t, &BabbageUtxoFailure{}, decoded.Err)
		require.True(t, containsInputSetEmpty(decoded.Err))
	})
}

func TestShelleyUtxowBabbagePayloadBoundaries(t *testing.T) {
	for _, payload := range []any{
		[]any{uint(BabbageUtxoAlonzoInBabbage)},
		[]any{uint(BabbageUtxoAlonzoInBabbage), "invalid"},
	} {
		wire, err := cbor.Encode([]any{ShelleyUtxowUtxoFailure, payload})
		require.NoError(t, err)
		decoded := &ShelleyUtxowFailure{}
		require.Error(t, decoded.unmarshalCBORWithEra(wire, EraIdBabbage))
	}
	inner := []byte{0x81, 0x18, 0xfa}
	wire, err := cbor.Encode([]any{
		ShelleyUtxowUtxoFailure,
		[]any{BabbageUtxoAlonzoInBabbage, cbor.RawMessage(inner)},
	})
	require.NoError(t, err)
	decoded := &ShelleyUtxowFailure{}
	require.NoError(t, decoded.unmarshalCBORWithEra(wire, EraIdBabbage))
	babbage, ok := decoded.Err.(*BabbageUtxoFailure)
	require.True(t, ok)
	utxo, ok := babbage.Err.(*UtxoFailure)
	require.True(t, ok)
	unknown, ok := utxo.Err.(*UnknownUtxoFailureError)
	require.True(t, ok)
	require.Equal(t, uint8(EraIdBabbage), unknown.Era)
	require.Equal(t, 250, unknown.FailureType)
	require.Equal(t, inner, []byte(unknown.Cbor))

	// The public standalone decoder keeps its Shelley-era default.
	shelleyWire, err := cbor.Encode([]any{
		ShelleyUtxowUtxoFailure, []any{uint(3)},
	})
	require.NoError(t, err)
	require.NoError(t, decoded.UnmarshalCBOR(shelleyWire))
	standalone, ok := decoded.Err.(*UtxoFailure)
	require.True(t, ok)
	require.Equal(t, uint8(EraIdShelley), standalone.Era)
	require.True(t, containsInputSetEmpty(standalone))
}

func TestNestedBabbageSpecificUtxoFailures(t *testing.T) {
	address := append([]byte{0x60}, make([]byte, 28)...)
	tests := []struct {
		name    string
		payload any
		check   func(*testing.T, error)
	}{
		{"collateral", []any{uint(2), int64(-100), uint64(500)},
			func(t *testing.T, err error) {
				v, ok := err.(*IncorrectTotalCollateralField)
				require.True(t, ok)
				require.Equal(t, uint8(2), v.Type)
				require.Equal(t, int64(-100), v.BalanceComputed)
				require.Equal(t, uint64(500), v.TotalCollateral)
			}},
		{"output", []any{uint(3), []any{[]any{
			[]any{address, uint64(1000000)}, uint64(2000000),
		}}}, func(t *testing.T, err error) {
			v, ok := err.(*BabbageOutputTooSmallUTxO)
			require.True(t, ok)
			require.Equal(t, uint8(3), v.Type)
			require.Len(t, v.Outputs, 1)
			require.Equal(t, uint64(2000000), v.Outputs[0].MinRequired)
		}},
		{"reference inputs", []any{uint(4), []any{
			[]any{make([]byte, 32), uint64(0)},
		}}, func(t *testing.T, err error) {
			v, ok := err.(*BabbageNonDisjointRefInputs)
			require.True(t, ok)
			require.Equal(t, uint8(4), v.Type)
			require.Len(t, v.Inputs, 1)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, inherited := range []bool{false, true} {
				var wrapper any = []any{BabbageUtxowUtxoFailure, tc.payload}
				if inherited {
					wrapper = []any{BabbageUtxowAlonzoInBabbage,
						[]any{AlonzoUtxowShelleyInAlonzo,
							[]any{ShelleyUtxowUtxoFailure, tc.payload}}}
				}
				wire, err := cbor.Encode(wrapper)
				require.NoError(t, err)
				decoded := UtxowFailure{era: EraIdBabbage}
				require.NoError(t, decoded.UnmarshalCBOR(wire))
				leaf := decoded.Err
				if inherited {
					alonzo, ok := leaf.(*AlonzoUtxowFailure)
					require.True(t, ok)
					shelley, ok := alonzo.Err.(*ShelleyUtxowFailure)
					require.True(t, ok)
					leaf = shelley.Err
				}
				babbage, ok := leaf.(*BabbageUtxoFailure)
				require.True(t, ok)
				tc.check(t, babbage.Err)
			}
		})
	}
}

func TestDijkstraMempoolFailureEnvelope(t *testing.T) {
	// cardano-ledger 2c33b4f858c0e62b300d121996a479f505d8c0e5:
	// Dijkstra/Rules/Mempool.hs and Dijkstra/Rules/Ledger.hs.
	for _, failure := range [][]any{
		{0, []any{0, []any{4}}}, // Unhandled Dijkstra MEMPOOL constructor.
		{1, []any{0, []any{4}}}, // Conway LEDGER is not Dijkstra MEMPOOL.
		{1, []any{2, []any{0}}}, // Unhandled Dijkstra LEDGER constructor.
		{2, "mempool rejected transaction"},
		{3}, // AllInputsAreSpent.
		{250, []any{1}},
	} {
		raw, err := cbor.Encode(failure)
		require.NoError(t, err)
		wire, err := cbor.Encode(
			[]any{[]any{EraIdDijkstra, []any{cbor.RawMessage(raw)}}},
		)
		require.NoError(t, err)
		decoded, err := NewShelleyTxValidationErrorFromCbor(wire)
		require.NoError(t, err)
		outer := decoded.(*ShelleyTxValidationError)
		require.Len(t, outer.Err.Failures, 1)
		unknown, ok := outer.Err.Failures[0].(*UnknownApplyTxFailureError)
		require.True(t, ok, "unexpected failure: %T", outer.Err.Failures[0])
		require.Equal(t, uint8(EraIdDijkstra), unknown.Era)
		require.Equal(t, failure[0], unknown.FailureType)
		require.Equal(t, raw, []byte(unknown.Cbor))
	}
	for _, failure := range [][]any{{1}, {1, []any{1}}} {
		wire, err := cbor.Encode([]any{[]any{EraIdDijkstra, []any{failure}}})
		require.NoError(t, err)
		_, err = NewShelleyTxValidationErrorFromCbor(wire)
		require.Error(t, err, "missing Dijkstra wrapper payload")
	}
}

func TestConwayLedgerDoesNotDecodeShelleyUtxowTag(t *testing.T) {
	// Conway/Rules/Ledger.hs at cardano-ledger
	// 2c33b4f858c0e62b300d121996a479f505d8c0e5 uses tag 1, not tag 0.
	raw := []byte{0x82, 0x00, 0x82, 0x00, 0x81, 0x04}
	wire, err := cbor.Encode([]any{[]any{EraIdConway, []any{cbor.RawMessage(raw)}}})
	require.NoError(t, err)
	decoded, err := NewShelleyTxValidationErrorFromCbor(wire)
	require.NoError(t, err)
	outer := decoded.(*ShelleyTxValidationError)
	require.Len(t, outer.Err.Failures, 1)
	unknown, ok := outer.Err.Failures[0].(*UnknownApplyTxFailureError)
	require.True(t, ok, "unexpected failure: %T", outer.Err.Failures[0])
	require.Equal(t, uint8(EraIdConway), unknown.Era)
	require.Equal(t, 0, unknown.FailureType)
	require.Equal(t, raw, []byte(unknown.Cbor))
}

func TestNestedUnknownUtxowFailureRetainsEnclosingEra(t *testing.T) {
	for _, tt := range []struct {
		name    string
		era     uint8
		payload []any
		unknown []byte
	}{
		{"Alonzo Shelley wrapper", EraIdAlonzo,
			[]any{0, []any{250}}, []byte{0x81, 0x18, 0xfa}},
		{"Babbage Alonzo wrapper", EraIdBabbage,
			[]any{1, []any{250, []any{}}}, []byte{0x82, 0x18, 0xfa, 0x80}},
		{"Babbage Shelley wrapper", EraIdBabbage,
			[]any{1, []any{0, []any{250}}}, []byte{0x81, 0x18, 0xfa}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			wire, err := cbor.Encode([]any{[]any{
				tt.era, []any{[]any{0, tt.payload}},
			}})
			require.NoError(t, err)
			decoded, err := NewShelleyTxValidationErrorFromCbor(wire)
			require.NoError(t, err)
			outer := decoded.(*ShelleyTxValidationError)
			require.Len(t, outer.Err.Failures, 1)
			failure := outer.Err.Failures[0].(*UtxowFailure).Err
			if wrapped, ok := failure.(*AlonzoUtxowFailure); ok {
				failure = wrapped.Err
			}
			if wrapped, ok := failure.(*ShelleyUtxowFailure); ok {
				failure = wrapped.Err
			}
			unknown, ok := failure.(*UnknownUtxowFailureError)
			require.True(t, ok, "unexpected nested failure: %T", failure)
			require.Equal(t, tt.era, unknown.Era)
			require.Equal(t, 250, unknown.FailureType)
			require.Equal(t, tt.unknown, []byte(unknown.Cbor))
		})
	}
}

func TestErrorDispatchersAcceptListLengthEncodings(t *testing.T) {
	t.Run("ApplyTxError", func(t *testing.T) {
		failure, err := cbor.Encode([]any{42})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(failure) {
			t.Run(encoding.Name, func(t *testing.T) {
				data, err := cbor.Encode([]cbor.RawMessage{encoding.Data})
				require.NoError(t, err)
				decoded := ApplyTxError{era: EraIdConway}
				require.NoError(t, decoded.UnmarshalCBOR(data))
				require.Len(t, decoded.Failures, 1)
				unknown, ok := decoded.Failures[0].(*UnknownApplyTxFailureError)
				require.True(t, ok)
				require.Equal(t, 42, unknown.FailureType)
			})
		}
	})

	t.Run("UtxowFailure", func(t *testing.T) {
		canonical, err := cbor.Encode([]any{ConwayUtxowInvalidMetadata})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
			t.Run(encoding.Name, func(t *testing.T) {
				decoded := UtxowFailure{era: EraIdConway}
				require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
				require.IsType(t, &InvalidMetadata{}, decoded.Err)
			})
		}
	})

	t.Run("UtxoFailure", func(t *testing.T) {
		inner, err := cbor.Encode([]any{42})
		require.NoError(t, err)
		for _, encoding := range test.CanonicalAndNonShortestList(inner) {
			t.Run(encoding.Name, func(t *testing.T) {
				data, err := cbor.Encode([]any{
					uint8(EraIdConway),
					cbor.RawMessage(encoding.Data),
				})
				require.NoError(t, err)
				var decoded UtxoFailure
				require.NoError(t, decoded.UnmarshalCBOR(data))
				unknown, ok := decoded.Err.(*UnknownUtxoFailureError)
				require.True(t, ok)
				require.Equal(t, 42, unknown.FailureType)
			})
		}
	})

	directCases := []struct {
		name      string
		canonical []byte
		decode    func(*testing.T, []byte) error
	}{
		{
			name: "ShelleyUtxowFailure",
			canonical: mustEncodeDispatchFixture(
				t,
				[]any{ShelleyUtxowInvalidMetadata},
			),
			decode: func(t *testing.T, data []byte) error {
				var decoded ShelleyUtxowFailure
				err := decoded.UnmarshalCBOR(data)
				if err == nil {
					require.IsType(t, &InvalidMetadata{}, decoded.Err)
				}
				return err
			},
		},
		{
			name:      "AlonzoUtxowFailure",
			canonical: mustEncodeDispatchFixture(t, []any{42, nil}),
			decode: func(t *testing.T, data []byte) error {
				var decoded AlonzoUtxowFailure
				err := decoded.UnmarshalCBOR(data)
				if err == nil {
					unknown, ok := decoded.Err.(*UnknownUtxowFailureError)
					require.True(t, ok)
					require.Equal(t, 42, unknown.FailureType)
				}
				return err
			},
		},
		{
			name:      "BabbageUtxoFailure",
			canonical: mustEncodeDispatchFixture(t, []any{42, nil}),
			decode: func(t *testing.T, data []byte) error {
				var decoded BabbageUtxoFailure
				err := decoded.UnmarshalCBOR(data)
				if err == nil {
					unknown, ok := decoded.Err.(*UnknownUtxoFailureError)
					require.True(t, ok)
					require.Equal(t, 42, unknown.FailureType)
				}
				return err
			},
		},
		{
			name: "ConwayUtxowFailure",
			canonical: mustEncodeDispatchFixture(
				t,
				[]any{ConwayUtxowInvalidMetadata},
			),
			decode: func(t *testing.T, data []byte) error {
				var decoded ConwayUtxowFailure
				err := decoded.UnmarshalCBOR(data)
				if err == nil {
					require.IsType(t, &InvalidMetadata{}, decoded.Err)
				}
				return err
			},
		},
	}
	for _, tc := range directCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, encoding := range test.CanonicalAndNonShortestList(tc.canonical) {
				t.Run(encoding.Name, func(t *testing.T) {
					require.NoError(t, tc.decode(t, encoding.Data))
				})
			}
		})
	}
}

func mustEncodeDispatchFixture(t *testing.T, value any) []byte {
	t.Helper()
	data, err := cbor.Encode(value)
	require.NoError(t, err)
	return data
}
