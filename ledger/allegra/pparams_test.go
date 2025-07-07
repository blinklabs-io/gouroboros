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

package allegra_test

import (
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

func TestAllegraProtocolParamsUpdate(t *testing.T) {
	testDefs := []struct {
		startParams    allegra.AllegraProtocolParameters
		updateCbor     string
		expectedParams allegra.AllegraProtocolParameters
	}{
		{
			startParams: allegra.AllegraProtocolParameters{
				Decentralization: &cbor.Rat{Rat: new(big.Rat).SetInt64(1)},
			},
			updateCbor: "a10cd81e82090a",
			expectedParams: allegra.AllegraProtocolParameters{
				Decentralization: &cbor.Rat{Rat: big.NewRat(9, 10)},
			},
		},
		{
			startParams: allegra.AllegraProtocolParameters{
				ProtocolMajor: 3,
			},
			updateCbor: "a10e820400",
			expectedParams: allegra.AllegraProtocolParameters{
				ProtocolMajor: 4,
			},
		},
	}
	for _, testDef := range testDefs {
		cborBytes, err := hex.DecodeString(testDef.updateCbor)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		var tmpUpdate allegra.AllegraProtocolParameterUpdate
		if _, err := cbor.Decode(cborBytes, &tmpUpdate); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpParams := testDef.startParams
		tmpParams.Update(&tmpUpdate)
		if !reflect.DeepEqual(tmpParams, testDef.expectedParams) {
			t.Fatalf(
				"did not get expected params:\n     got: %#v\n  wanted: %#v",
				tmpParams,
				testDef.expectedParams,
			)
		}
	}
}

func TestAllegraUtxorpc(t *testing.T) {
	inputParams := allegra.AllegraProtocolParameters{
		MinFeeA:            500,
		MinFeeB:            2,
		MaxBlockBodySize:   65536,
		MaxTxSize:          16384,
		MaxBlockHeaderSize: 1024,
		KeyDeposit:         2000,
		PoolDeposit:        500000,
		MaxEpoch:           2160,
		NOpt:               100,
		A0:                 &cbor.Rat{Rat: big.NewRat(1, 2)},
		Rho:                &cbor.Rat{Rat: big.NewRat(3, 4)},
		Tau:                &cbor.Rat{Rat: big.NewRat(5, 6)},
		ProtocolMajor:      8,
		ProtocolMinor:      0,
		MinUtxoValue:       1000000,
	}

	expectedUtxorpc := &cardano.PParams{
		MinFeeCoefficient:        500,
		MinFeeConstant:           2,
		MaxBlockBodySize:         65536,
		MaxTxSize:                16384,
		MaxBlockHeaderSize:       1024,
		StakeKeyDeposit:          2000,
		PoolDeposit:              500000,
		PoolRetirementEpochBound: 2160,
		DesiredNumberOfPools:     100,
		PoolInfluence: &cardano.RationalNumber{
			Numerator:   int32(1),
			Denominator: uint32(2),
		},
		MonetaryExpansion: &cardano.RationalNumber{
			Numerator:   int32(3),
			Denominator: uint32(4),
		},
		TreasuryExpansion: &cardano.RationalNumber{
			Numerator:   int32(5),
			Denominator: uint32(6),
		},
		ProtocolVersion: &cardano.ProtocolVersion{
			Major: 8,
			Minor: 0,
		},
	}

	result, err := inputParams.Utxorpc()
	if err != nil {
		t.Fatalf("Utxorpc() conversion failed: %v", err)
	}
	if !reflect.DeepEqual(result, expectedUtxorpc) {
		t.Fatalf(
			"Utxorpc() test failed for Allegra:\nExpected: %#v\nGot: %#v",
			expectedUtxorpc,
			result,
		)
	}
}

// Unit test for AllegraTransactionBody.Utxorpc()
func TestAllegraTransactionBody_Utxorpc(t *testing.T) {
	// mock input
	input := shelley.NewShelleyTransactionInput(
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		1,
	)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	address := common.Address{}

	// Define a transaction output
	output := shelley.ShelleyTransactionOutput{
		OutputAddress: address,
		OutputAmount:  1000,
	}

	// Create transaction body
	txBody := &allegra.AllegraTransactionBody{
		TxInputs:                inputSet,
		TxOutputs:               []shelley.ShelleyTransactionOutput{output},
		TxFee:                   200,
		Ttl:                     5000,
		TxValidityIntervalStart: 4500,
	}

	// Run Utxorpc conversion
	actual, err := txBody.Utxorpc()
	if err != nil {
		t.Errorf("Failed to convert the transaction")
	}

	// Check that the fee matches
	if actual.Fee != txBody.Fee() {
		t.Errorf(
			"AllegraTransactionBody.Utxorpc() fee mismatch\nGot: %d\nWant: %d",
			actual.Fee,
			txBody.Fee(),
		)
	}
	// Check number of inputs
	if len(actual.Inputs) != len(txBody.Inputs()) {
		t.Errorf(
			"AllegraTransactionBody.Utxorpc() input length mismatch\nGot: %d\nWant: %d",
			len(actual.Inputs),
			len(txBody.Inputs()),
		)
	}
	// Check number of outputs
	if len(actual.Outputs) != len(txBody.Outputs()) {
		t.Errorf(
			"AllegraTransactionBody.Utxorpc() output length mismatch\nGot: %d\nWant: %d",
			len(actual.Outputs),
			len(txBody.Outputs()),
		)
	}
}

// Unit test for AllegraTransaction.Utxorpc()
func TestAllegraTransaction_Utxorpc(t *testing.T) {
	// Prepare mock input
	input := shelley.NewShelleyTransactionInput(
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		0,
	)
	inputSet := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)

	// Prepare mock output
	address := common.Address{}
	output := shelley.ShelleyTransactionOutput{
		OutputAddress: address,
		OutputAmount:  2000,
	}

	// Create transaction body
	tx := &allegra.AllegraTransaction{
		Body: allegra.AllegraTransactionBody{
			TxInputs:                inputSet,
			TxOutputs:               []shelley.ShelleyTransactionOutput{output},
			TxFee:                   150,
			Ttl:                     1000,
			TxValidityIntervalStart: 950,
		},
		WitnessSet: shelley.ShelleyTransactionWitnessSet{},
		TxMetadata: nil,
	}

	// Run Utxorpc conversion
	actual, err := tx.Utxorpc()
	if err != nil {
		t.Errorf("Failed to convert the transaction")
	}
	// Assertion checks
	if actual.Fee != tx.Fee() {
		t.Errorf(
			"AllegraTransaction.Utxorpc() fee mismatch\nGot: %d\nWant: %d",
			actual.Fee,
			tx.Fee(),
		)
	}

	if len(actual.Inputs) != len(tx.Inputs()) {
		t.Errorf(
			"AllegraTransaction.Utxorpc() input length mismatch\nGot: %d\nWant: %d",
			len(actual.Inputs),
			len(tx.Inputs()),
		)
	}

	if len(actual.Outputs) != len(tx.Outputs()) {
		t.Errorf(
			"AllegraTransaction.Utxorpc() output length mismatch\nGot: %d\nWant: %d",
			len(actual.Outputs),
			len(tx.Outputs()),
		)
	}
}

// https://cexplorer.io/block/8115134ab013f6a5fd88fd2a10825177a2eedcde31cb2f1f35e492df469cf9a8
// Hash: 8115134ab013f6a5fd88fd2a10825177a2eedcde31cb2f1f35e492df469cf9a8
// Slot: 23068573
var allegraBlockHex = "84828f1a005280141a015fff9d58206c75ffd5efb79a234d49e9e279be21f7f9b73a5b9db6cd5271f5db00bb8cc1085820e35049a00c155dc46c09d1c2838cda7b8b2bc68f5134fd4be528fc388363d43658200ca8bfb99d223616e305f5839cff11a9c6dc637d2d9a3cda6531be1a842259a7825840bf1d6983a8e3fd881f8ae17fa168957e9fdc1d0d26acc13b2afdf3ff4f212f16f4be0bbf3a6ab4c1ffaff62e8ff4ac76ae3cccb845a73b4460617aa9bc4e4c555850aae23d05e6e52e9241ce52db8f1274a4cbbff19f1d9f854cc072701bb8c640f6fa25ef8132e2e721380051a7ee0cdb91fe9b9cbd30f28f278964462150c328984a88f4a2d3775518b9d106d91484580882584000048a1dfc14dfb36fe05a78fd7d4b15f91ca0c05b79977170ee616147e8d113e42fca6b97816ccb93af5339255d3beb04d2b7767d80e31306018e06bbe406cd58507c6e72d25832f5a30052f1a48fc188624d8b501a2b4178169b59999edfb14b9e50fed1ed8b76ef90d489a9ad4ba6afca79e0c8717e336fc0c3e5e29ddecbed3cb067d74868abc24763be13d1d7d9a70f1914e6582017c0799bb8e4aeb6e106f30b7032af347e1b1cfef38f45edcee333e18256c47c5820d259e09394ecabd3e7de126b4da365afcd556185807ab41db1189781b617490f0118ae584081a2a058d9d036e3e243e0a1b9f989796de96258f6308ecfef8708770f7ac2e746fee8583d962f1a7f2a161ada1daf627340ceb88d32d957990992afbc7d270804005901c0deb4e2adbe562bb76dda9d085fbff084a3da7a48164fc394ce6caae005aee7d80e51767ef329a809abb780c815afadaaf60cb4cc451654a641fe57f03266f40af55307c888af9dd02669af94647d371de1ed688c4e7c84d8f6065dd9dfdcae4d8599ead8bb39d70ce536e760464e07924e5d53dfba388ebe5370cdcb94ac19dedb31f9d48dd8bcda6b4717db02f52ab7cd2a4e92dde744f81dadd35b6c6bd66eb7185b49edb6fb394c74af90ccd5d815249a5546b511e77301051328f4bc39dc0d0c48002f042a9959fee8fcba68c45116eb3fccfd1b0c736069208ed8ff787dc6ae2f665866a8e55d08792591d789ced9860eb52d5642c5ec269551baf0042892edf489e5231904e510db7a5d639b5732999360a61485d800fd0315cf9b3467b8a98dc99e13f9bd587a487e278d2012e5e5a11477fedb80b93f9e62f2816db0fd625f8786dc2e44b9669496b6be1dd46f5b3dadbca2f236ae5fe0aac0528891d27f56ec7837e71a6e9c2d4d66fe2b9758a7734e0ca66bf02059302a9e0620e92431c06ea9642c6b4abdbbd02d84d5e0f69d2fa1b3574796f3df12ffdca65764b1216b0dcc47ac840296ac2c923ec4ba2febafc2ed4c5b183695c0b4b8b596198da500828258207935a8976f6a532174ba64cf6994ca38df98cb47bbb3a933ab6b12a3ed99460701825820fa75a71151f368630ff58e602a2b363d4c640b7d95408476a89d35313f141950010182825839012c152eaa9e68dd7123a3054190dc987a24e50f1ab389c44a0c7a4089beb4d4d62d8f0dce5d745df4a670998aa20f54703b2bdc7a00b7d3d21a000f4240825839017226dc6576691d1ecc2be8dba97382e14a26e8e10bd31f28dce4a0a5beb4d4d62d8f0dce5d745df4a670998aa20f54703b2bdc7a00b7d3d21b00000004e27c5efa021a0002d4dd031a01601b9d07582052dd921a07977725c4137ddd1bee1bd8b89220ad6eec43b232cb98bc11a90045a500818258201325597e43d1fc2293c8390d2e8427b49609080e2b0379cd43b6ade1003444c901018182583901df42df1f501b9cdbc3d0a662a88b4313e15f906ca9cb1f176143018b15824ce04d9248acead6e8a0ca88c63d3e1a591cfc240646ff907d711a0e1c1f7e021a0002a961031a01601b74048282008200581c15824ce04d9248acead6e8a0ca88c63d3e1a591cfc240646ff907d7183028200581c15824ce04d9248acead6e8a0ca88c63d3e1a591cfc240646ff907d71581c490353aa6b85efb28922acd9e0ee1dcf6d0c269b9f0583718b0274baa40081825820b343d8eaa23278024bfa69940446c4914226b3236372f93741d45ef4ad033175000182825839017edb694b6f23d40ae8940eb745d9be312b06e7539c2763f7f453ae2e7edb694b6f23d40ae8940eb745d9be312b06e7539c2763f7f453ae2e1a3ba70c9f82584c82d818584283581c052ea043cfe85334a029dcc9270b2f923988fe8cc6d3b89f907ab2fda101581e581c9b1771bd305e4a511efc4da9962b55e8e33fd65663038c8d3b0f826f001accb84ac91b000000195abc80f9021a0002a515031a01601ba1a40081825820d53ea8c652a58a8d0b4f62b92acdb5dd71a7b49c68cd3a686bec22e6e088005d01018282584c82d818584283581cf6afb6b5ffa538cba54edb1513a4aa031d280f9448861273b1a1bd6fa101581e581c2b0b011ba3683d15ad5eef2a4a1e16bc63fa2df24653334ea986ae10001a53d31d6b1a10a133c0825839012d1f6eb196eb8c352ad8512c39b2ef980fd9ff11f61f13b327cb3f06aba8304aa2f0dbce3c7b1201659b8872fdcfc2bbbc9144853f4bdace1a0f3efedb021a0002c30a031a01600d72a500828258207e5ff387a8ef0e3c155409e043cc3cabb6072fbafc4ea3f0a04511df4f94601600825820f60d6c6e2290f3209e17aec5dfed8a1348027132a7016c799b8d90b9560ae16a000181825839016c2e533b801a077b9547186eeea8ecbd86e3f60280c17ef6ee560e2d2a2d2b25c40708c7e402543dbd1d45638aa89bcf48f28b2cd2d0c2cd1a0240a1cb021a0002be85031a01601b9b048183028200581c2a2d2b25c40708c7e402543dbd1d45638aa89bcf48f28b2cd2d0c2cd581cbcd7cf751b59f949170a7e6599f9ac03e49b32c19f3f1d8dad3ac210a50081825820d0ec7f92adca13f9fd2a8efee0821a7d49bacfb9a277e874dbb82720c28eb98100018182583901bee18d4ca7b7ecede3dfd7ea503d5e565bbdd22ebdf5c76347009d83383da495e9fd7d8b209073801f2264dfb1f147abd3dc869853c902131a0f50c70a021a00060f2c031a01601b8205a1581de1383da495e9fd7d8b209073801f2264dfb1f147abd3dc869853c902131a00416216a50081825820d7d22133f8a214cae02867b15b8621bb289333fb01f0e0b9a8e1676e75cf1534130181825839010db698344190442da3efc5e07cef572f556235ae9cd61b4e6e921cc80d00e9b900af6834b7a20b6f61512eccf1ca5aa4b4e18c1c7a602e5f1a0086aadf021a0002a961031a01601b2d048282008200581c0d00e9b900af6834b7a20b6f61512eccf1ca5aa4b4e18c1c7a602e5f83028200581c0d00e9b900af6834b7a20b6f61512eccf1ca5aa4b4e18c1c7a602e5f581cb3cdd3bbf09af72badca85e515c0206d6d22038e2af843521e10a928a400818258205522324d0c5d1a7dc01a2fde2e1d9a4091453208a08834a61219d5a9deaec25001018282583901b73955dd3106bf1d53f0ee6658360c3667da33dad12df60670069599801e393fab6ee38e0285df75a10e7f6bcdad4f4cf8ea8d8666b82aa81a0098968082584c82d818584283581cdf80c93feb84ba882f68071f5c07449df2de1517554feffac9c09b1aa101581e581c3083349abf53ea9270946a5032f8707be9d3140396355bac9cd822e0001ab24c4ca81aa6377866021a0002a56d031a01601bada50081825820ddf7fc7148ef595e89819a3fbfc349d34f394efa61e2d20fa299fd1e8d20552600018182583901bcdd68c4ea7b3ffd4953ce845d0b8fc68d4baf4d5d9fe8b03e83543fb000fbcd2db0006bb1b6731423c376430870d848e2efb46d2785cd8e1a1aa4a0dc021a00062764031a01601b99048282008200581cb000fbcd2db0006bb1b6731423c376430870d848e2efb46d2785cd8e83028200581cb000fbcd2db0006bb1b6731423c376430870d848e2efb46d2785cd8e581c8f67312cb7927075292fcc986927c986ccdd630a3bae1b52dd4671a4a400818258205ad19227ed5da8d58ac79ab77f550ece3277d24e54ebdb8753ae1279ca1916ce01018282583901d973e05062480d23d925842f43ecd4c68801d7e71c3934298d61a9a4b924c914602591ba4f3ac65f1b50d65891de4742ec52bf577277d5501a0aa5b3d282584c82d818584283581c22d248876f14403424510eb85d38413a648381cdfa787a8b3d3b7319a101581e581c3083349abf53eab0296b1250821d55ddaabd554a052bd9c1e7121aaf001a92b8869e1b000000f492014cf3021a0002a56d031a01601bada50081825820e9227fb7ec548b61123d346d6d1f8c3cdcfd32775081f635288bf33ee33513f7181901818258390168937c4613ec9f5fd027cb14eb5c1482a4e2df8f8f5df2d4d144efa743baec8756821d9aa010e61e4f71417e2eafec103a3483c098cf378c1a055aa437021a0002a649031a01601bb6048183028200581c43baec8756821d9aa010e61e4f71417e2eafec103a3483c098cf378c581c77b0a93c26ac65be36e9a9f220f9a43cbc57d705fc5d8f1de5fdeea1a4008182582007bea772e2581048636e409107dce47f3b4c84850ef46238ada1efda8cddd51201018282581d61e989a19480bda0b952575a4241d81a7f4fc4b06f75d8ed846fc815941a3b9aca0082583901d8d0078e20f4bf010f1935a2b9e06a2618d5d8bcd9f23a9951ae7e41d8d0078e20f4bf010f1935a2b9e06a2618d5d8bcd9f23a9951ae7e411b00000002548017e9021a0002c6d1031a05f5e100a5008182582066a7ef7310b7e83712740291c7b65abb6fb4e3651b2f5877ac4a30a26db100941818018182583901c952e447c9df000e1bc8c6c5846b7e1d5ba026c75ba082f03de403918e634f72581f300556a6d2e0fd7f6106610fbe1d0559766f9a5087f21a09a6c6ff021a0002ac21031a01601bba048282008200581c8e634f72581f300556a6d2e0fd7f6106610fbe1d0559766f9a5087f283028200581c8e634f72581f300556a6d2e0fd7f6106610fbe1d0559766f9a5087f2581c5790d62ab1ba703e861fe800f9cefaaf1485c3ca42c6ba9ce74690a18da1008282582098b111708d2018d37389804020b755b357e858e6453539d328b2bb435035032858401e47d01f454398af6d5f3d07876a80aac5bdeb8e2b00008f8eeb32457ff00dc364723173b9d50c7133b485fb3c3b1f82329c4206f1fb151fb216782aacbbb80182582075475211efdf37e44dba3d7c6304679658bcacde7e7358449ad4d66b4b88b28658404797678b2fdf22271b619cbf04ce5510a5903d40b85ec542eb71f32055d87cab73c40ced1d3005728f3bf1c292c71210de84175400c7cbfa31ec3437f6a9f60da10082825820ca34bb4eb86a1b7ea29cf357386c192215708dda88bc5c8b91fe31b25b877e3d58408498a5b5b2a83e88ad54b1da5a67e816c13ed69516540ba4d113872cfb6a774ad3fd0ae3305951ceaea58d2a3f6452c6baedaf298b1c14cb5522926859c4da0682582083b0fa0ea8ef5a506c24e993b489e4e367328fa5cd39ffecdf792fcdebdc47995840a833a254cec8698f4801e2176d57c148c03adfcba9102fe73a479a6a28c32d0cd7e3d7d26751b8a328e10f2e2fc1b42d563e4655cddf8d4c7bbe07c8cc65e20ba1028184582066110eabde7e2f7772dc8b59e849e4414b8f10d7a93a823ca9132094f3d0d365584002aeed8a79739d7a83b2c84acfccd8c2f688c4b2f7a150c7937fa54d82028bb9af61d1a382c0bf76f28a094795260338b93e6b1ba33d206b97c5056f2a205100582006eade8cf75dffef76ef63d445164fa01ffc99211eb093cc340f5fab15b257cf5822a101581e581c9b1771bd305e4a3680416ba97628d422f50849601476779164f28915a100818258203c693f0da4e77b94bd0890d82ccd62066662ab7e5a60043854301be914b22b3d584000ef4847489cefcd1c77e70f7d30d2ef98cbd0b5177515add351308b4111c69ade63f5e98565108c3ffa82bebc3a331bdc4583112e8d6738dcf9eb5430673700a1008382582085f1a335b6a9a84e1f2a4ae34be9e130c743b961c3208de0240e828ff64731b9584080fff711fcdb593445215e443700c1625b57dd77d0d88ab9aefde50e9a6c9a519880a74c7a5bd2e24df2f0c07210531b972479a4486d030c889fd95f941726088258205f95813dcc70eb6f38a2a8ee1458d5d809fe5e91e58cc99655a7156bc4199dd15840573809cf5f8834fd49dfa13ab56c9890a6e6c6eab14b370b51fb291f0d2d4173b72fb978b01bf521e75df690f10b74a2607ef7324f90f17bfc9e93a1a99e1905825820c3241350c9f3a4d24a423d58096bbce05cd476f3c56325cfd44c560e2a62ac7158403e5ebf3ba4d3dd4ff17a0835f78ac953018773b3c956f597d07c1d53abd2bb83d96650d0fe43f1a3ee19fb421936efe77c0129d4fcb56e3ad9852038326c190ba10082825820afd3b9d859a5e6e614f23604b0be03f331aeb172795cd3b7c7580b62afc8964b5840b861f2daecd8cd2dd2ff5e57f58e765445a34a8948732b37c65bc3ca8528fc35c4e58845ee490aec9030ee174990b699f87b8cfb0bce2a70127c5ce7c6ff0f068258201f05b48f20a55fb5a00287ed6c25d6e10a7a45a2358187ee3435ca77c592b600584034dfbf9b72739739365b00a303a03de0091bf798275893d41f5287a1f219c2cb2effdc9766ead7e30cffcee8e40394c024f87e045562d1e1facd08fb43a5040da100828258206e49b012f3fe4b570cf394e4641cbab6f3eb105538db596635fdc8220a8521e85840f4ca60ba95156f717531de522b33c8db39c8913681343a462baa173b39adef2df037b22d649dca46401dd3dbc497bc888b1e702a04b17e64d9f8877cdec0a90d825820eda11b032112ae17d55b7693ad6fdd6f442dfd3ba17fce8de8f78af446e7f2d35840e1dc77609243de39134a925ba6caa0e4c6d1ecb1a10384031fbdce83dbd42b2630da7c1dca7973f87d9354451171d253b75880a9e09ffa40c6589c694f008e03a102818458207e3b560e96389a1b555ec4dbf4a3bee90bac5a0158bf6cbe7ccfb4df5f8333c95840fc47e142d6df76f2e1aa3a62bfefcf5d03b4994386c864eb5359eba6315bcfba40fb8cf7db4907aa5cfe913dffaae05e5cd3000f668192d1a84d33d6a13d700d5820c1318ae568db0d0551230debdedb18a44c9b0bc7446472391fb231e4fea72aa15822a101581e581c3083349abf53eaa85881a250fce14164bf6d67bfbcf8dc87873dcdbca100828258209ffbe292231bea24dee02ec841ff28062270935f5be98eba1a6216ba578cd13b584063a0c8beb47675229b0b65b24bba8b7ff90baee39f743bf88a99b8576c34b0b076c317eb1accb160048804202dedc5681367dbf9b12a36d20b4f2f69e6701b0a825820004eea3e4fe78dcdfc5862493e2e8d90b33922b48dcc4753d7e26a6e024aba0b5840a60f7ddb7fdaf6f22ac173c95a2db41d599be8edff4a606f7df8914b82a51efe9a9a1e5c31dab06d21b6b6ad7ca8e3428000b3fbcbc785306653acd70d857d0da10281845820ef3fa874d1299f785c4bab6eb8585d1fbf99fbf98ebee0d672466cee09783b075840e974d15072531f035a378f6782eaca7134ff89d8a97418b426f7f6a461953a5878370019d61e4182cf448e1731768e246e1a9a2347841d1077e7eb923436ad0e5820398dc3ab8679c82a199ce3daa4252366de1e7c84e84be107444e7aa083a4c46f5822a101581e581c3083349abf53ea824967b5502a6abd083a41a4e20b2dd69c6bc98377a100828258204d42e2c5bdc4d664b48bd04b7bf5b7d5b7e5aefc51b9fe883c1ea84f3f96c4275840fc34da55fde96f26af213b227fcd8a63d71df0ef7ac5c44fd45a01d407a3935b3e5c925ffa904b7298398d795f6f5e3d9f077dcf2cb99de2ef450bb742f0c40e8258205cd899fb94213b3fe6bbdfb43bc99455b3caa7fe15ed492abfa7183ad18dccda5840783eee39faede9c9e7f3957837ef1c6e0e0816b6f09d9ea2d80e530bb1c5003d409317ef2d12f49950bde0f5a68fd1ec4fa13b702e03f5bbe4c3ae42c94daf04a100818258204e239dc6f6169577d9074ae152507487ac59c5d3cde476353216508f2e00082a584012a5637bb0e8c7ffd7ab5cbb8bba9d6192d9f79c485d4b4fca82555ee6c97661cf21637d47d4fee25cccdae785e7490b9eb9ae2307d6c3f8352f36de4b28270ca100828258207a1db219789f33e3d6f0934b1d73b40fab4fc3cbdec8f819c690a047fef785b15840904a03a4941644fea2bfc8dbe4e1986968db634c98960d2e768f727b837cc686e00a55b93447287e8150d135c8489acf09541cc2b0eebc97df38395ee6d3e50d82582021907b3e7afeef914eaafe287ff8dd5e498379dd0fb9c7c7e9e645b77cd18e345840fa568cc76bc929dfaf6db760456d56b539bb8b0e5c94230b0a30d262e7951c61a04009a778b9ab5985ddb618ede802ad07ddac13bf2063c0188f97c73be4e301a10082a219ef64a301582095b1d64fbf76f17b1920a34d14fbca1f5ab499ea59eac37a8117d5e6b2e09605025820f3157c8eda34976620ad12e0979b2d3135a784c5d6a185878987143053c17d1c035839012c152eaa9e68dd7123a3054190dc987a24e50f1ab389c44a0c7a4089beb4d4d62d8f0dce5d745df4a670998aa20f54703b2bdc7a00b7d3d219ef65a1015840897063bdeab54d2e0586529909f20b42447bfaccdfb9988d2558896baf82a37f43c2fa4ae4240f5761e3dccf9523d7305d728f21dee4491e02373de6b14f7e0780"
