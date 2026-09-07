// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dijkstra

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

// TestDijkstraRequiredGuardsV4NormalizesDatum exercises the
// dijkstraRequiredGuardsV4 code path with a deliberately definite-encoded
// inline datum. The fix at plutus_v4.go:1341 (data.Normalize(datum.Data))
// ensures that a required top-level guard datum reaches a PlutusV4 script
// with canonical indefinite-length encoding, matching cardano-ledger's
// fresh reconstruction of script-visible data.
func TestDijkstraRequiredGuardsV4NormalizesDatum(t *testing.T) {
	// DEFINITE-encoded datum on the wire.
	//   d8 79       tag(121) -- datum
	//   81          array(1)     <- definite-length; Normalize rewrites to 9f/ff
	//   41 2a       bytes(1) 0x2a
	const wireDatumHex = "d87981412a"

	// Build a Datum carrying the definite-length datum.
	wireDatumBytes, err := hex.DecodeString(wireDatumHex)
	require.NoError(t, err)
	var datum common.Datum
	datum.SetCbor(wireDatumBytes)
	tmpData, err := data.Decode(wireDatumBytes)
	require.NoError(t, err)
	datum.Data = tmpData

	// Confirm the fixture really is definite on the wire, so a pass here
	// can only come from Normalize and not from the input already being canonical.
	asDecoded, err := data.Encode(datum.Data)
	require.NoError(t, err)
	require.Equal(
		t, wireDatumHex, hex.EncodeToString(asDecoded),
		"fixture datum should round-trip to its definite-length wire bytes",
	)

	wantNormalized, err := data.Encode(data.Normalize(datum.Data))
	require.NoError(t, err)
	require.NotEqual(
		t, wireDatumHex, hex.EncodeToString(wantNormalized),
		"Normalize must change this fixture, or the test proves nothing",
	)

	// A script-hash credential for the guard (28 bytes exactly).
	var guardHash common.Blake2b224
	copy(guardHash[:], []byte{
		0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00,
		0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88,
		0x99, 0xaa, 0xbb, 0xcc,
	})
	guardCred := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: guardHash,
	}

	// Encode the required-guards map exactly as it appears in a Dijkstra
	// sub-transaction body (map from credential to optional datum).
	// The credential key is a struct-as-array [Type, Hash].
	credKey := dijkstraV4TestCredentialKey{
		Type: uint(guardCred.CredType),
		Hash: guardCred.Credential,
	}
	datumCbor, err := cbor.Encode(datum)
	require.NoError(t, err)
	requiredGuardsRaw, err := cbor.Encode(map[dijkstraV4TestCredentialKey]cbor.RawMessage{
		credKey: datumCbor,
	})
	require.NoError(t, err)

	// Call the internal function under test.
	rendered, err := dijkstraRequiredGuardsV4(requiredGuardsRaw)
	require.NoError(t, err)

	renderedBytes, err := data.Encode(rendered)
	require.NoError(t, err)
	renderedHex := hex.EncodeToString(renderedBytes)

	require.Contains(
		t, renderedHex, hex.EncodeToString(wantNormalized),
		"rendered required-guards must embed the NORMALIZED guard datum",
	)
	require.NotContains(
		t, renderedHex, wireDatumHex,
		"rendered required-guards still embeds the definite-length wire datum: "+
			"dijkstraRequiredGuardsV4 is handing unnormalized PlutusData to the script",
	)
}