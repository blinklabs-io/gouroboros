// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package script_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

// TxInfoV2 renders its inputs, reference inputs and outputs through
// WithZeroAdaAsset (unlike TxInfoV1, which wraps them in WithOptionDatum and so
// only ever shows a datum hash, and unlike TxInfoV3, which dispatches to each
// output's own ToPlutusData and picks up the Normalize in
// BabbageTransactionOutput). That made WithZeroAdaAsset's TransactionOutput
// case the one script-visible inline-datum boundary with no Normalize on it: it
// used Data.Clone(), and Clone deliberately preserves the wire's
// definite/indefinite array encoding.
//
// The consequence is the same divergence that stalled a mainnet block producer
// on tx deab9ef3...de8a56: cardano-ledger builds script-visible values fresh,
// so a definite-encoded inline datum handed to a PlutusV2 script serialises to
// bytes the reference implementation never emits, and any script that hashes or
// compares serialiseData of it disagrees with the reference.
//
// The datum below is DEFINITE-encoded on the wire on purpose. A fixture that
// happens to be indefinite cannot regress this: Normalize is a no-op there.
func TestWithZeroAdaAssetNormalizesInlineDatum(t *testing.T) {
	// Constr tag 0, one ByteString field, definite-length array:
	//   d8 79   tag(121)
	//   81      array(1)          <- definite; Normalize rewrites this to 9f/ff
	//   41 2a   bytes(1) 0x2a
	const wireDatumHex = "d87981412a"
	// Babbage output map: {0: address, 1: coin, 2: [1, #6.24(bytes .cbor datum)]}
	outputCbor, err := hex.DecodeString(
		"a3" +
			"00" + "581d" + "61" + "00000000000000000000000000000000000000000000000000000000" +
			"01" + "1a000f4240" +
			"02" + "82" + "01" + "d818" + "45" + wireDatumHex,
	)
	require.NoError(t, err)

	var output babbage.BabbageTransactionOutput
	_, err = cbor.Decode(outputCbor, &output)
	require.NoError(t, err)
	require.NotNil(t, output.Datum(), "fixture must carry an inline datum")

	// Confirm the fixture really is definite on the wire, so a pass here can
	// only come from Normalize and not from the input already being canonical.
	asDecoded, err := data.Encode(output.Datum().Data)
	require.NoError(t, err)
	require.Equal(
		t, wireDatumHex, hex.EncodeToString(asDecoded),
		"fixture datum should round-trip to its definite-length wire bytes",
	)

	wantNormalized, err := data.Encode(data.Normalize(output.Datum().Data))
	require.NoError(t, err)
	require.NotEqual(
		t, wireDatumHex, hex.EncodeToString(wantNormalized),
		"Normalize must change this fixture, or the test proves nothing",
	)

	rendered, err := data.Encode(
		script.WithZeroAdaAsset{Value: output}.ToPlutusData(),
	)
	require.NoError(t, err)
	renderedHex := hex.EncodeToString(rendered)

	require.Contains(
		t, renderedHex, hex.EncodeToString(wantNormalized),
		"rendered output must embed the NORMALIZED inline datum",
	)
	require.NotContains(
		t, renderedHex, wireDatumHex,
		"rendered output still embeds the definite-length wire datum: "+
			"WithZeroAdaAsset is handing unnormalized PlutusData to the script",
	)
}
