package common

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
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
		t.Fatalf("mismatch:\n got: %s\nwant: %s", hex.EncodeToString(enc), allegraBlockHex)
	}
}
