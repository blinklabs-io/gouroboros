package ledger

import (
	"encoding/hex"
	"testing"
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
			t.Fatalf("asset fingerprint did not match expected value, got: %s, wanted: %s", fp.String(), test.expectedFingerprint)
		}
	}
}
