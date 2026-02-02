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
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		// Shelley address with stake pointer
		{
			addressBytesHex: "40000000000000000000000000000000000000000000000000000000008198bd431b03",
			expectedAddress: "addr_test1gqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqypnz75xxcrsxvt6scmqvvrw720",
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

func TestBech32Roundtrip_MainnetExamples(t *testing.T) {
	examples := []string{
		"addr1z8snz7c4974vzdpxu65ruphl3zjdvtxw8strf2c2tmqnxz2j2c79gy9l76sdg0xwhd7r0c0kna0tycz4y5s6mlenh8pq0xmsha",
		"addr1qyln2c2cx5jc4hw768pwz60n5245462dvp4auqcw09rl2xz07huw84puu6cea3qe0ce3apks7hjckqkh5ad4uax0l9ws0q9xty",
		"addr1wysmmrpwphe0h6fpxlmcmw46frmzxz89yvpsf8cdv29kcnqsw3vw6",
	}

	for _, s := range examples {
		a, err := NewAddress(s)
		assert.Nil(t, err, "should decode example address: %s", s)
		// Roundtrip
		r := a.String()
		assert.Equal(t, s, r, "roundtrip should preserve bech32 string")
	}
}

func TestBech32Roundtrip_TestnetExample(t *testing.T) {
	s := "addr_test1gqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqypnz75xxcrsxvt6scmqvvrw720"
	a, err := NewAddress(s)
	assert.Nil(t, err)
	assert.Equal(t, s, a.String())
}

func TestBech32MalformedInputs(t *testing.T) {
	bad := []string{"", "not-bech32", "addr1xxxxzzzz..."}
	for _, s := range bad {
		_, err := NewAddress(s)
		assert.Error(t, err, "expected error for malformed input: %s", s)
	}
}

func BenchmarkAddressFromBytes(b *testing.B) {
	addrBytes, err := hex.DecodeString(
		"11e1317b152faac13426e6a83e06ff88a4d62cce3c1634ab0a5ec1330952563c5410bff6a0d43ccebb7c37e1f69f5eb260552521adff33b9c2",
	)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_, err := NewAddressFromBytes(addrBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestCIP0019AddressParsing(t *testing.T) {
	// Test vectors from CIP-0019
	testDefs := []struct {
		address         string
		expectedType    uint8
		expectedNetwork uint
	}{
		// Mainnet addresses
		{
			"addr1qx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgse35a3x",
			AddressTypeKeyKey,
			1,
		},
		{
			"addr1z8phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gten0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgs9yc0hh",
			AddressTypeScriptKey,
			1,
		},
		{
			"addr1yx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzerkr0vd4msrxnuwnccdxlhdjar77j6lg0wypcc9uar5d2shs2z78ve",
			AddressTypeKeyScript,
			1,
		},
		{
			"addr1x8phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gt7r0vd4msrxnuwnccdxlhdjar77j6lg0wypcc9uar5d2shskhj42g",
			AddressTypeScriptScript,
			1,
		},
		{
			"addr1gx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer5pnz75xxcrzqf96k",
			AddressTypeKeyPointer,
			1,
		},
		{
			"addr128phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtupnz75xxcrtw79hu",
			AddressTypeScriptPointer,
			1,
		},
		{
			"addr1vx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzers66hrl8",
			AddressTypeKeyNone,
			1,
		},
		{
			"addr1w8phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtcyjy7wx",
			AddressTypeScriptNone,
			1,
		},
		{
			"stake1uyehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gh6ffgw",
			AddressTypeNoneKey,
			1,
		},
		{
			"stake178phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtcccycj5",
			AddressTypeNoneScript,
			1,
		},
		// Testnet addresses
		{
			"addr_test1qz2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgs68faae",
			AddressTypeKeyKey,
			0,
		},
		{
			"addr_test1zrphkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gten0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgsxj90mg",
			AddressTypeScriptKey,
			0,
		},
		{
			"addr_test1yz2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzerkr0vd4msrxnuwnccdxlhdjar77j6lg0wypcc9uar5d2shsf5r8qx",
			AddressTypeKeyScript,
			0,
		},
		{
			"addr_test1xrphkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gt7r0vd4msrxnuwnccdxlhdjar77j6lg0wypcc9uar5d2shs4p04xh",
			AddressTypeScriptScript,
			0,
		},
		{
			"addr_test1gz2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer5pnz75xxcrdw5vky",
			AddressTypeKeyPointer,
			0,
		},
		{
			"addr_test12rphkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtupnz75xxcryqrvmw",
			AddressTypeScriptPointer,
			0,
		},
		{
			"addr_test1vz2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzerspjrlsz",
			AddressTypeKeyNone,
			0,
		},
		{
			"addr_test1wrphkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtcl6szpr",
			AddressTypeScriptNone,
			0,
		},
		{
			"stake_test1uqehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gssrtvn",
			AddressTypeNoneKey,
			0,
		},
		{
			"stake_test17rphkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gtcljw6kf",
			AddressTypeNoneScript,
			0,
		},
	}
	for _, testDef := range testDefs {
		addr, err := NewAddress(testDef.address)
		if err != nil {
			t.Fatalf("failed to parse address %s: %s", testDef.address, err)
		}
		if addr.Type() != testDef.expectedType {
			t.Fatalf(
				"address type mismatch for %s: got %d, wanted %d",
				testDef.address,
				addr.Type(),
				testDef.expectedType,
			)
		}
		if addr.NetworkId() != testDef.expectedNetwork {
			t.Fatalf(
				"network ID mismatch for %s: got %d, wanted %d",
				testDef.address,
				addr.NetworkId(),
				testDef.expectedNetwork,
			)
		}
	}
}

func BenchmarkAddressFromString(b *testing.B) {
	addrStr := "addr1z8snz7c4974vzdpxu65ruphl3zjdvtxw8strf2c2tmqnxz2j2c79gy9l76sdg0xwhd7r0c0kna0tycz4y5s6mlenh8pq0xmsha"
	b.ResetTimer()
	for b.Loop() {
		_, err := NewAddress(addrStr)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func TestAddressCbor(t *testing.T) {
	// Test CBOR encoding/decoding for CIP-0019 address format compliance
	addrStr := "addr1qx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgse35a3x"

	// Parse address from Bech32
	addr, err := NewAddress(addrStr)
	assert.NoError(t, err)

	// Encode to CBOR
	cborData, err := addr.MarshalCBOR()
	assert.NoError(t, err)
	assert.NotEmpty(t, cborData)

	// Decode back from CBOR
	var decoded Address
	err = decoded.UnmarshalCBOR(cborData)
	assert.NoError(t, err)

	// Verify the decoded address matches the original
	assert.Equal(t, addr.String(), decoded.String())
}

func TestAddressBech32_CIP0005(t *testing.T) {
	// Test Bech32 encoding/decoding for CIP-0005 Common Bech32 Prefixes compliance
	testCases := []struct {
		name        string
		address     string
		expectedHRP string
		networkId   uint
		addressType uint8
	}{
		{
			name:        "Mainnet payment address",
			address:     "addr1q9d66zzs27kppmx8qc8h43q7m4hkxp5d39377lvxefvxd8j7eukjsdqc5c97t2zg5guqadepqqx6rc9m7wtnxy6tajjvk4a0kze4ljyuvvrpexg5up2sqxj33363v35gtew",
			expectedHRP: "addr",
			networkId:   1, // mainnet
			addressType: 0, // payment key hash
		},
		{
			name:        "Testnet payment address",
			address:     "addr_test1gqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqypnz75xxcrsxvt6scmqvvrw720",
			expectedHRP: "addr_test",
			networkId:   0, // testnet
			addressType: 4, // key pointer (this address is actually a pointer address)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse address from Bech32 string
			addr, err := NewAddress(tc.address)
			assert.NoError(t, err)
			assert.NotNil(t, addr)

			// Verify the address string matches (round-trip)
			assert.Equal(t, tc.address, addr.String())

			// Verify network ID
			assert.Equal(t, tc.networkId, addr.NetworkId())

			// Verify address type
			assert.Equal(t, tc.addressType, addr.Type())

			// Verify Bech32 HRP prefix matches expected per CIP-0005
			assert.True(
				t,
				strings.HasPrefix(addr.String(), tc.expectedHRP),
				"expected HRP prefix %s",
				tc.expectedHRP,
			)

			// Verify Bech32 encoding/decoding
			// Parse the generated Bech32 string back
			addr2, err := NewAddress(addr.String())
			assert.NoError(t, err)
			assert.Equal(t, addr, addr2)
		})
	}
}

// CIP-0019 Address Edge Case Tests
// These tests verify correct handling of malformed, edge case, and boundary condition addresses
// as specified in CIP-0019 (Cardano Address Format).

func TestCIP0019_MalformedAddressBytes(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectError bool
		errorMsg    string
	}{
		{
			name:        "empty input",
			input:       []byte{},
			expectError: true,
			errorMsg:    "invalid address data: empty byte slice",
		},
		{
			name:        "single byte only (header)",
			input:       []byte{0x01},
			expectError: true,
			errorMsg:    "invalid payment payload",
		},
		{
			name:        "truncated key hash (too short)",
			input:       []byte{0x01, 0x02, 0x03, 0x04},
			expectError: true,
			errorMsg:    "invalid payment payload",
		},
		{
			name:        "truncated type 0 address (27 bytes instead of 57)",
			input:       append([]byte{0x01}, make([]byte, 26)...),
			expectError: true,
			errorMsg:    "invalid payment payload",
		},
		{
			name:        "truncated type 0 address (29 bytes - missing staking)",
			input:       append([]byte{0x01}, make([]byte, 28)...),
			expectError: true,
			errorMsg:    "invalid staking payload",
		},
		{
			name:        "valid type 6 enterprise address (29 bytes total)",
			input:       append([]byte{0x61}, make([]byte, 28)...),
			expectError: false,
		},
		{
			name:        "truncated script hash",
			input:       append([]byte{0x11}, make([]byte, 20)...), // type 1, needs 28 bytes
			expectError: true,
			errorMsg:    "invalid payment payload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAddressFromBytes(tt.input)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCIP0019_AllAddressTypes(t *testing.T) {
	// Test all 8 Shelley address types (0-7) plus reward types (14-15)
	// Create valid test data for each address type
	paymentHash := make([]byte, 28)
	stakingHash := make([]byte, 28)
	for i := 0; i < 28; i++ {
		paymentHash[i] = byte(i)
		stakingHash[i] = byte(i + 28)
	}

	tests := []struct {
		name        string
		addrType    uint8
		networkId   uint8
		hasPayment  bool
		hasStaking  bool
		isPointer   bool
		expectError bool
	}{
		{
			name:       "Type 0: KeyKey (payment key + stake key)",
			addrType:   AddressTypeKeyKey,
			networkId:  AddressNetworkMainnet,
			hasPayment: true,
			hasStaking: true,
		},
		{
			name:       "Type 1: ScriptKey (payment script + stake key)",
			addrType:   AddressTypeScriptKey,
			networkId:  AddressNetworkMainnet,
			hasPayment: true,
			hasStaking: true,
		},
		{
			name:       "Type 2: KeyScript (payment key + stake script)",
			addrType:   AddressTypeKeyScript,
			networkId:  AddressNetworkMainnet,
			hasPayment: true,
			hasStaking: true,
		},
		{
			name:       "Type 3: ScriptScript (payment script + stake script)",
			addrType:   AddressTypeScriptScript,
			networkId:  AddressNetworkMainnet,
			hasPayment: true,
			hasStaking: true,
		},
		{
			name:       "Type 4: KeyPointer (payment key + stake pointer)",
			addrType:   AddressTypeKeyPointer,
			networkId:  AddressNetworkMainnet,
			hasPayment: true,
			hasStaking: true,
			isPointer:  true,
		},
		{
			name:       "Type 5: ScriptPointer (payment script + stake pointer)",
			addrType:   AddressTypeScriptPointer,
			networkId:  AddressNetworkMainnet,
			hasPayment: true,
			hasStaking: true,
			isPointer:  true,
		},
		{
			name:       "Type 6: KeyNone (enterprise key address)",
			addrType:   AddressTypeKeyNone,
			networkId:  AddressNetworkMainnet,
			hasPayment: true,
			hasStaking: false,
		},
		{
			name:       "Type 7: ScriptNone (enterprise script address)",
			addrType:   AddressTypeScriptNone,
			networkId:  AddressNetworkMainnet,
			hasPayment: true,
			hasStaking: false,
		},
		{
			name:       "Type 14: NoneKey (reward key address)",
			addrType:   AddressTypeNoneKey,
			networkId:  AddressNetworkMainnet,
			hasPayment: false,
			hasStaking: true,
		},
		{
			name:       "Type 15: NoneScript (reward script address)",
			addrType:   AddressTypeNoneScript,
			networkId:  AddressNetworkMainnet,
			hasPayment: false,
			hasStaking: true,
		},
		// Testnet variants
		{
			name:       "Type 0: KeyKey (testnet)",
			addrType:   AddressTypeKeyKey,
			networkId:  AddressNetworkTestnet,
			hasPayment: true,
			hasStaking: true,
		},
		{
			name:       "Type 6: KeyNone (testnet)",
			addrType:   AddressTypeKeyNone,
			networkId:  AddressNetworkTestnet,
			hasPayment: true,
			hasStaking: false,
		},
		{
			name:       "Type 14: NoneKey (testnet)",
			addrType:   AddressTypeNoneKey,
			networkId:  AddressNetworkTestnet,
			hasPayment: false,
			hasStaking: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var addr Address
			var err error

			if tt.isPointer {
				// Build pointer address manually
				header := (tt.addrType << 4) | (tt.networkId & AddressHeaderNetworkMask)
				data := []byte{header}
				data = append(data, paymentHash...)
				// Add pointer data (slot=1, tx=0, cert=0 -> minimal encoding)
				data = append(data, 0x01, 0x00, 0x00)
				addr, err = NewAddressFromBytes(data)
			} else if tt.hasPayment && tt.hasStaking {
				addr, err = NewAddressFromParts(tt.addrType, tt.networkId, paymentHash, stakingHash)
			} else if tt.hasPayment {
				addr, err = NewAddressFromParts(tt.addrType, tt.networkId, paymentHash, nil)
			} else {
				// Reward address - build directly from bytes
				header := (tt.addrType << 4) | (tt.networkId & AddressHeaderNetworkMask)
				data := []byte{header}
				data = append(data, stakingHash...)
				addr, err = NewAddressFromBytes(data)
			}

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.addrType, addr.Type())
			assert.Equal(t, uint(tt.networkId), addr.NetworkId())

			// Round-trip test
			addrBytes, err := addr.Bytes()
			require.NoError(t, err)
			addr2, err := NewAddressFromBytes(addrBytes)
			require.NoError(t, err)
			assert.Equal(t, addr.String(), addr2.String())
		})
	}
}

func TestCIP0019_NetworkIdEdgeCases(t *testing.T) {
	paymentHash := make([]byte, 28)
	stakingHash := make([]byte, 28)

	tests := []struct {
		name              string
		networkId         uint8
		expectError       bool
		expectedNetworkId uint
	}{
		{
			name:              "testnet (0)",
			networkId:         AddressNetworkTestnet,
			expectError:       false,
			expectedNetworkId: AddressNetworkTestnet,
		},
		{
			name:              "mainnet (1)",
			networkId:         AddressNetworkMainnet,
			expectError:       false,
			expectedNetworkId: AddressNetworkMainnet,
		},
		{
			name:        "invalid network id (2)",
			networkId:   2,
			expectError: true,
		},
		{
			name:        "invalid network id (15)",
			networkId:   15,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := NewAddressFromParts(
				AddressTypeKeyKey,
				tt.networkId,
				paymentHash,
				stakingHash,
			)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedNetworkId, addr.NetworkId())
			}
		})
	}
}

func TestCIP0019_ByronAddressCRC32Validation(t *testing.T) {
	tests := []struct {
		name          string
		addressHex    string
		expectError   bool
		errorContains string
	}{
		{
			name:        "valid Byron mainnet address",
			addressHex:  "82d818584283581caf56de241bcca83d72c51e74d18487aa5bc68b45e2caa170fa329d3aa101581e581cea1425ccdd649b25af5deb7e6335da2eb8167353a55e77925122e95f001a3a858621",
			expectError: false,
		},
		{
			name:        "valid Byron testnet address",
			addressHex:  "82d818582483581c5d5e698eba3dd9452add99a1af9461beb0ba61b8bece26e7399878dda1024102001a36d41aba",
			expectError: false,
		},
		{
			name:          "Byron address with invalid CRC32 (modified checksum)",
			addressHex:   "82d818582483581c5d5e698eba3dd9452add99a1af9461beb0ba61b8bece26e7399878dda1024102001a00000000",
			expectError:   true,
			errorContains: "checksum",
		},
		{
			name:          "Byron address with corrupted payload",
			addressHex:   "82d818582483581c0000000000000000000000000000000000000000000000000000000000a1024102001a36d41aba",
			expectError:   true,
			errorContains: "checksum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addrBytes, err := hex.DecodeString(tt.addressHex)
			require.NoError(t, err)

			_, err = NewAddressFromBytes(addrBytes)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCIP0019_EmptyAndNilInputHandling(t *testing.T) {
	t.Run("empty bytes", func(t *testing.T) {
		_, err := NewAddressFromBytes([]byte{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty byte slice")
	})

	t.Run("nil bytes", func(t *testing.T) {
		_, err := NewAddressFromBytes(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty byte slice")
	})

	t.Run("empty string", func(t *testing.T) {
		_, err := NewAddress("")
		require.Error(t, err)
	})

	t.Run("whitespace only string", func(t *testing.T) {
		_, err := NewAddress("   ")
		require.Error(t, err)
	})
}

func TestCIP0019_MaximumLengthAddresses(t *testing.T) {
	// Test addresses with extra data (as seen in the wild)
	// Reference: https://github.com/IntersectMBO/cardano-ledger/issues/2729

	tests := []struct {
		name       string
		addressHex string
		expectLen  int
	}{
		{
			name:       "long address with extra data",
			addressHex: "015bad085057ac10ecc7060f7ac41edd6f63068d8963ef7d86ca58669e5ecf2d283418a60be5a848a2380eb721000da1e0bbf39733134beca4cb57afb0b35fc89c63061c9914e055001a518c7516",
			expectLen:  78, // This address has extra data beyond standard 57 bytes
		},
		{
			name:       "enterprise address with extra trailing data",
			addressHex: "61549b5a20e449a3e394b762705f64b9a26b99013003a2bfdba239967c00",
			expectLen:  30,
		},
		{
			name:       "standard type 0 address",
			addressHex: "013f35615835258addded1c2e169f3a2ab4ae94d606bde030e7947f5184ff5f8e3d43ce6b19ec4197e331e86d0f5e58b02d7a75b5e74cff95d",
			expectLen:  57,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addrBytes, err := hex.DecodeString(tt.addressHex)
			require.NoError(t, err)
			assert.Equal(t, tt.expectLen, len(addrBytes))

			addr, err := NewAddressFromBytes(addrBytes)
			require.NoError(t, err)

			// Round-trip should preserve the data
			roundTrip, err := addr.Bytes()
			require.NoError(t, err)
			assert.Equal(t, addrBytes, roundTrip)

			// String encoding should work
			addrStr := addr.String()
			assert.NotEmpty(t, addrStr)

			// Should be able to parse back from string
			addr2, err := NewAddress(addrStr)
			require.NoError(t, err)
			assert.Equal(t, addr.String(), addr2.String())
		})
	}
}

func TestCIP0019_PointerAddressEdgeCases(t *testing.T) {
	paymentHash := make([]byte, 28)

	tests := []struct {
		name         string
		slot         uint64
		txIndex      uint64
		certIndex    uint64
		expectError  bool
		expectedLen  int
	}{
		{
			name:        "minimal pointer (0,0,0)",
			slot:        0,
			txIndex:     0,
			certIndex:   0,
			expectedLen: 32, // 1 + 28 + 3 (one byte each)
		},
		{
			name:        "small values (1,1,1)",
			slot:        1,
			txIndex:     1,
			certIndex:   1,
			expectedLen: 32,
		},
		{
			name:        "values requiring 2 bytes (128,128,128)",
			slot:        128,
			txIndex:     128,
			certIndex:   128,
			expectedLen: 35, // 1 + 28 + 6 (two bytes each)
		},
		{
			name:        "large slot value",
			slot:        0xFFFFFFFF,
			txIndex:     0,
			certIndex:   0,
			expectedLen: 36, // 1 + 28 + 5 + 1 + 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build the pointer encoding manually
			encodeVarUint := func(val uint64) []byte {
				if val == 0 {
					return []byte{0x00}
				}
				result := []byte{byte(val & 0x7F)}
				val /= 128
				for val > 0 {
					result = append([]byte{byte((val & 0x7F) | 0x80)}, result...)
					val /= 128
				}
				return result
			}

			header := byte((AddressTypeKeyPointer << 4) | (AddressNetworkMainnet & AddressHeaderNetworkMask))
			data := []byte{header}
			data = append(data, paymentHash...)
			data = append(data, encodeVarUint(tt.slot)...)
			data = append(data, encodeVarUint(tt.txIndex)...)
			data = append(data, encodeVarUint(tt.certIndex)...)

			addr, err := NewAddressFromBytes(data)
			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, uint8(AddressTypeKeyPointer), addr.Type())

			// Verify the pointer data is preserved
			stakingPayload := addr.StakingPayload()
			require.NotNil(t, stakingPayload)
			pointer, ok := stakingPayload.(AddressPayloadPointer)
			require.True(t, ok)
			assert.Equal(t, tt.slot, pointer.Slot)
			assert.Equal(t, tt.txIndex, pointer.TxIndex)
			assert.Equal(t, tt.certIndex, pointer.CertIndex)
		})
	}
}

func TestCIP0019_InvalidBech32Inputs(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		{
			name:        "invalid characters",
			input:       "addr1qx!@#$%^&*()",
			expectError: true,
		},
		{
			name:        "wrong HRP",
			input:       "wrong1qx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgse35a3x",
			expectError: true,
		},
		{
			name:        "truncated address",
			input:       "addr1qx2fxv2",
			expectError: true,
		},
		{
			name:        "invalid checksum",
			input:       "addr1qx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgsXXXXXX",
			expectError: true,
		},
		{
			name:        "mixed case (should be interpreted as base58)",
			input:       "Ae2tdPwUPEYwFx4dmJheyNPPYXtvHbJLeCaA96o6Y2iiUL18cAt7AizN2zG",
			expectError: false, // This is a valid Byron address
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewAddress(tt.input)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCIP0019_PaymentAndStakeAddressExtraction(t *testing.T) {
	tests := []struct {
		name              string
		address           string
		hasPaymentAddress bool
		hasStakeAddress   bool
	}{
		{
			name:              "Type 0 (KeyKey) - has both",
			address:           "addr1qx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgse35a3x",
			hasPaymentAddress: true,
			hasStakeAddress:   true,
		},
		{
			name:              "Type 6 (KeyNone) - payment only",
			address:           "addr1vx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzers66hrl8",
			hasPaymentAddress: true,
			hasStakeAddress:   false,
		},
		{
			name:              "Type 14 (NoneKey) - stake only",
			address:           "stake1uyehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gh6ffgw",
			hasPaymentAddress: false,
			hasStakeAddress:   false, // StakeAddress() returns nil for reward addresses
		},
		{
			name:              "Type 4 (KeyPointer) - no payment/stake address extraction",
			address:           "addr1gx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer5pnz75xxcrzqf96k",
			hasPaymentAddress: false, // PaymentAddress() returns nil for pointer addresses
			hasStakeAddress:   false, // Pointer addresses don't extract stake address
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := NewAddress(tt.address)
			require.NoError(t, err)

			if tt.hasPaymentAddress {
				assert.NotNil(t, addr.PaymentAddress())
			} else {
				assert.Nil(t, addr.PaymentAddress())
			}

			if tt.hasStakeAddress {
				assert.NotNil(t, addr.StakeAddress())
			} else {
				assert.Nil(t, addr.StakeAddress())
			}
		})
	}
}

func TestCIP0019_HeaderByteParsing(t *testing.T) {
	// Test that address type and network ID are correctly extracted from header byte
	tests := []struct {
		name              string
		headerByte        byte
		expectedType      uint8
		expectedNetworkId uint8
	}{
		{
			name:              "Type 0, Testnet",
			headerByte:        0x00,
			expectedType:      0,
			expectedNetworkId: 0,
		},
		{
			name:              "Type 0, Mainnet",
			headerByte:        0x01,
			expectedType:      0,
			expectedNetworkId: 1,
		},
		{
			name:              "Type 1, Mainnet",
			headerByte:        0x11,
			expectedType:      1,
			expectedNetworkId: 1,
		},
		{
			name:              "Type 6, Mainnet (enterprise)",
			headerByte:        0x61,
			expectedType:      6,
			expectedNetworkId: 1,
		},
		{
			name:              "Type 7, Testnet (enterprise script)",
			headerByte:        0x70,
			expectedType:      7,
			expectedNetworkId: 0,
		},
		{
			name:              "Type 14, Mainnet (stake key)",
			headerByte:        0xE1,
			expectedType:      14,
			expectedNetworkId: 1,
		},
		{
			name:              "Type 15, Testnet (stake script)",
			headerByte:        0xF0,
			expectedType:      15,
			expectedNetworkId: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify header byte parsing
			parsedType := (tt.headerByte & AddressHeaderTypeMask) >> 4
			parsedNetwork := tt.headerByte & AddressHeaderNetworkMask

			assert.Equal(t, tt.expectedType, parsedType)
			assert.Equal(t, tt.expectedNetworkId, parsedNetwork)
		})
	}
}

func TestCIP0019_CBORRoundTrip(t *testing.T) {
	testAddresses := []string{
		// Type 0 - KeyKey
		"addr1qx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgse35a3x",
		// Type 1 - ScriptKey
		"addr1z8phkx6acpnf78fuvxn0mkew3l0fd058hzquvz7w36x4gten0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgs9yc0hh",
		// Type 6 - KeyNone
		"addr1vx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzers66hrl8",
		// Type 14 - NoneKey (stake)
		"stake1uyehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gh6ffgw",
		// Testnet
		"addr_test1qz2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgs68faae",
	}

	for _, addrStr := range testAddresses {
		t.Run(addrStr[:20]+"...", func(t *testing.T) {
			addr, err := NewAddress(addrStr)
			require.NoError(t, err)

			// Marshal to CBOR
			cborData, err := addr.MarshalCBOR()
			require.NoError(t, err)
			assert.NotEmpty(t, cborData)

			// Unmarshal from CBOR
			var decoded Address
			err = decoded.UnmarshalCBOR(cborData)
			require.NoError(t, err)

			// Verify round-trip
			assert.Equal(t, addr.String(), decoded.String())
			assert.Equal(t, addr.Type(), decoded.Type())
			assert.Equal(t, addr.NetworkId(), decoded.NetworkId())
		})
	}
}

func TestCIP0019_ByronAddressAttributes(t *testing.T) {
	// Test Byron addresses with various attribute configurations
	tests := []struct {
		name              string
		address           string
		expectMainnet     bool
		hasDerivationPath bool
	}{
		{
			name:              "mainnet with derivation path",
			address:           "DdzFFzCqrht2ii4Vc7KRchSkVvQtCqdGkQt4nF4Yxg1NpsubFBity2Tpt2eSEGrxBH1eva8qCFKM2Y5QkwM1SFBizRwZgz1N452WYvgG",
			expectMainnet:     true,
			hasDerivationPath: true,
		},
		{
			name:              "testnet (preview) without derivation path",
			address:           "FHnt4NL7yPXvDWHa8bVs73UEUdJd64VxWXSFNqetECtYfTd9TtJguJ14Lu3feth",
			expectMainnet:     false,
			hasDerivationPath: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := NewAddress(tt.address)
			require.NoError(t, err)

			assert.Equal(t, uint8(AddressTypeByron), addr.Type())

			if tt.expectMainnet {
				assert.Equal(t, uint(AddressNetworkMainnet), addr.NetworkId())
			} else {
				assert.Equal(t, uint(AddressNetworkTestnet), addr.NetworkId())
			}

			// Check Byron attributes
			attr := addr.ByronAttr()
			if tt.hasDerivationPath {
				assert.NotEmpty(t, attr.Payload)
			}
		})
	}
}
