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
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/assert"
)

func TestAddressFromBytes(t *testing.T) {
	testDefs := []struct {
		addressBytesHex string
		expectedAddress string
	}{
		{
			addressBytesHex: "11e1317b152faac13426e6a83e06ff88a4d62cce3c1634ab0a5ec1330952563c5410bff6a0d43ccebb7c37e1f69f5eb260552521adff33b9c2",
			expectedAddress: "addr1z8snz7c4974vzdpxu65ruphl3zjdvtxw8strf2c2tmqnxz2j2c79gy9l76sdg0xwhd7r0c0kna0tycz4y5s6mlenh8pq0xmsha",
		},
		{
			addressBytesHex: "013f35615835258addded1c2e169f3a2ab4ae94d606bde030e7947f5184ff5f8e3d43ce6b19ec4197e331e86d0f5e58b02d7a75b5e74cff95d",
			expectedAddress: "addr1qyln2c2cx5jc4hw768pwz60n5245462dvp4auqcw09rl2xz07huw84puu6cea3qe0ce3apks7hjckqkh5ad4uax0l9ws0q9xty",
		},
		{
			addressBytesHex: "7121bd8c2e0df2fbe92137f78dbaba48f62308e52303049f0d628b6c4c",
			expectedAddress: "addr1wysmmrpwphe0h6fpxlmcmw46frmzxz89yvpsf8cdv29kcnqsw3vw6",
		},
		{
			addressBytesHex: "61cfe224295a282d69edda5fa8de4f131e2b9cd21a6c9235597fa4ff6b",
			expectedAddress: "addr1v887yfpftg5z660dmf063hj0zv0zh8xjrfkfyd2e07j076cecha5k",
		},
		// Long (but apparently valid) address from:
		// https://github.com/IntersectMBO/cardano-ledger/issues/2729
		{
			addressBytesHex: "015bad085057ac10ecc7060f7ac41edd6f63068d8963ef7d86ca58669e5ecf2d283418a60be5a848a2380eb721000da1e0bbf39733134beca4cb57afb0b35fc89c63061c9914e055001a518c7516",
			expectedAddress: "addr1q9d66zzs27kppmx8qc8h43q7m4hkxp5d39377lvxefvxd8j7eukjsdqc5c97t2zg5guqadepqqx6rc9m7wtnxy6tajjvk4a0kze4ljyuvvrpexg5up2sqxj33363v35gtew",
		},
		// Another long (but apparently valid) address seen in the wild
		{
			addressBytesHex: "61549b5a20e449a3e394b762705f64b9a26b99013003a2bfdba239967c00",
			expectedAddress: "addr1v92fkk3qu3y68cu5ka38qhmyhx3xhxgpxqp6907m5guevlqqjd7xgj",
		},
		// Byron address, mainnet with derivation
		{
			addressBytesHex: "82d818584283581caf56de241bcca83d72c51e74d18487aa5bc68b45e2caa170fa329d3aa101581e581cea1425ccdd649b25af5deb7e6335da2eb8167353a55e77925122e95f001a3a858621",
			expectedAddress: "DdzFFzCqrht2ii4Vc7KRchSkVvQtCqdGkQt4nF4Yxg1NpsubFBity2Tpt2eSEGrxBH1eva8qCFKM2Y5QkwM1SFBizRwZgz1N452WYvgG",
		},
		// Byron address, preview
		{
			addressBytesHex: "82d818582483581c5d5e698eba3dd9452add99a1af9461beb0ba61b8bece26e7399878dda1024102001a36d41aba",
			expectedAddress: "FHnt4NL7yPXvDWHa8bVs73UEUdJd64VxWXSFNqetECtYfTd9TtJguJ14Lu3feth",
		},
	}
	for _, testDef := range testDefs {
		addr, err := NewAddressFromBytes(
			test.DecodeHexString(testDef.addressBytesHex),
		)
		if err != nil {
			t.Fatalf(
				"failure populating address from bytes: %s",
				err,
			)
		}
		if addr.String() != testDef.expectedAddress {
			t.Fatalf(
				"address did not match expected value, got: %s, wanted: %s",
				addr.String(),
				testDef.expectedAddress,
			)
		}
	}
}

func TestAddressFromParts(t *testing.T) {
	// We can't use our network helpers due to an import cycle
	var networkMainnetId uint8 = 1
	testDefs := []struct {
		addressType     uint8
		networkId       uint8
		paymentAddr     []byte
		stakingAddr     []byte
		expectedAddress string
	}{
		{
			addressType: AddressTypeScriptKey,
			networkId:   networkMainnetId,
			paymentAddr: test.DecodeHexString(
				"e1317b152faac13426e6a83e06ff88a4d62cce3c1634ab0a5ec13309",
			),
			stakingAddr: test.DecodeHexString(
				"52563c5410bff6a0d43ccebb7c37e1f69f5eb260552521adff33b9c2",
			),
			expectedAddress: "addr1z8snz7c4974vzdpxu65ruphl3zjdvtxw8strf2c2tmqnxz2j2c79gy9l76sdg0xwhd7r0c0kna0tycz4y5s6mlenh8pq0xmsha",
		},
		{
			addressType: AddressTypeKeyKey,
			networkId:   networkMainnetId,
			paymentAddr: test.DecodeHexString(
				"3f35615835258addded1c2e169f3a2ab4ae94d606bde030e7947f518",
			),
			stakingAddr: test.DecodeHexString(
				"4ff5f8e3d43ce6b19ec4197e331e86d0f5e58b02d7a75b5e74cff95d",
			),
			expectedAddress: "addr1qyln2c2cx5jc4hw768pwz60n5245462dvp4auqcw09rl2xz07huw84puu6cea3qe0ce3apks7hjckqkh5ad4uax0l9ws0q9xty",
		},
		{
			addressType: AddressTypeScriptNone,
			networkId:   networkMainnetId,
			paymentAddr: test.DecodeHexString(
				"21bd8c2e0df2fbe92137f78dbaba48f62308e52303049f0d628b6c4c",
			),
			expectedAddress: "addr1wysmmrpwphe0h6fpxlmcmw46frmzxz89yvpsf8cdv29kcnqsw3vw6",
		},
		{
			addressType: AddressTypeKeyNone,
			networkId:   networkMainnetId,
			paymentAddr: test.DecodeHexString(
				"cfe224295a282d69edda5fa8de4f131e2b9cd21a6c9235597fa4ff6b",
			),
			expectedAddress: "addr1v887yfpftg5z660dmf063hj0zv0zh8xjrfkfyd2e07j076cecha5k",
		},
	}
	for _, testDef := range testDefs {
		addr, err := NewAddressFromParts(
			testDef.addressType,
			testDef.networkId,
			testDef.paymentAddr,
			testDef.stakingAddr,
		)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if addr.String() != testDef.expectedAddress {
			t.Fatalf(
				"address did not match expected value, got: %s, wanted: %s",
				addr.String(),
				testDef.expectedAddress,
			)
		}
	}
}

func TestByronAddressFromParts(t *testing.T) {
	testDefs := []struct {
		addressType     uint64
		paymentAddr     []byte
		addressAttr     ByronAddressAttributes
		expectedAddress string
	}{
		// Byron address, mainnet with derivation
		{
			addressType: ByronAddressTypePubkey,
			paymentAddr: test.DecodeHexString(
				"af56de241bcca83d72c51e74d18487aa5bc68b45e2caa170fa329d3a",
			),
			addressAttr: ByronAddressAttributes{
				Payload: test.DecodeHexString(
					"581cea1425ccdd649b25af5deb7e6335da2eb8167353a55e77925122e95f",
				),
			},
			expectedAddress: "DdzFFzCqrht2ii4Vc7KRchSkVvQtCqdGkQt4nF4Yxg1NpsubFBity2Tpt2eSEGrxBH1eva8qCFKM2Y5QkwM1SFBizRwZgz1N452WYvgG",
		},
		// Byron address, preview
		{
			addressType: ByronAddressTypePubkey,
			paymentAddr: test.DecodeHexString(
				"5d5e698eba3dd9452add99a1af9461beb0ba61b8bece26e7399878dd",
			),
			addressAttr: ByronAddressAttributes{
				// We have to jump through this hoop to get an inline pointer to a uint32
				Network: func() *uint32 { ret := uint32(2); return &ret }(),
			},
			expectedAddress: "FHnt4NL7yPXvDWHa8bVs73UEUdJd64VxWXSFNqetECtYfTd9TtJguJ14Lu3feth",
		},
	}
	for _, testDef := range testDefs {
		addr, err := NewByronAddressFromParts(
			testDef.addressType,
			testDef.paymentAddr,
			testDef.addressAttr,
		)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if addr.String() != testDef.expectedAddress {
			t.Fatalf(
				"address did not match expected value, got: %s, wanted: %s",
				addr.String(),
				testDef.expectedAddress,
			)
		}
	}
}

func TestAddressPaymentAddress(t *testing.T) {
	testDefs := []struct {
		address                string
		expectedPaymentAddress string
	}{
		{
			address:                "addr_test1qqawz5hm2tchtmarkfn2tamzvd2spatl89gtutgra6zwc3ktqj7p944ckc9lq7u36jrq99znwhzlq6jfv2j4ql92m4rq07hp8t",
			expectedPaymentAddress: "addr_test1vqawz5hm2tchtmarkfn2tamzvd2spatl89gtutgra6zwc3s5t5a7q",
		},
		{
			address:                "addr_test1vpmwd5tk8quxnzxq46h8vztf00xtphrd7zd0al5ur5jsylg3r9v4l",
			expectedPaymentAddress: "addr_test1vpmwd5tk8quxnzxq46h8vztf00xtphrd7zd0al5ur5jsylg3r9v4l",
		},
		// Script address with script staking key
		{
			address:                "addr1x8nz307k3sr60gu0e47cmajssy4fmld7u493a4xztjrll0aj764lvrxdayh2ux30fl0ktuh27csgmpevdu89jlxppvrswgxsta",
			expectedPaymentAddress: "addr1w8nz307k3sr60gu0e47cmajssy4fmld7u493a4xztjrll0cm9703s",
		},
	}
	for _, testDef := range testDefs {
		addr, err := NewAddress(testDef.address)
		if err != nil {
			t.Fatalf("failed to decode address: %s", err)
		}
		if addr.PaymentAddress() == nil {
			t.Fatalf("payment address is nil")
		}
		if addr.PaymentAddress().String() != testDef.expectedPaymentAddress {
			t.Fatalf(
				"payment address did not match expected value, got: %s, wanted: %s",
				addr.PaymentAddress().String(),
				testDef.expectedPaymentAddress,
			)
		}
	}
}

func TestAddressStakeAddress(t *testing.T) {
	testDefs := []struct {
		address              string
		expectedStakeAddress string
	}{
		{
			address:              "addr1q8fv95d4g2599v3gzq7wnva34ykt4d2zerl0wyke36zml0neqj84x95mgp694rv8gfqy6u67ms38lx30texma843yd5qmvkqcz",
			expectedStakeAddress: "stake1u9usfr6nz6d5qaz63kr5yszdwd0dcgnlngh4und7n6cjx6qh02h9m",
		},
		{
			address:              "addr1q8uas4shlrnxhd8dnqxesk9hlgmtx65xlmkq9c7acfa2ksvjn4k4w8d7lwnzkf3dkq26kxz3re50h89adduskx08rr6qq2g0f2",
			expectedStakeAddress: "stake1uxff6m2hrkl0hf3tyckmq9dtrpg3u68mnj7kk7gtr8n33aq5fqskz",
		},
		{
			address:              "addr1q9h4f2vhh5vnqgnsejan3psw6mj3a504fxlqm2eh3262qufesdvfs83ulr22vprsv9mwnt0vgkfwxlflxkns32twqzdqjpq2na",
			expectedStakeAddress: "stake1uyucxkycrc70349xq3cxzahf4hkytyhr05lntfcg49hqpxsqlrayk",
		},
		// Script address
		{
			address:              "addr1z8snz7c4974vzdpxu65ruphl3zjdvtxw8strf2c2tmqnxz2j2c79gy9l76sdg0xwhd7r0c0kna0tycz4y5s6mlenh8pq0xmsha",
			expectedStakeAddress: "stake1u9f9v0z5zzlldgx58n8tklphu8mf7h4jvp2j2gddluemnssjfnkzz",
		},
	}
	for _, testDef := range testDefs {
		addr, err := NewAddress(testDef.address)
		if err != nil {
			t.Fatalf("failed to decode address: %s", err)
		}
		if addr.StakeAddress() == nil {
			t.Fatalf("stake address is nil")
		}
		if addr.StakeAddress().String() != testDef.expectedStakeAddress {
			t.Fatalf(
				"stake address did not match expected value, got: %s, wanted: %s",
				addr.StakeAddress().String(),
				testDef.expectedStakeAddress,
			)
		}
	}
}

func TestAddressPaymentAddress_MixedCase(t *testing.T) {
	// address with mixed case Byron address
	mixedCaseAddress := "Ae2tdPwUPEYwFx4dmJheyNPPYXtvHbJLeCaA96o6Y2iiUL18cAt7AizN2zG"
	addr, err := NewAddress(mixedCaseAddress)
	assert.Nil(t, err, "Expected no error when decoding a mixed-case address")
	assert.NotNil(t, addr, "Expected a valid address object after decoding")
}

func TestAddressNetworkId(t *testing.T) {
	testDefs := []struct {
		address           string
		expectedNetworkId uint
	}{
		{
			address:           "addr_test1wpvdxve27gk4ylwyf7t6xn3c7swrfzwz9uv0akwnpctkc4qdhgye0",
			expectedNetworkId: AddressNetworkTestnet,
		},
		{
			address:           "FHnt4NL7yPXsabNmoHMHCfzUVkAC1vSZKd3fgyPfvRGhoXdum5oadfcrADWF8Fc",
			expectedNetworkId: AddressNetworkTestnet,
		},
		{
			address:           "addr1q862w5ru0hpxl4r6vezgtegrfqve0dm2dp3yj2f7y4arrf223wd3fr6qcumc6873am478xnxmfp8lgpe6q6ju9ttjgns2xavze",
			expectedNetworkId: AddressNetworkMainnet,
		},
	}
	for _, testDef := range testDefs {
		addr, err := NewAddress(testDef.address)
		if err != nil {
			t.Fatalf("failed to decode address: %s", err)
		}
		if addr.NetworkId() != testDef.expectedNetworkId {
			t.Fatalf(
				"address did not return expected network ID, got: %d, wanted: %d",
				addr.NetworkId(),
				testDef.expectedNetworkId,
			)
		}
	}
}

func TestAddressToPlutusData(t *testing.T) {
	testDefs := []struct {
		address      string
		expectedData data.PlutusData
	}{
		// Payment-only address
		// This was adapted from the Aiken tests
		{
			address: "addr_test1vqg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zyg3zygxrcya6",
			expectedData: data.NewConstr(
				0,
				data.NewConstr(
					0,
					data.NewByteString(
						[]byte{
							0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
						},
					),
				),
				data.NewConstr(
					1,
				),
			),
		},
		/*
			{
				address: "addr1qyln2c2cx5jc4hw768pwz60n5245462dvp4auqcw09rl2xz07huw84puu6cea3qe0ce3apks7hjckqkh5ad4uax0l9ws0q9xty",
			},
			{
				address: "addr1wysmmrpwphe0h6fpxlmcmw46frmzxz89yvpsf8cdv29kcnqsw3vw6",
			},
			// Script address with script staking key
			{
				address: "addr1x8nz307k3sr60gu0e47cmajssy4fmld7u493a4xztjrll0aj764lvrxdayh2ux30fl0ktuh27csgmpevdu89jlxppvrswgxsta",
			},
		*/
	}
	for _, testDef := range testDefs {
		tmpAddr, err := NewAddress(testDef.address)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpPd := tmpAddr.ToPlutusData()
		if !reflect.DeepEqual(tmpPd, testDef.expectedData) {
			t.Errorf(
				"did not get expected PlutusData\n     got: %#v\n  wanted: %#v",
				tmpPd,
				testDef.expectedData,
			)
		}
	}
}
