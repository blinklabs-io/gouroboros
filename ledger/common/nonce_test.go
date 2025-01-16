// Copyright 2024 Blink Labs Software
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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestNonceUnmarshalCBOR(t *testing.T) {
	testCases := []struct {
		name        string
		data        []byte
		expectedErr string
	}{
		{
			name: "NonceTypeNeutral",
			data: []byte{0x81, 0x00},
		},
		{
			name: "NonceTypeNonce",
			data: []byte{0x82, 0x01, 0x42, 0x01, 0x02},
		},
		{
			name:        "UnsupportedNonceType",
			data:        []byte{0x82, 0x02},
			expectedErr: "unsupported nonce type 2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			n := &common.Nonce{}
			err := n.UnmarshalCBOR(tc.data)
			if err != nil {
				if tc.expectedErr == "" || err.Error() != tc.expectedErr {
					t.Errorf("unexpected error: %v", err)
				}
			} else if tc.expectedErr != "" {
				t.Errorf("expected error: %v, got nil", tc.expectedErr)
			}
		})
	}
}

func TestCalculateRollingNonce(t *testing.T) {
	testDefs := []struct {
		prevBlockNonce string
		blockVrf       string
		expectedNonce  string
	}{
		{
			// Shelley genesis hash (mainnet)
			prevBlockNonce: "1a3be38bcbb7911969283716ad7aa550250226b76a61fc51cc9a9a35d9276d81",
			blockVrf:       "36ec5378d1f5041a59eb8d96e61de96f0950fb41b49ff511f7bc7fd109d4383e1d24be7034e6749c6612700dd5ceb0c66577b88a19ae286b1321d15bce1ab736",
			expectedNonce:  "2af15f57076a8ff225746624882a77c8d2736fe41d3db70154a22b50af851246",
		},
		{
			blockVrf:      "e0bf34a6b73481302f22987cde4c12807cbc2c3fea3f7fcb77261385a50e8ccdda3226db3efff73e9fb15eecf841bbc85ce37550de0435ebcdcb205e0ed08467",
			expectedNonce: "a815ff978369b57df09b0072485c26920dc0ec8e924a852a42f0715981cf0042",
		},
		{
			blockVrf:      "7107ef8c16058b09f4489715297e55d145a45fc0df75dfb419cab079cd28992854a034ad9dc4c764544fb70badd30a9611a942a03523c6f3d8967cf680c4ca6b",
			expectedNonce: "f112d91435b911b6b5acaf27198762905b1cdec8c5a7b712f925ce3c5c76bb5f",
		},
		{
			blockVrf:      "6f561aad83884ee0d7b19fd3d757c6af096bfd085465d1290b13a9dfc817dfcdfb0b59ca06300206c64d1ba75fd222a88ea03c54fbbd5d320b4fbcf1c228ba4e",
			expectedNonce: "5450d95d9be4194a0ded40fbb4036b48d1f1d6da796e933fefd2c5c888794b4b",
		},
		{
			blockVrf:      "3d3ba80724db0a028783afa56a85d684ee778ae45b9aa9af3120f5e1847be1983bd4868caf97fcfd82d5a3b0b7c1a6d53491d75440a75198014eb4e707785cad",
			expectedNonce: "c5c0f406cb522ad3fead4ecc60bce9c31e80879bc17eb1bb9acaa9b998cdf8bf",
		},
		{
			blockVrf:      "0b07976bc04321c2e7ba0f1acb3c61bd92b5fc780a855632e30e6746ab4ac4081490d816928762debd3e512d22ad512a558612adc569718df1784261f5c26aff",
			expectedNonce: "5857048c728580549de645e087ba20ef20bb7c51cc84b5bc89df6b8b0ed98c41",
		},
		{
			blockVrf:      "5e9e001fb1e2ddb0dc7ff40af917ecf4ba9892491d4bcbf2c81db2efc57627d40d7aac509c9bcf5070d4966faaeb84fd76bb285af2e51af21a8c024089f598c1",
			expectedNonce: "d6f40ef403687115db061b2cb9b1ab4ddeb98222075d5a3e03c8d217d4d7c40e",
		},
		{
			blockVrf:      "182e83f8c67ad2e6bddead128e7108499ebcbc272b50c42783ef08f035aa688fecc7d15be15a90dbfe7fe5d7cd9926987b6ec12b05f2eadfe0eb6cad5130aca4",
			expectedNonce: "5489d75a9f4971c1824462b5e2338609a91f121241f21fee09811bd5772ae0a8",
		},
		{
			blockVrf:      "275e7404b2385a9d606d67d0e29f5516fb84c1c14aaaf91afa9a9b3dcdfe09075efdadbaf158cfa1e9f250cc7c691ed2db4a29288d2426bd74a371a2a4b91b57",
			expectedNonce: "04716326833ecdb595153adac9566a4b39e5c16e8d02526cb4166e4099a00b1a",
		},
		{
			blockVrf:      "0f35c7217792f8b0cbb721ae4ae5c9ae7f2869df49a3db256aacc10d23997a09e0273261b44ebbcecd6bf916f2c1cd79cf25b0c2851645d75dd0747a8f6f92f5",
			expectedNonce: "39db709f50c8a279f0a94adcefb9360dbda6cdce168aed4288329a9cd53492b6",
		},
		{
			blockVrf:      "14c28bf9b10421e9f90ffc9ab05df0dc8c8a07ffac1c51725fba7e2b7972d0769baea248f93ed0f2067d11d719c2858c62fc1d8d59927b41d4c0fbc68d805b32",
			expectedNonce: "c784b8c8678e0a04748a3ad851dd7c34ed67141cd9dc0c50ceaff4df804699a7",
		},
		{
			blockVrf:      "e4ce96fee9deb9378a107db48587438cddf8e20a69e21e5e4fbd35ef0c56530df77eba666cb152812111ba66bbd333ed44f627c727115f8f4f15b31726049a19",
			expectedNonce: "cc1a5861358c075de93a26a91c5a951d5e71190d569aa2dc786d4ca8fc80cc38",
		},
		{
			blockVrf:      "b38f315e3ce369ea2551bf4f44e723dd15c7d67ba4b3763997909f65e46267d6540b9b00a7a65ae3d1f3a3316e57a821aeaac33e4e42ded415205073134cd185",
			expectedNonce: "514979c89313c49e8f59fb8445113fa7623e99375cc4917fe79df54f8d4bdfce",
		},
		{
			blockVrf:      "4bcbf774af9c8ff24d4d96099001ec06a24802c88fea81680ea2411392d32dbd9b9828a690a462954b894708d511124a2db34ec4179841e07a897169f0f1ac0e",
			expectedNonce: "6a783e04481b9e04e8f3498a3b74c90c06a1031fb663b6793ce592a6c26f56f4",
		},
		{
			blockVrf:      "65247ace6355f978a12235265410c44f3ded02849ec8f8e6db2ac705c3f57d322ea073c13cf698e15d7e1d7f2bc95e7b3533be0dee26f58864f1664df0c1ebba",
			expectedNonce: "1190f5254599dcee4f3cf1afdf4181085c36a6db6c30f334bfe6e6f320a6ed91",
		},
	}
	var rollingNonce []byte
	for _, testDef := range testDefs {
		// Populate initial nonce
		if testDef.prevBlockNonce != "" {
			tmpNonce, err := hex.DecodeString(testDef.prevBlockNonce)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			rollingNonce = tmpNonce
		}
		blockVrfBytes, err := hex.DecodeString(testDef.blockVrf)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		nonce, err := common.CalculateRollingNonce(rollingNonce, blockVrfBytes)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		nonceHex := hex.EncodeToString(nonce.Bytes())
		if nonceHex != testDef.expectedNonce {
			t.Fatalf("did not get expected nonce value: got %s, wanted %s", nonceHex, testDef.expectedNonce)
		}
		rollingNonce = nonce.Bytes()
	}
}

func TestCalculateEpochNonce(t *testing.T) {
	testDefs := []struct {
		stableBlockNonce        string
		prevEpochFirstBlockHash string
		extraEntropy            string
		expectedNonce           string
	}{
		{
			stableBlockNonce:        "e86e133bd48ff5e79bec43af1ac3e348b539172f33e502d2c96735e8c51bd04d",
			prevEpochFirstBlockHash: "d7a1ff2a365abed59c9ae346cba842b6d3df06d055dba79a113e0704b44cc3e9",
			expectedNonce:           "e536a0081ddd6d19786e9d708a85819a5c3492c0da7349f59c8ad3e17e4acd98",
		},
		{
			stableBlockNonce:        "d1340a9c1491f0face38d41fd5c82953d0eb48320d65e952414a0c5ebaf87587",
			prevEpochFirstBlockHash: "ee91d679b0a6ce3015b894c575c799e971efac35c7a8cbdc2b3f579005e69abd",
			extraEntropy:            "d982e06fd33e7440b43cefad529b7ecafbaa255e38178ad4189a37e4ce9bf1fa",
			expectedNonce:           "0022cfa563a5328c4fb5c8017121329e964c26ade5d167b1bd9b2ec967772b60",
		},
	}
	for _, testDef := range testDefs {
		stableBlockNonce, err := hex.DecodeString(testDef.stableBlockNonce)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		prevEpochFirstBlockHash, err := hex.DecodeString(testDef.prevEpochFirstBlockHash)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var extraEntropy []byte
		if testDef.extraEntropy != "" {
			tmpEntropy, err := hex.DecodeString(testDef.extraEntropy)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			extraEntropy = tmpEntropy
		}
		tmpNonce, err := common.CalculateEpochNonce(stableBlockNonce, prevEpochFirstBlockHash, extraEntropy)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpNonceHex := hex.EncodeToString(tmpNonce.Bytes())
		if tmpNonceHex != testDef.expectedNonce {
			t.Fatalf("did not get expected epoch nonce: got %s, wanted %s", tmpNonceHex, testDef.expectedNonce)
		}
	}
}
