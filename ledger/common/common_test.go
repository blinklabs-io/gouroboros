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

package common

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

func TestMultiAssetCompare(t *testing.T) {
	testDefs := []struct {
		asset1         *MultiAsset[MultiAssetTypeOutput]
		asset2         *MultiAsset[MultiAssetTypeOutput]
		expectedResult bool
	}{
		{
			asset1: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): 123,
					},
				},
			},
			asset2: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): 123,
					},
				},
			},
			expectedResult: true,
		},
		{
			asset1: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): 123,
					},
				},
			},
			asset2: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): 124,
					},
				},
			},
			expectedResult: false,
		},
		{
			asset1: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): 0,
					},
				},
			},
			asset2:         nil,
			expectedResult: true,
		},
		{
			asset1: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): 123,
					},
				},
			},
			asset2: &MultiAsset[MultiAssetTypeOutput]{
				data: map[Blake2b224]map[cbor.ByteString]MultiAssetTypeOutput{
					NewBlake2b224([]byte("abcd")): {
						cbor.NewByteString([]byte("cdef")): 123,
						cbor.NewByteString([]byte("efgh")): 123,
					},
				},
			},
			expectedResult: false,
		},
	}
	for _, testDef := range testDefs {
		tmpResult := testDef.asset1.Compare(testDef.asset2)
		if tmpResult != testDef.expectedResult {
			t.Errorf(
				"did not get expected result: got %v, wanted %v",
				tmpResult,
				testDef.expectedResult,
			)
		}
	}
}

// Test the MarshalJSON method for Blake2b224 to ensure it properly converts to JSON.
func TestBlake2b224_MarshalJSON(t *testing.T) {
	// Example data to represent Blake2b224 hash
	data := []byte("blinklabs")
	hash := NewBlake2b224(data)

	// Blake2b224 always produces 28 bytes (224 bits) as its output.
	// Expected JSON value: the hex-encoded string of "blinklabs" padded to fit 28 bytes.
	// JSON marshalling adds quotes around the string, so include quotes in expected value.
	expected := `"626c696e6b6c61627300000000000000000000000000000000000000"`

	// Marshal the Blake2b224 object to JSON
	jsonData, err := json.Marshal(hash)
	if err != nil {
		t.Fatalf("failed to marshal Blake2b224: %v", err)
	}

	// Compare the result with the expected output
	if string(jsonData) != expected {
		t.Errorf("expected %s but got %s", expected, string(jsonData))
	}
}

func TestBlake2b224_String(t *testing.T) {
	data := []byte("blinklabs") // Less than 28 bytes
	hash := NewBlake2b224(data)

	// Expected hex string for "blinklabs" padded/truncated to fit 28 bytes
	expected := "626c696e6b6c61627300000000000000000000000000000000000000"

	// Verify if String() gives the correct hex-encoded string
	if hash.String() != expected {
		t.Errorf("expected %s but got %s", expected, hash.String())
	}
}

func TestCertificateTypeMethods(t *testing.T) {
	tests := []struct {
		name     string
		cert     Certificate
		expected uint
	}{
		{
			"StakeRegistration",
			&StakeRegistrationCertificate{
				CertType: CertificateTypeStakeRegistration,
			},
			CertificateTypeStakeRegistration,
		},
		{
			"StakeDeregistration",
			&StakeDeregistrationCertificate{
				CertType: CertificateTypeStakeDeregistration,
			},
			CertificateTypeStakeDeregistration,
		},
		{
			"StakeDelegation",
			&StakeDelegationCertificate{
				CertType: CertificateTypeStakeDelegation,
			},
			CertificateTypeStakeDelegation,
		},
		{
			"PoolRegistration",
			&PoolRegistrationCertificate{
				CertType: CertificateTypePoolRegistration,
			},
			CertificateTypePoolRegistration,
		},
		{
			"PoolRetirement",
			&PoolRetirementCertificate{CertType: CertificateTypePoolRetirement},
			CertificateTypePoolRetirement,
		},
		{
			"GenesisKeyDelegation",
			&GenesisKeyDelegationCertificate{
				CertType: CertificateTypeGenesisKeyDelegation,
			},
			CertificateTypeGenesisKeyDelegation,
		},
		{
			"MoveInstantaneousRewards",
			&MoveInstantaneousRewardsCertificate{
				CertType: CertificateTypeMoveInstantaneousRewards,
			},
			CertificateTypeMoveInstantaneousRewards,
		},
		{
			"Registration",
			&RegistrationCertificate{CertType: CertificateTypeRegistration},
			CertificateTypeRegistration,
		},
		{
			"Deregistration",
			&DeregistrationCertificate{CertType: CertificateTypeDeregistration},
			CertificateTypeDeregistration,
		},
		{
			"VoteDelegation",
			&VoteDelegationCertificate{CertType: CertificateTypeVoteDelegation},
			CertificateTypeVoteDelegation,
		},
		{
			"StakeVoteDelegation",
			&StakeVoteDelegationCertificate{
				CertType: CertificateTypeStakeVoteDelegation,
			},
			CertificateTypeStakeVoteDelegation,
		},
		{
			"StakeRegistrationDelegation",
			&StakeRegistrationDelegationCertificate{
				CertType: CertificateTypeStakeRegistrationDelegation,
			},
			CertificateTypeStakeRegistrationDelegation,
		},
		{
			"VoteRegistrationDelegation",
			&VoteRegistrationDelegationCertificate{
				CertType: CertificateTypeVoteRegistrationDelegation,
			},
			CertificateTypeVoteRegistrationDelegation,
		},
		{
			"StakeVoteRegistrationDelegation",
			&StakeVoteRegistrationDelegationCertificate{
				CertType: CertificateTypeStakeVoteRegistrationDelegation,
			},
			CertificateTypeStakeVoteRegistrationDelegation,
		},
		{
			"AuthCommitteeHot",
			&AuthCommitteeHotCertificate{
				CertType: CertificateTypeAuthCommitteeHot,
			},
			CertificateTypeAuthCommitteeHot,
		},
		{
			"ResignCommitteeCold",
			&ResignCommitteeColdCertificate{
				CertType: CertificateTypeResignCommitteeCold,
			},
			CertificateTypeResignCommitteeCold,
		},
		{
			"RegistrationDrep",
			&RegistrationDrepCertificate{
				CertType: CertificateTypeRegistrationDrep,
			},
			CertificateTypeRegistrationDrep,
		},
		{
			"DeregistrationDrep",
			&DeregistrationDrepCertificate{
				CertType: CertificateTypeDeregistrationDrep,
			},
			CertificateTypeDeregistrationDrep,
		},
		{
			"UpdateDrep",
			&UpdateDrepCertificate{CertType: CertificateTypeUpdateDrep},
			CertificateTypeUpdateDrep,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cert.Type()
			if got != tt.expected {
				t.Errorf(
					"FAIL: %s -> Type() = %d, expected = %d",
					tt.name,
					got,
					tt.expected,
				)
			}
		})
	}
}
