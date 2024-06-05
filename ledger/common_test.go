package ledger

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
)

func TestAssetFingerprint(t *testing.T) {
	testDefs := []struct {
		policyIdHex         string
		assetNameHex        string
		expectedFingerprint string
	}{
		// NOTE: these test defs were created from a random sampling of recent assets on cexplorer.io
		{
			policyIdHex:         "29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61",
			assetNameHex:        "6675726e697368613239686e",
			expectedFingerprint: "asset1jdu2xcrwlqsjqqjger6kj2szddz8dcpvcg4ksz",
		},
		{
			policyIdHex:         "eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5",
			assetNameHex:        "426f7764757261436f6e63657074733638",
			expectedFingerprint: "asset1kp7hdhqc7chmyqvtqrsljfdrdt6jz8mg5culpe",
		},
		{
			policyIdHex:         "cf78aeb9736e8aa94ce8fab44da86b522fa9b1c56336b92a28420525",
			assetNameHex:        "363438346330393264363164373033656236333233346461",
			expectedFingerprint: "asset1rx3cnlsvh3udka56wyqyed3u695zd5q2jck2yd",
		},
	}
	for _, test := range testDefs {
		policyIdBytes, err := hex.DecodeString(test.policyIdHex)
		if err != nil {
			t.Fatalf("failed to decode policy ID hex: %s", err)
		}
		assetNameBytes, err := hex.DecodeString(test.assetNameHex)
		if err != nil {
			t.Fatalf("failed to decode asset name hex: %s", err)
		}
		fp := NewAssetFingerprint(policyIdBytes, assetNameBytes)
		if fp.String() != test.expectedFingerprint {
			t.Fatalf(
				"asset fingerprint did not match expected value, got: %s, wanted: %s",
				fp.String(),
				test.expectedFingerprint,
			)
		}
	}
}

func TestMultiAssetJson(t *testing.T) {
	testDefs := []struct {
		multiAssetObj interface{}
		expectedJson  string
	}{
		{
			multiAssetObj: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61")): {
						cbor.NewByteString(test.DecodeHexString("6675726e697368613239686e")): 123456,
					},
				},
			},
			expectedJson: `[{"name":"furnisha29hn","nameHex":"6675726e697368613239686e","policyId":"29a8fb8318718bd756124f0c144f56d4b4579dc5edf2dd42d669ac61","fingerprint":"asset1jdu2xcrwlqsjqqjger6kj2szddz8dcpvcg4ksz","amount":123456}]`,
		},
		{
			multiAssetObj: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5")): {
						cbor.NewByteString(test.DecodeHexString("426f7764757261436f6e63657074733638")): 234567,
					},
				},
			},
			expectedJson: `[{"name":"BowduraConcepts68","nameHex":"426f7764757261436f6e63657074733638","policyId":"eaf8042c1d8203b1c585822f54ec32c4c1bb4d3914603e2cca20bbd5","fingerprint":"asset1kp7hdhqc7chmyqvtqrsljfdrdt6jz8mg5culpe","amount":234567}]`,
		},
		{
			multiAssetObj: MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224(test.DecodeHexString("cf78aeb9736e8aa94ce8fab44da86b522fa9b1c56336b92a28420525")): {
						cbor.NewByteString(test.DecodeHexString("363438346330393264363164373033656236333233346461")): 12345678,
					},
				},
			},
			expectedJson: `[{"name":"6484c092d61d703eb63234da","nameHex":"363438346330393264363164373033656236333233346461","policyId":"cf78aeb9736e8aa94ce8fab44da86b522fa9b1c56336b92a28420525","fingerprint":"asset1rx3cnlsvh3udka56wyqyed3u695zd5q2jck2yd","amount":12345678}]`,
		},
	}
	for _, test := range testDefs {
		var err error
		var jsonData []byte
		switch v := test.multiAssetObj.(type) {
		case MultiAsset[MultiAssetTypeOutput]:
			jsonData, err = json.Marshal(&v)
		case MultiAsset[MultiAssetTypeMint]:
			jsonData, err = json.Marshal(&v)
		default:
			t.Fatalf("unexpected test object type: %T", test.multiAssetObj)
		}
		if err != nil {
			t.Fatalf("failed to marshal MultiAsset object into JSON: %s", err)
		}
		if string(jsonData) != test.expectedJson {
			t.Fatalf(
				"MultiAsset object did not marshal into expected JSON\n  got: %s\n  wanted: %s",
				jsonData,
				test.expectedJson,
			)
		}
	}
}
