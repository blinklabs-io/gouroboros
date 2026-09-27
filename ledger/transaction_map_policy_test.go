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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

type ledgerMapEntry struct {
	key   any
	value any
}

type ledgerMapUnmarshaler interface {
	UnmarshalCBOR([]byte) error
}

func encodeLedgerMap(t *testing.T, entries ...ledgerMapEntry) []byte {
	t.Helper()
	require.LessOrEqual(t, len(entries), 23)
	encoded := []byte{cbor.CborTypeMap + uint8(len(entries))}
	for _, entry := range entries {
		key, err := cbor.Encode(entry.key)
		require.NoError(t, err)
		value, err := cbor.Encode(entry.value)
		require.NoError(t, err)
		encoded = append(encoded, key...)
		encoded = append(encoded, value...)
	}
	return encoded
}

func TestTransactionBodiesRejectDuplicateAndUnknownMapFields(t *testing.T) {
	tests := []struct {
		name     string
		newBody  func() ledgerMapUnmarshaler
		required []ledgerMapEntry
	}{
		{
			name: "Shelley",
			newBody: func() ledgerMapUnmarshaler {
				return &shelley.ShelleyTransactionBody{}
			},
			required: []ledgerMapEntry{
				{key: uint64(0), value: []any{}},
				{key: uint64(1), value: []any{}},
				{key: uint64(2), value: uint64(0)},
				{key: uint64(3), value: uint64(0)},
			},
		},
		{
			name: "Allegra",
			newBody: func() ledgerMapUnmarshaler {
				return &allegra.AllegraTransactionBody{}
			},
			required: requiredBodyFields(),
		},
		{
			name: "Mary",
			newBody: func() ledgerMapUnmarshaler {
				return &mary.MaryTransactionBody{}
			},
			required: requiredBodyFields(),
		},
		{
			name: "Alonzo",
			newBody: func() ledgerMapUnmarshaler {
				return &alonzo.AlonzoTransactionBody{}
			},
			required: requiredBodyFields(),
		},
		{
			name: "Babbage",
			newBody: func() ledgerMapUnmarshaler {
				return &babbage.BabbageTransactionBody{}
			},
			required: requiredBodyFields(),
		},
		{
			name: "Conway",
			newBody: func() ledgerMapUnmarshaler {
				return &conway.ConwayTransactionBody{}
			},
			required: requiredBodyFields(),
		},
		{
			name: "Dijkstra",
			newBody: func() ledgerMapUnmarshaler {
				return &dijkstra.DijkstraTransactionBody{}
			},
			required: requiredBodyFields(),
		},
		{
			name: "Dijkstra sub-transaction",
			newBody: func() ledgerMapUnmarshaler {
				return &dijkstra.DijkstraSubTransactionBody{}
			},
			required: []ledgerMapEntry{
				{key: uint64(0), value: []any{}},
				{key: uint64(1), value: []any{}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			duplicateFields := append(
				append([]ledgerMapEntry(nil), tt.required...),
				ledgerMapEntry{key: uint64(0), value: []any{}},
			)
			duplicateWire := encodeLedgerMap(t, duplicateFields...)
			_, err := cbor.DecodeLenient(duplicateWire, tt.newBody())
			require.Error(t, err)
			require.True(t, cbor.IsDuplicateMapKeyError(err))

			unknownFields := append(
				append([]ledgerMapEntry(nil), tt.required...),
				ledgerMapEntry{key: uint64(99), value: uint64(0)},
			)
			unknownWire := encodeLedgerMap(t, unknownFields...)
			_, err = cbor.DecodeLenient(unknownWire, tt.newBody())
			require.Error(t, err)
		})
	}
}

func TestBabbageAndConwayOutputsRejectDuplicateAndUnknownMapFields(
	t *testing.T,
) {
	address, err := common.NewAddress(
		"addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd",
	)
	require.NoError(t, err)
	addressCBOR, err := cbor.Encode(address)
	require.NoError(t, err)

	duplicateOutput := encodeLedgerMap(
		t,
		ledgerMapEntry{key: uint64(0), value: cbor.RawMessage(addressCBOR)},
		ledgerMapEntry{key: uint64(1), value: uint64(0)},
		ledgerMapEntry{key: uint64(1), value: uint64(0)},
	)
	unknownOutput := encodeLedgerMap(
		t,
		ledgerMapEntry{key: uint64(0), value: cbor.RawMessage(addressCBOR)},
		ledgerMapEntry{key: uint64(1), value: uint64(0)},
		ledgerMapEntry{key: uint64(99), value: uint64(0)},
	)
	for _, tc := range []struct {
		name string
		wire []byte
	}{
		{name: "duplicate output field", wire: duplicateOutput},
		{name: "unknown output field", wire: unknownOutput},
	} {
		t.Run("Babbage "+tc.name, func(t *testing.T) {
			var output babbage.BabbageTransactionOutput
			_, err := cbor.DecodeLenient(tc.wire, &output)
			require.Error(t, err)
			if tc.name == "duplicate output field" {
				require.True(t, cbor.IsDuplicateMapKeyError(err))
			}
		})
		t.Run("Conway "+tc.name, func(t *testing.T) {
			bodyWire := encodeLedgerMap(
				t,
				ledgerMapEntry{key: uint64(0), value: []any{}},
				ledgerMapEntry{
					key: uint64(1),
					value: []any{
						cbor.RawMessage(tc.wire),
					},
				},
				ledgerMapEntry{key: uint64(2), value: uint64(0)},
			)
			var body conway.ConwayTransactionBody
			_, err := cbor.DecodeLenient(bodyWire, &body)
			require.Error(t, err)
			if tc.name == "duplicate output field" {
				require.True(t, cbor.IsDuplicateMapKeyError(err))
			}
		})
	}
}

func requiredBodyFields() []ledgerMapEntry {
	return []ledgerMapEntry{
		{key: uint64(0), value: []any{}},
		{key: uint64(1), value: []any{}},
		{key: uint64(2), value: uint64(0)},
	}
}
