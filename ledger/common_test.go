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
		// Byron address
		{
			addressBytesHex: "82d818584283581caf56de241bcca83d72c51e74d18487aa5bc68b45e2caa170fa329d3aa101581e581cea1425ccdd649b25af5deb7e6335da2eb8167353a55e77925122e95f001a3a858621",
			expectedAddress: "DdzFFzCqrht2ii4Vc7KRchSkVvQtCqdGkQt4nF4Yxg1NpsubFBity2Tpt2eSEGrxBH1eva8qCFKM2Y5QkwM1SFBizRwZgz1N452WYvgG",
		},
	}
	for _, testDef := range testDefs {
		addr := Address{}
		err := addr.populateFromBytes(test.DecodeHexString(testDef.addressBytesHex))
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
