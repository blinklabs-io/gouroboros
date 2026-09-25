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

package common_test

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

func encodeTaggedAuxiliaryData(
	t *testing.T,
	fields map[uint]any,
) []byte {
	t.Helper()
	content, err := cbor.Encode(fields)
	require.NoError(t, err)
	tagged, err := cbor.Encode(cbor.RawTag{
		Number:  cbor.CborTagMap,
		Content: content,
	})
	require.NoError(t, err)
	return tagged
}

func TestDecodeAuxiliaryDataForEraFormatMatrix(t *testing.T) {
	t.Parallel()

	formats := map[string][]byte{
		"metadata map":     {0xa0},
		"Allegra array":    {0x82, 0xf6, 0x80},
		"tagged auxiliary": {0xd9, 0x01, 0x03, 0xa0},
	}
	for _, era := range []struct {
		name  string
		value common.AuxiliaryDataEra
	}{
		{"Shelley", common.AuxiliaryDataEraShelley},
		{"Allegra", common.AuxiliaryDataEraAllegra},
		{"Mary", common.AuxiliaryDataEraMary},
		{"Alonzo", common.AuxiliaryDataEraAlonzo},
		{"Babbage", common.AuxiliaryDataEraBabbage},
		{"Conway", common.AuxiliaryDataEraConway},
		{"Dijkstra", common.AuxiliaryDataEraDijkstra},
	} {
		t.Run(era.name, func(t *testing.T) {
			t.Parallel()
			for format, raw := range formats {
				want := format == "metadata map" ||
					(format == "Allegra array" && era.value != common.AuxiliaryDataEraShelley) ||
					(format == "tagged auxiliary" && era.value >= common.AuxiliaryDataEraAlonzo)
				_, err := common.DecodeAuxiliaryDataForEra(raw, era.value)
				if want {
					require.NoError(t, err, "%s in %s", format, era.name)
				} else {
					require.Error(t, err, "%s in %s", format, era.name)
				}
			}
		})
	}
}

func TestAuxiliaryNativeScriptConstructorsRespectEra(t *testing.T) {
	t.Parallel()

	// Constructor 6 was added with Dijkstra and must remain a valid native
	// script there while earlier eras reject it, even inside auxiliary data.
	nativeScript := mustEncodeCBOR(t, []any{
		uint(6),
		[]any{uint(0), make([]byte, 28)},
	})
	scriptList := mustEncodeCBOR(t, []cbor.RawMessage{nativeScript})
	array := mustEncodeCBOR(t, []cbor.RawMessage{
		{0xf6},
		scriptList,
	})
	_, err := common.DecodeAuxiliaryDataForEra(array, common.AuxiliaryDataEraMary)
	require.ErrorContains(t, err, "native script constructor 6 is not supported")
	_, err = common.DecodeAuxiliaryDataForEra(array, common.AuxiliaryDataEraDijkstra)
	require.NoError(t, err)

	tagged := encodeTaggedAuxiliaryData(t, map[uint]any{
		1: []cbor.RawMessage{nativeScript},
	})
	_, err = common.DecodeAuxiliaryDataForEra(tagged, common.AuxiliaryDataEraConway)
	require.ErrorContains(t, err, "native script constructor 6 is not supported")
	_, err = common.DecodeAuxiliaryDataForEra(tagged, common.AuxiliaryDataEraDijkstra)
	require.NoError(t, err)
}

func TestDecodeAuxiliaryDataForEraPlutusLanguageMatrix(t *testing.T) {
	t.Parallel()

	for _, era := range []struct {
		name common.AuxiliaryDataEra
		max  uint
	}{
		{common.AuxiliaryDataEraAlonzo, 1},
		{common.AuxiliaryDataEraBabbage, 2},
		{common.AuxiliaryDataEraConway, 3},
		{common.AuxiliaryDataEraDijkstra, 4},
	} {
		for version := uint(1); version <= 4; version++ {
			version := version
			t.Run(fmt.Sprintf("%d/V%d", era.name, version), func(t *testing.T) {
				t.Parallel()
				raw := encodeTaggedAuxiliaryData(t, map[uint]any{
					version + 1: []any{},
				})
				_, err := common.DecodeAuxiliaryDataForEra(raw, era.name)
				if version <= era.max {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "not supported in this era")
				}
			})
		}
	}
}

func TestDecodeAuxiliaryDataForEraRejectsNonArrayScriptFields(t *testing.T) {
	t.Parallel()

	for key := uint(1); key <= 5; key++ {
		key := key
		t.Run(fmt.Sprintf("field_%d", key), func(t *testing.T) {
			t.Parallel()
			raw := encodeTaggedAuxiliaryData(t, map[uint]any{
				key: cbor.RawMessage{0xf6},
			})
			_, err := common.DecodeAuxiliaryDataForEra(
				raw,
				common.AuxiliaryDataEraDijkstra,
			)
			require.ErrorContains(t, err, "must be a CBOR array")
		})
	}
}

func TestDecodeAuxiliaryDataForEraRejectsUnknownTaggedFields(t *testing.T) {
	t.Parallel()

	raw := encodeTaggedAuxiliaryData(t, map[uint]any{
		0:  map[uint]any{1: "ok"},
		99: []byte{1},
	})
	for _, era := range []common.AuxiliaryDataEra{
		common.AuxiliaryDataEraAlonzo,
		common.AuxiliaryDataEraBabbage,
		common.AuxiliaryDataEraConway,
		common.AuxiliaryDataEraDijkstra,
	} {
		era := era
		t.Run(fmt.Sprintf("era_%d", era), func(t *testing.T) {
			t.Parallel()
			_, err := common.DecodeAuxiliaryDataForEra(raw, era)
			require.ErrorContains(t, err, "unknown auxiliary-data field 99")
		})
	}
}

func TestDecodeAuxiliaryDataForEraRequiresTransactionMetadataDomain(t *testing.T) {
	t.Parallel()

	textLabelMetadata := mustEncodeCBOR(t, map[string]any{"label": "value"})
	_, err := common.DecodeAuxiliaryDataForEra(
		textLabelMetadata,
		common.AuxiliaryDataEraShelley,
	)
	require.ErrorContains(t, err, "labels must be unsigned 64-bit integers")

	taggedScalar := encodeTaggedAuxiliaryData(t, map[uint]any{
		0: mustEncodeCBOR(t, "metadata must be a map"),
	})
	_, err = common.DecodeAuxiliaryDataForEra(taggedScalar, common.AuxiliaryDataEraAlonzo)
	require.ErrorContains(t, err, "transaction metadata must be a map")

	taggedTextLabel := encodeTaggedAuxiliaryData(t, map[uint]any{
		0: textLabelMetadata,
	})
	_, err = common.DecodeAuxiliaryDataForEra(taggedTextLabel, common.AuxiliaryDataEraConway)
	require.ErrorContains(t, err, "labels must be unsigned 64-bit integers")

	arrayTextLabel := mustEncodeCBOR(t, []cbor.RawMessage{
		textLabelMetadata,
		{0x80},
	})
	_, err = common.DecodeAuxiliaryDataForEra(arrayTextLabel, common.AuxiliaryDataEraMary)
	require.ErrorContains(t, err, "labels must be unsigned 64-bit integers")
}

func TestAuxiliaryScriptValidationUsesActiveProtocolContext(t *testing.T) {
	t.Parallel()

	for _, era := range []struct {
		name          common.AuxiliaryDataEra
		max           uint
		protocolMajor uint
	}{
		{common.AuxiliaryDataEraAlonzo, 1, 6},
		{common.AuxiliaryDataEraBabbage, 2, 8},
		{common.AuxiliaryDataEraConway, 3, 9},
		{common.AuxiliaryDataEraDijkstra, 4, 12},
	} {
		for version := uint(1); version <= era.max; version++ {
			version := version
			t.Run(fmt.Sprintf("%d/V%d", era.name, version), func(t *testing.T) {
				t.Parallel()
				uplcVersion := lang.LanguageVersionV1
				if version > 1 {
					uplcVersion = lang.LanguageVersionV2
				}
				validScript := encodePlutusContextTestScript(
					t,
					uplcVersion,
					0,
					nil,
				)
				key := version + 1
				valid := encodeTaggedAuxiliaryData(t, map[uint]any{
					key: []cbor.RawMessage{mustEncodeCBOR(t, validScript)},
				})
				aux, err := common.DecodeAuxiliaryDataForEra(valid, era.name)
				require.NoError(t, err, "valid UPLC must remain accepted")
				require.NoError(t, common.ValidateAuxiliaryDataPlutusScriptsWellFormed(
					aux,
					era.protocolMajor,
				))

				malformedWrapper := mustEncodeCBOR(t, []byte("not a FLAT program"))
				malformed := encodeTaggedAuxiliaryData(t, map[uint]any{
					key: []cbor.RawMessage{malformedWrapper},
				})
				aux, err = common.DecodeAuxiliaryDataForEra(malformed, era.name)
				require.NoError(t, err, "CBOR decoding does not apply UPLC validation")
				err = common.ValidateAuxiliaryDataPlutusScriptsWellFormed(
					aux,
					era.protocolMajor,
				)
				require.ErrorContains(t, err, "malformed auxiliary-data Plutus V")
			})
		}
	}
}

func mustEncodeCBOR(t *testing.T, value any) cbor.RawMessage {
	t.Helper()
	raw, err := cbor.Encode(value)
	require.NoError(t, err)
	return raw
}

func TestDecodeAuxiliaryDataForEraAcceptsStoredNewerTermVersion(t *testing.T) {
	t.Parallel()

	// Plutus V1 carries UPLC version 1.1.0 here but is never invoked. #2316
	// moved the Van Rossem term-version check to execution, so auxiliary-data
	// validation must not apply that execution-time gate.
	script := encodePlutusContextTestScript(
		t,
		lang.LanguageVersionV2,
		0,
		nil,
	)
	raw := encodeTaggedAuxiliaryData(t, map[uint]any{
		2: []cbor.RawMessage{mustEncodeCBOR(t, script)},
	})
	aux, err := common.DecodeAuxiliaryDataForEra(raw, common.AuxiliaryDataEraAlonzo)
	require.NoError(t, err)
	require.NoError(t, common.ValidateAuxiliaryDataPlutusScriptsWellFormed(aux, 6))
}

func TestEraDecodersRejectMalformedAuxiliaryData(t *testing.T) {
	t.Parallel()

	validMetadata := map[uint]any{0: map[uint]any{1: "ok"}}
	unknownField := map[uint]any{0: map[uint]any{1: "ok"}, 99: []byte{1}}
	malformed := []struct {
		name        string
		blockHex    string
		eraFormat   []byte
		blockDecode func([]byte) error
		txDecode    func([]byte) error
		fourFields  bool
		invalidTx   bool
		wantError   string
	}{
		{
			name:      "Shelley Allegra array",
			blockHex:  testdata.ShelleyBlockHex,
			eraFormat: []byte{0x82, 0xf6, 0x80},
			wantError: "not supported in this era",
			blockDecode: func(raw []byte) error {
				_, err := shelley.NewShelleyBlockFromCbor(raw)
				return err
			},
			txDecode: func(raw []byte) error {
				_, err := shelley.NewShelleyTransactionFromCbor(raw)
				return err
			},
		},
		{
			name:      "Shelley tagged map",
			blockHex:  testdata.ShelleyBlockHex,
			eraFormat: encodeTaggedAuxiliaryData(t, validMetadata),
			wantError: "not supported in this era",
			blockDecode: func(raw []byte) error {
				_, err := shelley.NewShelleyBlockFromCbor(raw)
				return err
			},
			txDecode: func(raw []byte) error {
				_, err := shelley.NewShelleyTransactionFromCbor(raw)
				return err
			},
		},
		{
			name:      "Allegra tagged map",
			blockHex:  testdata.AllegraBlockHex,
			eraFormat: encodeTaggedAuxiliaryData(t, validMetadata),
			wantError: "not supported in this era",
			blockDecode: func(raw []byte) error {
				_, err := allegra.NewAllegraBlockFromCbor(raw)
				return err
			},
			txDecode: func(raw []byte) error {
				_, err := allegra.NewAllegraTransactionFromCbor(raw)
				return err
			},
		},
		{
			name:      "Mary tagged map",
			blockHex:  testdata.MaryBlockHex,
			eraFormat: encodeTaggedAuxiliaryData(t, validMetadata),
			wantError: "not supported in this era",
			blockDecode: func(raw []byte) error {
				_, err := mary.NewMaryBlockFromCbor(raw)
				return err
			},
			txDecode: func(raw []byte) error {
				_, err := mary.NewMaryTransactionFromCbor(raw)
				return err
			},
		},
		{
			name:       "Alonzo V2 field",
			blockHex:   testdata.AlonzoBlockHex,
			eraFormat:  encodeTaggedAuxiliaryData(t, map[uint]any{3: []any{}}),
			fourFields: true,
			wantError:  "auxiliary scripts for Plutus V2 are not supported",
			blockDecode: func(raw []byte) error {
				_, err := alonzo.NewAlonzoBlockFromCbor(raw)
				return err
			},
			txDecode: func(raw []byte) error {
				_, err := alonzo.NewAlonzoTransactionFromCbor(raw)
				return err
			},
		},
		{
			name:       "Babbage V3 field",
			blockHex:   testdata.BabbageBlockHex,
			eraFormat:  encodeTaggedAuxiliaryData(t, map[uint]any{4: []any{}}),
			fourFields: true,
			wantError:  "auxiliary scripts for Plutus V3 are not supported",
			blockDecode: func(raw []byte) error {
				_, err := babbage.NewBabbageBlockFromCbor(raw)
				return err
			},
			txDecode: func(raw []byte) error {
				_, err := babbage.NewBabbageTransactionFromCbor(raw)
				return err
			},
		},
		{
			name:       "Conway V4 field",
			blockHex:   testdata.ConwayBlockHex,
			eraFormat:  encodeTaggedAuxiliaryData(t, map[uint]any{5: []any{}}),
			fourFields: true,
			wantError:  "auxiliary scripts for Plutus V4 are not supported",
			blockDecode: func(raw []byte) error {
				_, err := conway.NewConwayBlockFromCbor(raw)
				return err
			},
			txDecode: func(raw []byte) error {
				_, err := conway.NewConwayTransactionFromCbor(raw)
				return err
			},
		},
	}

	for _, tc := range malformed {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			block, tx := blockAndTransactionWithAuxiliaryData(
				t,
				tc.blockHex,
				tc.eraFormat,
				tc.fourFields,
				tc.invalidTx,
			)
			require.ErrorContains(t, tc.blockDecode(block), tc.wantError, "raw block must reject for the era rule")
			require.ErrorContains(t, tc.txDecode(tx), tc.wantError, "raw transaction must reject for the era rule")
		})
	}

	t.Run("Dijkstra unknown tagged field", func(t *testing.T) {
		t.Parallel()
		aux := encodeTaggedAuxiliaryData(t, unknownField)
		tx, block := dijkstraBlockAndTransactionWithAuxiliaryData(t, aux)
		var err error
		_, err = dijkstra.NewDijkstraTransactionFromCbor(tx)
		require.ErrorContains(t, err, "unknown auxiliary-data field 99")
		_, err = dijkstra.NewDijkstraBlockFromCbor(block)
		require.ErrorContains(t, err, "unknown auxiliary-data field 99")
	})
	t.Run("Dijkstra malformed Plutus V4 auxiliary script", func(t *testing.T) {
		t.Parallel()
		aux := encodeTaggedAuxiliaryData(t, map[uint]any{
			5: []cbor.RawMessage{mustEncodeCBOR(t, []byte("not a FLAT program"))},
		})
		tx, block := dijkstraBlockAndTransactionWithAuxiliaryData(t, aux)
		decoded, err := dijkstra.NewDijkstraTransactionFromCbor(tx)
		require.NoError(t, err, "CBOR decoding does not apply UPLC validation")
		_, err = dijkstra.NewDijkstraBlockFromCbor(block)
		require.NoError(t, err, "CBOR decoding does not apply UPLC validation")
		err = dijkstra.UtxoValidateMalformedReferenceScripts(
			decoded,
			0,
			nil,
			&dijkstra.DijkstraProtocolParameters{
				ConwayProtocolParameters: conway.ConwayProtocolParameters{
					ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 12},
				},
			},
		)
		require.ErrorContains(t, err, "malformed auxiliary-data Plutus V4 script")
	})
}

func dijkstraBlockAndTransactionWithAuxiliaryData(
	t *testing.T,
	auxiliaryData []byte,
) ([]byte, []byte) {
	t.Helper()
	body, witness := dijkstraTestTransactionParts(t, auxiliaryData)
	tx, err := cbor.Encode([]cbor.RawMessage{body, witness, auxiliaryData})
	require.NoError(t, err)
	validity, err := cbor.Encode(true)
	require.NoError(t, err)
	blockTx, err := cbor.Encode([]cbor.RawMessage{body, witness, auxiliaryData, validity})
	require.NoError(t, err)
	txs, err := cbor.Encode([]cbor.RawMessage{blockTx})
	require.NoError(t, err)
	blockBody, err := cbor.Encode([]cbor.RawMessage{txs, {0xf6}, {0xf6}})
	require.NoError(t, err)
	conwayBlock, err := hex.DecodeString(strings.TrimSpace(testdata.ConwayBlockHex))
	require.NoError(t, err)
	var conwayComponents []cbor.RawMessage
	_, err = cbor.Decode(conwayBlock, &conwayComponents)
	require.NoError(t, err)
	require.NotEmpty(t, conwayComponents)
	var headerComponents []cbor.RawMessage
	_, err = cbor.Decode(conwayComponents[0], &headerComponents)
	require.NoError(t, err)
	require.NotEmpty(t, headerComponents)
	var headerBody []cbor.RawMessage
	_, err = cbor.Decode(headerComponents[0], &headerBody)
	require.NoError(t, err)
	require.NotEmpty(t, headerBody)
	blockBodyHash, err := cbor.Encode(common.Blake2b256Hash(blockBody).Bytes())
	require.NoError(t, err)
	headerBody[7] = blockBodyHash
	headerComponents[0], err = cbor.Encode(headerBody)
	require.NoError(t, err)
	conwayComponents[0], err = cbor.Encode(headerComponents)
	require.NoError(t, err)
	block, err := cbor.Encode([]cbor.RawMessage{conwayComponents[0], blockBody})
	require.NoError(t, err)
	return tx, block
}

func TestEraDecodersPreserveValidAuxiliaryScripts(t *testing.T) {
	t.Parallel()

	script := encodePlutusContextTestScript(t, lang.LanguageVersionV1, 0, nil)
	aux := encodeTaggedAuxiliaryData(t, map[uint]any{
		2: []cbor.RawMessage{mustEncodeCBOR(t, script)},
	})
	block, tx := blockAndTransactionWithAuxiliaryData(
		t,
		testdata.AlonzoBlockHex,
		aux,
		true,
		true,
	)
	_, err := alonzo.NewAlonzoBlockFromCbor(
		block,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	decoded, err := alonzo.NewAlonzoTransactionFromCbor(tx)
	require.NoError(t, err)
	require.False(t, decoded.IsValid())
	require.NotNil(t, decoded.AuxiliaryData())
	require.NoError(t, alonzo.UtxoValidateMetadata(
		decoded,
		0,
		nil,
		&alonzo.AlonzoProtocolParameters{ProtocolMajor: 6},
	))
}

func TestAuxiliaryMalformedScriptsRejectedByEraRules(t *testing.T) {
	t.Parallel()

	malformedScript := mustEncodeCBOR(t, []byte("not a FLAT program"))
	t.Run("Alonzo V1 even when transaction is invalid", func(t *testing.T) {
		t.Parallel()
		aux := encodeTaggedAuxiliaryData(t, map[uint]any{
			2: []cbor.RawMessage{malformedScript},
		})
		_, txRaw := blockAndTransactionWithAuxiliaryData(
			t, testdata.AlonzoBlockHex, aux, true, true,
		)
		tx, err := alonzo.NewAlonzoTransactionFromCbor(txRaw)
		require.NoError(t, err)
		require.False(t, tx.IsValid())
		err = alonzo.UtxoValidateMetadata(
			tx, 0, nil, &alonzo.AlonzoProtocolParameters{ProtocolMajor: 6},
		)
		require.ErrorContains(t, err, "malformed auxiliary-data Plutus V1 script")
	})
	t.Run("Babbage V2", func(t *testing.T) {
		t.Parallel()
		aux := encodeTaggedAuxiliaryData(t, map[uint]any{
			3: []cbor.RawMessage{malformedScript},
		})
		_, txRaw := blockAndTransactionWithAuxiliaryData(
			t, testdata.BabbageBlockHex, aux, true, false,
		)
		tx, err := babbage.NewBabbageTransactionFromCbor(txRaw)
		require.NoError(t, err)
		err = babbage.UtxoValidateMalformedReferenceScripts(
			tx, 0, nil, &babbage.BabbageProtocolParameters{ProtocolMajor: 8},
		)
		require.ErrorContains(t, err, "malformed auxiliary-data Plutus V2 script")
	})
	t.Run("Conway V3", func(t *testing.T) {
		t.Parallel()
		aux := encodeTaggedAuxiliaryData(t, map[uint]any{
			4: []cbor.RawMessage{malformedScript},
		})
		_, txRaw := blockAndTransactionWithAuxiliaryData(
			t, testdata.ConwayBlockHex, aux, true, false,
		)
		tx, err := conway.NewConwayTransactionFromCbor(txRaw)
		require.NoError(t, err)
		err = conway.UtxoValidateMalformedReferenceScripts(
			tx, 0, nil, &conway.ConwayProtocolParameters{
				ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 9},
			},
		)
		require.ErrorContains(t, err, "malformed auxiliary-data Plutus V3 script")
	})
}

func dijkstraTestTransactionParts(
	t *testing.T,
	auxiliaryData []byte,
) (cbor.RawMessage, cbor.RawMessage) {
	t.Helper()
	body := map[uint]cbor.RawMessage{}
	for key, value := range map[uint]any{
		0: []any{},
		1: []any{},
		2: uint64(0),
	} {
		raw, err := cbor.Encode(value)
		require.NoError(t, err)
		body[key] = raw
	}
	hash, err := cbor.Encode(common.Blake2b256Hash(auxiliaryData).Bytes())
	require.NoError(t, err)
	body[7] = hash
	bodyRaw, err := cbor.Encode(body)
	require.NoError(t, err)
	witnessRaw, err := cbor.Encode(map[uint]any{})
	require.NoError(t, err)
	return bodyRaw, witnessRaw
}

func blockAndTransactionWithAuxiliaryData(
	t *testing.T,
	blockHex string,
	auxiliaryData []byte,
	fourFields bool,
	invalidTx bool,
) ([]byte, []byte) {
	t.Helper()
	blockRaw, err := hex.DecodeString(strings.TrimSpace(blockHex))
	require.NoError(t, err)
	var blockComponents []cbor.RawMessage
	_, err = cbor.Decode(blockRaw, &blockComponents)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(blockComponents), 4)

	var bodies []cbor.RawMessage
	_, err = cbor.Decode(blockComponents[1], &bodies)
	require.NoError(t, err)
	require.NotEmpty(t, bodies)
	var bodyFields map[uint]cbor.RawMessage
	_, err = cbor.Decode(bodies[0], &bodyFields)
	require.NoError(t, err)
	require.NotNil(t, bodyFields)
	hashRaw, err := cbor.Encode(common.Blake2b256Hash(auxiliaryData).Bytes())
	require.NoError(t, err)
	bodyFields[7] = hashRaw
	bodies[0], err = cbor.Encode(bodyFields)
	require.NoError(t, err)
	blockComponents[1], err = cbor.Encode(bodies)
	require.NoError(t, err)
	metadataSetRaw, err := cbor.Encode(map[uint]cbor.RawMessage{0: auxiliaryData})
	require.NoError(t, err)
	blockComponents[3] = metadataSetRaw
	var headerComponents []cbor.RawMessage
	_, err = cbor.Decode(blockComponents[0], &headerComponents)
	require.NoError(t, err)
	require.NotEmpty(t, headerComponents)
	var headerBody []cbor.RawMessage
	_, err = cbor.Decode(headerComponents[0], &headerBody)
	require.NoError(t, err)
	require.NotEmpty(t, headerBody)
	bodyHashIndex := 8
	if len(headerBody) == 10 {
		bodyHashIndex = 7
	}
	require.Greater(t, len(headerBody), bodyHashIndex)
	var bodyHashes []byte
	for _, component := range blockComponents[1:] {
		bodyHashes = append(bodyHashes, common.Blake2b256Hash(component).Bytes()...)
	}
	blockBodyHash := cbor.RawMessage(mustEncodeCBOR(t, common.Blake2b256Hash(bodyHashes).Bytes()))
	headerBody[bodyHashIndex] = blockBodyHash
	headerComponents[0], err = cbor.Encode(headerBody)
	require.NoError(t, err)
	blockComponents[0], err = cbor.Encode(headerComponents)
	require.NoError(t, err)
	blockRaw, err = cbor.Encode(blockComponents)
	require.NoError(t, err)

	var witnesses []cbor.RawMessage
	_, err = cbor.Decode(blockComponents[2], &witnesses)
	require.NoError(t, err)
	require.NotEmpty(t, witnesses)
	txComponents := []cbor.RawMessage{bodies[0], witnesses[0]}
	if fourFields {
		validRaw, encodeErr := cbor.Encode(!invalidTx)
		require.NoError(t, encodeErr)
		txComponents = append(txComponents, validRaw)
	}
	txComponents = append(txComponents, auxiliaryData)
	txRaw, err := cbor.Encode(txComponents)
	require.NoError(t, err)
	return blockRaw, txRaw
}
