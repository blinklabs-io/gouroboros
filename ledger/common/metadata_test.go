package common

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var allegraBlockHex = "a219ef64a301582095b1d64fbf76f17b1920a34d14fbca1f5ab499ea59eac37a8117d5e6b2e09605025820f3157c8eda34976620ad12e0979b2d3135a784c5d6a185878987143053c17d1c035839012c152eaa9e68dd7123a3054190dc987a24e50f1ab389c44a0c7a4089beb4d4d62d8f0dce5d745df4a670998aa20f54703b2bdc7a00b7d3d219ef65a1015840897063bdeab54d2e0586529909f20b42447bfaccdfb9988d2558896baf82a37f43c2fa4ae4240f5761e3dccf9523d7305d728f21dee4491e02373de6b14f7e07"

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

func TestMetadataSetIgnoresUnknownAuxiliaryDataKeys(t *testing.T) {
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

	aux, err := DecodeAuxiliaryData(rawMd)
	require.NoError(t, err)
	assert.Equal(t, auxiliaryDataHex, hex.EncodeToString(aux.Cbor()))
	md, err = aux.Metadata()
	require.NoError(t, err)
	require.NotNil(t, md)
	assertMetadataEntry(t, md)
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
