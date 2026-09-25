package common

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var allegraBlockHex = "a219ef64a301582095b1d64fbf76f17b1920a34d14fbca1f5ab499ea59eac37a8117d5e6b2e09605025820f3157c8eda34976620ad12e0979b2d3135a784c5d6a185878987143053c17d1c035839012c152eaa9e68dd7123a3054190dc987a24e50f1ab389c44a0c7a4089beb4d4d62d8f0dce5d745df4a670998aa20f54703b2bdc7a00b7d3d219ef65a1015840897063bdeab54d2e0586529909f20b42447bfaccdfb9988d2558896baf82a37f43c2fa4ae4240f5761e3dccf9523d7305d728f21dee4491e02373de6b14f7e07"

func TestBlockMetadataSetRejectsUnknownTaggedAuxiliaryField(t *testing.T) {
	metadata, err := cbor.Encode(map[uint]string{674: "metadata"})
	require.NoError(t, err)
	fields, err := cbor.Encode(map[uint]cbor.RawMessage{
		0:  metadata,
		99: {0x41, 0x01},
	})
	require.NoError(t, err)
	auxiliaryData, err := cbor.Encode(&cbor.RawTag{
		Number:  cbor.CborTagMap,
		Content: fields,
	})
	require.NoError(t, err)
	blockMetadata, err := cbor.Encode(map[uint]cbor.RawMessage{0: auxiliaryData})
	require.NoError(t, err)

	var set TransactionMetadataSet
	_, err = cbor.Decode(blockMetadata, &set)
	require.NoError(t, err)
	require.ErrorContains(
		t,
		set.ValidateAuxiliaryDataForEra(AuxiliaryDataEraAlonzo),
		"unknown auxiliary-data field 99",
	)
}

func Test_Metadata_RoundTrip_AllegraSample(t *testing.T) {

	raw, err := hex.DecodeString(allegraBlockHex)
	if err != nil {
		t.Fatalf("bad hex: %v", err)
	}

	var set TransactionMetadataSet
	if _, err := cbor.Decode(raw, &set); err != nil {
		t.Fatalf("decode: %v", err)
	}

	enc, err := set.MarshalCBOR()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	if hex.EncodeToString(enc) != allegraBlockHex {
		t.Fatalf(
			"mismatch:\n got: %s\nwant: %s",
			hex.EncodeToString(enc),
			allegraBlockHex,
		)
	}
}

// Test decoding a CIP-0025-like NFT metadata structure under label 721
func TestCIP25_NFTMetadataDecode(t *testing.T) {
	// Construct a simple CIP-25 style metadata map:
	// {721: {"policyid": {"MyNFT": {"name":"Test NFT","image":"ipfs://abc"}}}}
	innerAsset := make(map[string]any)
	innerAsset["name"] = "Test NFT"
	innerAsset["image"] = "ipfs://abc"

	assets := make(map[string]any)
	assets["MyNFT"] = innerAsset

	policyMap := make(map[string]any)
	policyMap["policyid"] = assets

	outer := make(map[uint]any)
	outer[721] = policyMap

	data, err := cbor.Encode(outer)
	if err != nil {
		t.Fatalf("failed to encode CIP-25 test metadata: %v", err)
	}

	aux, err := DecodeAuxiliaryData(data)
	if err != nil {
		t.Fatalf("DecodeAuxiliaryData failed: %v", err)
	}

	md, err := aux.Metadata()
	if err != nil {
		t.Fatalf("Metadata() error: %v", err)
	}
	if md == nil {
		t.Fatal("expected metadata, got nil")
	}

	// Expect a MetaMap
	mm, ok := md.(MetaMap)
	if !ok {
		t.Fatalf("expected MetaMap, got %T", md)
	}

	// Find key 721 in pairs
	var found bool
	for _, p := range mm.Pairs {
		if ki, ok := p.Key.(MetaInt); ok {
			if ki.Value != nil && ki.Value.Uint64() == 721 {
				found = true
				// value should be a MetaMap representing policy map
				if _, ok := p.Value.(MetaMap); !ok {
					t.Fatalf("expected MetaMap for 721 value, got %T", p.Value)
				}
				break
			}
		}
	}
	if !found {
		t.Fatal("did not find metadata label 721 in decoded pairs")
	}

	// Additional sanity: roundtrip encode/decode
	re := aux.Cbor()
	aux2, err := DecodeAuxiliaryData(re)
	if err != nil {
		t.Fatalf("roundtrip DecodeAuxiliaryData failed: %v", err)
	}
	if _, err := aux2.Metadata(); err != nil {
		t.Fatalf("roundtrip metadata() failed: %v", err)
	}
}

func TestMetadataSetPreservesButEraDecoderRejectsUnknownAuxiliaryDataKeys(t *testing.T) {
	// {6: #6.259({0: {1: "ok"}, 6: [1]})}
	// Key 6 inside the auxiliary-data map is a VanRossem-era extension.
	const metadataSetHex = "a106d90103a200a101626f6b068101"
	const auxiliaryDataHex = "d90103a200a101626f6b068101"

	raw, err := hex.DecodeString(metadataSetHex)
	require.NoError(t, err)

	var set TransactionMetadataSet
	_, err = cbor.Decode(raw, &set)
	require.NoError(t, err)

	md, ok := set.GetMetadata(6)
	require.True(t, ok)
	require.NotNil(t, md)
	assertMetadataEntry(t, md)

	rawMd, ok := set.GetRawMetadata(6)
	require.True(t, ok)
	require.NotNil(t, rawMd)
	assert.Equal(t, auxiliaryDataHex, hex.EncodeToString(rawMd))

	_, err = DecodeAuxiliaryDataForEra(rawMd, AuxiliaryDataEraDijkstra)
	require.ErrorContains(t, err, "unknown auxiliary-data field 6")
}

func TestDecodeAuxiliaryDataForEra(t *testing.T) {
	arrayAux := []byte{0x82, 0xa0, 0x80}
	taggedAux := []byte{0xd9, 0x01, 0x03, 0xa0}
	tests := []struct {
		name string
		era  AuxiliaryDataEra
		raw  []byte
		ok   bool
	}{
		{"Shelley map", AuxiliaryDataEraShelley, []byte{0xa0}, true},
		{"Shelley rejects array", AuxiliaryDataEraShelley, arrayAux, false},
		{"Allegra accepts array", AuxiliaryDataEraAllegra, arrayAux, true},
		{"Mary accepts array", AuxiliaryDataEraMary, arrayAux, true},
		{"Mary rejects tag", AuxiliaryDataEraMary, taggedAux, false},
		{"Alonzo accepts tag", AuxiliaryDataEraAlonzo, taggedAux, true},
		{"Alonzo accepts non-minimal tag", AuxiliaryDataEraAlonzo, []byte{0xda, 0, 0, 1, 3, 0xa0}, true},
		{"Dijkstra accepts tag", AuxiliaryDataEraDijkstra, taggedAux, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeAuxiliaryDataForEra(test.raw, test.era)
			if test.ok {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestDecodeAuxiliaryDataForEraEnforcesPlutusLanguageBounds(t *testing.T) {
	auxiliaryData := func(field uint) []byte {
		scripts, err := cbor.Encode([][]byte{})
		require.NoError(t, err)
		fields, err := cbor.Encode(map[uint]cbor.RawMessage{field: scripts})
		require.NoError(t, err)
		tag := cbor.RawTag{Number: cbor.CborTagMap, Content: fields}
		encoded, err := cbor.Encode(&tag)
		require.NoError(t, err)
		return encoded
	}
	tests := []struct {
		name  string
		era   AuxiliaryDataEra
		field uint
		valid bool
	}{
		{name: "Alonzo V1", era: AuxiliaryDataEraAlonzo, field: 2, valid: true},
		{name: "Alonzo rejects V2", era: AuxiliaryDataEraAlonzo, field: 3},
		{name: "Babbage V2", era: AuxiliaryDataEraBabbage, field: 3, valid: true},
		{name: "Babbage rejects V3", era: AuxiliaryDataEraBabbage, field: 4},
		{name: "Conway V3", era: AuxiliaryDataEraConway, field: 4, valid: true},
		{name: "Conway rejects V4", era: AuxiliaryDataEraConway, field: 5},
		{name: "Dijkstra V3", era: AuxiliaryDataEraDijkstra, field: 4, valid: true},
		{name: "Dijkstra V4", era: AuxiliaryDataEraDijkstra, field: 5, valid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeAuxiliaryDataForEra(auxiliaryData(test.field), test.era)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "not supported in this era")
			}
		})
	}
}

func TestDecodeMetadatumRawRejectsNilGenericMapKey(t *testing.T) {
	raw, err := hex.DecodeString("a28031f730")
	if err != nil {
		t.Fatalf("bad hex: %v", err)
	}

	if _, err := DecodeMetadatumRaw(raw); err == nil {
		t.Fatal("expected error for unsupported metadata map key")
	}
}

// TestDuplicateNestedMetadataKeyDecodes reproduces
// blinklabs-io/gouroboros#2323: a nested metadata map (a label's own value,
// e.g. the CIP-25/CIP-721 shape) with a duplicate key must decode
// successfully and preserve every pair, matching upstream cardano-ledger's
// decodeMapN (libs/cardano-ledger-core/src/Cardano/Ledger/Metadata.hs), which
// has never rejected duplicate keys at this position in any era. Only the
// separately era-gated outer label map has ever had duplicate-key handling;
// see TestOuterAuxiliaryDataLabelMapRejectsDuplicateKeys.
func TestDuplicateNestedMetadataKeyDecodes(t *testing.T) {
	t.Parallel()

	// {721: {1: "a", 1: "b"}}, the exact bytes from the issue.
	const nestedDuplicateKeyHex = "a11902d1a2016161016162"

	requirePairs := func(t *testing.T, md TransactionMetadatum) {
		t.Helper()
		outer, ok := md.(MetaMap)
		require.True(t, ok, "expected outer MetaMap, got %T", md)
		require.Len(t, outer.Pairs, 1)
		inner, ok := outer.Pairs[0].Value.(MetaMap)
		require.True(t, ok, "expected inner MetaMap, got %T", outer.Pairs[0].Value)
		require.Len(
			t,
			inner.Pairs,
			2,
			"both pairs of the duplicate key must be preserved, not deduplicated",
		)
		for i, want := range []string{"a", "b"} {
			key, ok := inner.Pairs[i].Key.(MetaInt)
			require.True(t, ok)
			require.Equal(t, uint64(1), key.Value.Uint64())
			value, ok := inner.Pairs[i].Value.(MetaText)
			require.True(t, ok)
			require.Equal(t, want, value.Value)
		}
	}

	t.Run("Shelley", func(t *testing.T) {
		t.Parallel()
		raw, err := hex.DecodeString(nestedDuplicateKeyHex)
		require.NoError(t, err)
		var aux ShelleyAuxiliaryData
		require.NoError(t, aux.UnmarshalCBOR(raw))
		md, err := aux.Metadata()
		require.NoError(t, err)
		requirePairs(t, md)
	})

	t.Run("ShelleyMa", func(t *testing.T) {
		t.Parallel()
		// [metadata_map, native_scripts]
		raw, err := hex.DecodeString("82" + nestedDuplicateKeyHex + "80")
		require.NoError(t, err)
		var aux ShelleyMaAuxiliaryData
		require.NoError(t, aux.UnmarshalCBOR(raw))
		md, err := aux.Metadata()
		require.NoError(t, err)
		requirePairs(t, md)
	})

	t.Run("Alonzo", func(t *testing.T) {
		t.Parallel()
		// #6.259({0: metadata_map})
		raw, err := hex.DecodeString("d90103a100" + nestedDuplicateKeyHex)
		require.NoError(t, err)
		var aux AlonzoAuxiliaryData
		require.NoError(t, aux.UnmarshalCBOR(raw))
		md, err := aux.Metadata()
		require.NoError(t, err)
		requirePairs(t, md)
	})
}

// TestOuterAuxiliaryDataLabelMapRejectsDuplicateKeys proves the fix for #2323
// is scoped to the inner, per-label Metadatum value's own nested-map decode.
// The outer Word64-keyed label map is decoded by the general-purpose CBOR
// decoder (cbor.Decode, DupMapKeyEnforcedAPF) via TransactionMetadataSet and
// AlonzoAuxiliaryData, an unrelated code path this fix must not touch.
func TestOuterAuxiliaryDataLabelMapRejectsDuplicateKeys(t *testing.T) {
	t.Parallel()

	t.Run("TransactionMetadataSet", func(t *testing.T) {
		t.Parallel()
		// {1: 0, 1: 0} - duplicate outer label 1.
		raw, err := hex.DecodeString("a201000100")
		require.NoError(t, err)
		var set TransactionMetadataSet
		_, err = cbor.Decode(raw, &set)
		require.Error(t, err)
		require.True(
			t,
			cbor.IsDuplicateMapKeyError(err),
			"expected a duplicate map key error, got %v",
			err,
		)
	})

	t.Run("AlonzoAuxiliaryData", func(t *testing.T) {
		t.Parallel()
		// #6.259({0: {}, 0: {}}) - duplicate outer key 0 in the tagged aux map,
		// bypassing the decodeAuxiliaryMetadataOnly fast path (2 entries).
		raw, err := hex.DecodeString("d90103a200a000a000")
		require.NoError(t, err)
		var aux AlonzoAuxiliaryData
		err = aux.UnmarshalCBOR(raw)
		require.Error(t, err)
		require.ErrorContains(t, err, "duplicate auxiliary-data field 0")
	})
}

func assertMetadataEntry(t *testing.T, md TransactionMetadatum) {
	t.Helper()

	if !assert.IsType(t, MetaMap{}, md) {
		return
	}
	mm := md.(MetaMap)
	if !assert.Len(t, mm.Pairs, 1) {
		return
	}

	if !assert.IsType(t, MetaInt{}, mm.Pairs[0].Key) {
		return
	}
	key := mm.Pairs[0].Key.(MetaInt)
	if assert.True(t, key.Value != nil) {
		assert.Equal(t, uint64(1), key.Value.Uint64())
	}
	if !assert.IsType(t, MetaText{}, mm.Pairs[0].Value) {
		return
	}
	value := mm.Pairs[0].Value.(MetaText)
	assert.Equal(t, "ok", value.Value)
}
