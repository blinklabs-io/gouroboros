// Copyright 2025 Blink Labs Software
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

package ledger_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
)

const (
	byronTxCborHex   = "82838080a080"
	shelleyTxCborHex = "83a5008182582086e5ea8e926955f7b35e94c7a8a7ce9837928752bdf7c2a099bfeb56de0f9c6e01018182583901beea25bc9b21865d0b6fd7b61033cea4e5ab97378fcf53305f178b581c644bf11e7528a7e230125a8b7a1dbceea8ee033aea783fd94e3be71b0000000e15f87186021a001c4b7a031a0044c57804828a03581ccc1008a2bff95e4fa408a8d8d4adfac342120307e0fb3c713fce88345820df5b4f160800ea24e4e328b12e0598653591330f0afa86b71d081ba21be777d71b0000000df84758001a1443fd00d81e82111901f4581de11c644bf11e7528a7e230125a8b7a1dbceea8ee033aea783fd94e3be781581c1c644bf11e7528a7e230125a8b7a1dbceea8ee033aea783fd94e3be7848400191f9144d161aec5f68400191f9244d161aec5f68400191f9144640b0c2ef68400191f9244640b0c2ef682782268747470733a2f2f6265747465727374616b652e636f6d2f62706f6f6c2e6a736f6e5820b5b2bc313684b4410d091b374b42a18701dde1b1365037490f94d8c5b473d42f83028200581c1c644bf11e7528a7e230125a8b7a1dbceea8ee033aea783fd94e3be7581ccc1008a2bff95e4fa408a8d8d4adfac342120307e0fb3c713fce8834a10083825820722391106ef912f0cdbe2e1353b9c370fb55e8d197ff9e97662ae95cf21a32a95840b70df72a21791ac1648c83b926c7604623e81696d857ce3da48ad0a1f3a8c59787a9104268eba8cb0420db34c5b4a2c0219c7a6e7b6f274a824653ebd797bd028258204cd4ea9234d75f088b21015e6f15b4aca66cd02394678e9d9563dc6d89305190584012cee31daa17c580bc0b8b0462b24cc2589a8fdb74005baa04022349e6257909d42ea10ba2e7fbfbe8974f74bf564c1aa2e1fe5ff36eab9b958f296c7d8db00682582050b9afde3642819ddad24be4bfbf9b7b08b00aa1254b4f9eb9bce779e977cb5258400097a1b083ff4fe74edb7486c7a2dbcbfccf83bacb819b7fad7bb3849575dca41f516bb77b8c4c78420704774658f7654d7c4314793daaccbdb68689c2bd8003f6"
	allegraTxCborHex = "83a500818258207c685f9d47332cee3c54afba19b6e2bb8ab402c4bb29cef2353ec178ced2501b010181825839013181113a038b59ab33a32e4db71e57562c08183c95fe0dcdd9600068ba98d33801b8794c5b20efe7f740a43a1c82e9ca02f10f6c20ffa5e81a04ce7af2021a00032acd031a00fd221904818a03581c6d9ce533e4874ff84f89a0f1d2f8a26c0838898ba16fcf66aa790c9e58203d5023f152e4c230c71fe09165ea754b484cfc7557fa8fd48b792cbf2edf42271b000000746a5288001a1443fd00d81e82011828581de10122c6ccb1533c9992ff08a9029b7b5a016d5c08d31a4963cd027afc82581c50980d844ceff6882f9d56fa94bc21c932e336c4136df1ecc91951a1581cba98d33801b8794c5b20efe7f740a43a1c82e9ca02f10f6c20ffa5e88183011917706c36322e3131362e3138322e3682782a68747470733a2f2f377275332e636f6d2f6d657461646174612f747275655f3038323032302e6a736f6e58207778c9ba2c70915cbd67783d0a1d20df2b16469ec3b178b90ec2aef4ddceca35a10085825820dd767642628c78208833b8e3fae2e2896fda0293eecbd1a13bfbef7b088462ed584058a59c7f50b28e36759e10e6616ddffb245d21256111d9d7c50a91491301786309470f1714026d2e951562c40fbc7877826238a8b2260b4aeeee456e56bbc108825820ae621ba88e58d56cefd82f454360b33527bf2b8b958ce0392bae644bf444dbc55840f7a42f2afdbcc5176fdcdb28de9d723c0e38290381a65468cc4d223d73bed2893e4fe87a71564eb8a6f9367a1e86dc034aa35fcfa44d1530dddbfc762522fa0d825820cecd648d4e4bd55d92eb0e8d28d217a65b0d1d609b904b28ef77d1ba6edd26e358403e7d28b9686b1af9d9c004e7d0cdb9618e8356a1ec4b61231b9c71e4c673aa2cb6ad689ac1d941ed5878113fc792f3d1e49c0ccd9d09ea69b776fe49dfbb39028258206a6be24f4c436aebea0fce9a3ed03666c4d66fbd66857f7ff8504a44061642485840e4c009d38b9d2c86ef3c019b8d3d9d973e108035bb41a71c71f44e682631ed095972075e2a0daf607b12f5bc414d4fb343f34b1752299e84b68233dccf53f90882582009fbfe452aef3d3ad946e45192a676761e8cb408f49636531272c180dec0364b584081cb9a113aa5fbd654bcb7d34df0e8b8340ba03bdafdb69d0906e817b7da0d0b6a6cba882fb4424579a614d13466115d905f9558a9d947e495dbd22cbc14d409f6"
	// CSCS pool registration
	maryTxCborHex    = "83a50081825820377732953cbd7eb824e58291dd08599cfcfe6eedb49f590633610674fc3c33c50001818258390187acac5a3d0b41cd1c5e8c03af5be782f261f21beed70970ddee0873ae34f9d442c7f1a01de9f2dd520791122d6fbf3968c5f8328e9091331a597dbcf1021a0002fd99031a025c094a04828a03581c27b1b4470c84db78ce1ffbfff77bb068abb4e47d43cb6009caaa352358204a7537ce9eeaba1c650261f167827b4e12dd403c4bf13c56b2cba06288f7e9ab1a59682f001a1443fd00d81e82011864581de1ae34f9d442c7f1a01de9f2dd520791122d6fbf3968c5f8328e90913381581cae34f9d442c7f1a01de9f2dd520791122d6fbf3968c5f8328e9091338183011917706e33342e3139382e3234312e323336827468747470733a2f2f6769742e696f2f4a7543786e5820fa77d30bb41e2998233245d269ff5763ecf4371388214943ecef277cae45492783028200581cae34f9d442c7f1a01de9f2dd520791122d6fbf3968c5f8328e909133581c27b1b4470c84db78ce1ffbfff77bb068abb4e47d43cb6009caaa3523a10083825820922a22d07c0ca148105760cb767ece603574ea465d6697c87da8207c8936ebea58405594a100197379c0de715de0b5304e0546e661dae2f36b12173cc150a42215356a5600bf0c02954f02ce3620cfb7f12c23a19328fd00dd1194b4f363675ef407825820727c1891d01cf29ccd1146528221827dcf00a093498509404af77a8b15d77c925840f52e0e1403167212b11fe5d87b7cfdb2f39e5384979ac3625917127ad46763d864a7fcb7147c7b85322ada7ba8fe91c0b5152c74ef4ff0c8132b125e681af50382582073c16f2b67ff85307c4c5935bad1389b9ead473419dbad20f5d5e6436982992b58400572eed773b9a199fd486ebe61b480f05803d107ea97ff649f28b8874d3117f890f80657cbb6eea0d833c21e4e8bc7f1a27cddb9e24fc1ed79b04ddbdcd11d0ff6"
	alonzoTxCborHex  = "84a400838258203650540ca26b22596543862300899b7e51af75f933dd70c8bc717e6f9346d5bc018258207cd6c8a610c6b89782b05266138840491def8a5b1dedc169eb5f48f83c811c5800825820a90046b2c507d970350dd101f80cae88099eb9c16ff58dc2ca3141cebe14b7d601018282581d61a17caf4da8995968c9afa0baf2d5b53cdcb9ce00e9f6fcb442b3dae61a002dc6c082583901f361a13e63dfa5a0e9ea26320b27bca5ab944564c823425841c075f2c342a4c6c2ddb549715b4c6e58b36043639297c0b3d2c381562865b0821a0dd83fa5b4581c01e3c1b5e7ce94a556bead799ca036bc0c84bc16674dbeff6f2f04e0a14f4242426574614c61727665314e723301581c0d9cb6eff0cb4e8df5b9b44a4af445dd1245d48eaa6b9be04482c2ada14f4242426574614c61727665314e723201581c11cca1d26fc711f4119122dcafbf371d014aec3fb94aae5b18060694a14b424242657461456767303909581c36ef74427e125936104d8aeb85e19c25b4b7cc610a39f9092e755178a14b424242657461456767303308581c5de5d32c2ad78fcd07444a859a6ee13f53a026271773396340c4252ea14b424242657461456767303809581c6377c2c3e02818ec06daa91d27c98dfc2f9920c43d39c172fe294b41a14b424242657461456767303509581c6e6c6a46823e4ac4a61a9bcdcee5a32cb23a525c31792e63248bbb46a1504242426574614c6172766530304e723201581c716c311894af097cc07b4be39ae56153a995d5a1b268eda0eecfb8e1a1504242426574614c6172766530304e723701581c82c7122974aff4a6d9f26162898d5d9847bdd205f0ff8e6dc05c3b47a14d4242426574614c61727665303001581c835fade7ad9e2fb1938a807185c7a056086b70b6799392c31e7b7248a14b424242657461456767303409581c84f306c414a9a54f06045aedf1729e7166dd617bc66cb7a4ffe860a8a14b424242657461456767303209581c87921b425a0467f6b91e8ff67f94139f06836f1bd1a92711eb64d3bba1504242426574614c6172766530304e723601581c92b68843d0b714c0c5dfbdd11780f356f9a40eb189b38bb7f73b875fa1504242426574614c6172766530304e723501581caa093a4d6922ef15ea9d25d3382d1081b6d64731db8e47a74bff26f0a14b424242657461456767313009581cb71cecdd7dac01afb2dcf562311bcd2023690aec9b4634cd92f0a1bca14f4242426574614c61727665314e723101581cc4e9a75d751901fc531705fb6a19af8116bc33a7aa34496878cda14da1504242426574614c6172766530304e723401581cccbd38f09601415028647dd65895d27ba635ce9ff3fe49437596ff52a14b424242657461456767303709581cd2f7ab6723239cf8095ea6865b7d24f641ba011763336450641199a1a14b424242657461456767303109581ce19178a4757457dd76be8a12b51cda5f9fde90ef9e4ffce0912b51a8a14b424242657461456767303609581ce1b91ebf52575643fd8ee4963065a3284723501d65355ef2e1f32cd3a1504242426574614c6172766530304e723301021a00038115031a02613161a10083825820cc6834b6867893cb053c638622e2b574ebc9a6183c6b521431d58fceec155a7e58409f1cfeb1377f6c6ad27ab71f6e7ab993fab537a7855e92163433b817da34f310660584930fa5506a364471a0c8d069c906ccb63d10c32b94f00996f1be681c0d82582074b4821f97da974ca2b4c7b14421e587a1eddf90ff9b914507e4011fe8d3409d584077ad90d93a2ffc410fa5502056039073d4c270fdbde7c77d81e8e6ffaf22a2197e613838e0591fd176917cc2df1f1f97fc256ed15a70247fa5f2c24168b5e10d82582021f8ac17f9995e8334da3c5dce41bec5cd28019c98d4f3b79bb18138867e593c5840efcd97da471dd6e2337564e17158bf54a094ee999b860600e0ece23d135d191bbdc66466faf801714bac1ddbb454375f477246b01baf535cebde843bcb03cd05f5f6"
	babbageTxCborHex = "84a40081825820e8f54ff4cfcb14a11995995e99b8445b977badaf1f13130ba7f9140e2d8ef40f01018282581d6124d274bfd913b241a8cca20c6977775718fddb8f3763f1d365c854451a001e848082583901c7cfad58a5cbce2a0460f304a3b47df1a3f5ad17f118ed1b07c929c1245fce8a5f9aa0ccc732a20a39630205a74e2500014544b80d7c62c51a009f1df5021a00028cad031a050ef54da10081825820f9e738c0a455f79529e807e2382da4392d8c57636ca1fccef9414186af07e05958406a219088d2b13beff7006780f210b403666e3f20e04a4a3dc560091e255e5e5c2366a0187936026a63017b23c3c498801475c718edaccde0b3c568557052ad0cf5f6"
	// Amaru Treasury Withdrawal Proposal
	conwayTxCborHex = "84a500d9010281825820279184037d249e397d97293738370756da559718fcdefae9924834840046b37b01018282583900923d4b64e1d730a4baf3e6dc433a9686983940f458363f37aad7a1a9568b72f85522e4a17d44a45cd021b9741b55d7cbc635c911625b015e1a00a9867082583900923d4b64e1d730a4baf3e6dc433a9686983940f458363f37aad7a1a9568b72f85522e4a17d44a45cd021b9741b55d7cbc635c911625b015e1b00000001267d7b04021a0002938d031a04e304e70800a100d9010281825820b829480e5d5827d2e1bd7c89176a5ca125c30812e54be7dbdf5c47c835a17f3d5840b13a76e7f2b19cde216fcad55ceeeb489ebab3dcf63ef1539ac4f535dece00411ee55c9b8188ef04b4aa3c72586e4a0ec9b89949367d7270fdddad3b18731403f5f6"
)

func TestDetermineTransactionType(t *testing.T) {
	testDefs := []struct {
		name           string
		txCborHex      string
		expectedTxType uint
	}{
		{
			name:           "ConwayTx",
			txCborHex:      conwayTxCborHex,
			expectedTxType: 6,
		},
		{
			name:           "ByronTx",
			txCborHex:      byronTxCborHex,
			expectedTxType: 0,
		},
		{
			name:           "MaryTx",
			txCborHex:      maryTxCborHex,
			expectedTxType: 1, // Shelley-compatible
		},
	}
	for _, testDef := range testDefs {
		txCbor, err := hex.DecodeString(testDef.txCborHex)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		tmpTxType, err := ledger.DetermineTransactionType(txCbor)
		if err != nil {
			t.Fatalf(
				"DetermineTransactionType failed with an unexpected error: %s",
				err)
		}
		if tmpTxType != testDef.expectedTxType {
			t.Fatalf(
				"did not get expected TX type: got %d, wanted %d",
				tmpTxType,
				testDef.expectedTxType)
		}
	}

}

func TestMaryTransactionCborRoundTrip(t *testing.T) {
	txCbor, err := hex.DecodeString(maryTxCborHex)
	if err != nil {
		t.Fatalf("failed to decode Mary transaction hex: %s", err)
	}

	// Deserialize
	tx, err := ledger.NewMaryTransactionFromCbor(txCbor)
	if err != nil {
		t.Fatalf("failed to deserialize Mary transaction: %s", err)
	}
	if tx == nil {
		t.Fatal("tx is nil")
	}

	// Serialize
	serializedCbor := tx.Cbor()
	if len(serializedCbor) == 0 {
		t.Fatal("serialized CBOR is empty")
	}

	// Check if it matches original byte-for-byte
	if !bytes.Equal(serializedCbor, txCbor) {
		t.Fatalf("CBOR byte mismatch: serialized CBOR does not match original")
	}

	// Ensure deserialization of the re-serialized works
	_, err = ledger.NewMaryTransactionFromCbor(serializedCbor)
	if err != nil {
		t.Fatalf(
			"failed to deserialize re-serialized Mary transaction: %s",
			err,
		)
	}
}

func BenchmarkDetermineTransactionType_Conway(b *testing.B) {
	txCbor, _ := hex.DecodeString(conwayTxCborHex)

	b.ResetTimer()
	for b.Loop() {
		_, err := ledger.DetermineTransactionType(txCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmarks for type determination, deserialization, and serialization across Cardano ledger eras.
// Note: Conway type determination benchmark covers attempts on Shelley, Allegra, Mary, Alonzo, Babbage eras.
// Specific deserialization/serialization benchmarks are provided for Byron era.
// Additional era-specific benchmarks can be added when valid transaction hex strings are available.

func BenchmarkDetermineTransactionType_Byron(b *testing.B) {
	txCbor, _ := hex.DecodeString(byronTxCborHex)

	b.ResetTimer()
	for b.Loop() {
		_, err := ledger.DetermineTransactionType(txCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransactionDeserialization_Byron(b *testing.B) {
	txCbor, _ := hex.DecodeString(byronTxCborHex)

	b.ResetTimer()
	for b.Loop() {
		_, err := ledger.NewByronTransactionFromCbor(txCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransactionSerialization_Byron(b *testing.B) {
	txCbor, _ := hex.DecodeString(byronTxCborHex)
	tx, err := ledger.NewByronTransactionFromCbor(txCbor)
	if err != nil {
		b.Fatal(err)
	}
	if tx == nil {
		b.Fatal("tx is nil")
	}

	b.ResetTimer()
	for b.Loop() {
		_ = tx.Cbor()
	}
}

func BenchmarkTransactionDeserialization_Conway(b *testing.B) {
	txCbor, _ := hex.DecodeString(conwayTxCborHex)

	b.ResetTimer()
	for b.Loop() {
		_, err := ledger.NewConwayTransactionFromCbor(txCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransactionSerialization_Conway(b *testing.B) {
	txCbor, _ := hex.DecodeString(conwayTxCborHex)
	tx, err := ledger.NewConwayTransactionFromCbor(txCbor)
	if err != nil {
		b.Fatal(err)
	}
	if tx == nil {
		b.Fatal("tx is nil")
	}

	b.ResetTimer()
	for b.Loop() {
		_ = tx.Cbor()
	}
}

func BenchmarkTransactionDeserialization_Shelley(b *testing.B) {
	txCbor, _ := hex.DecodeString(shelleyTxCborHex)
	b.ResetTimer()
	for b.Loop() {
		_, err := ledger.NewShelleyTransactionFromCbor(txCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransactionSerialization_Shelley(b *testing.B) {
	txCbor, _ := hex.DecodeString(shelleyTxCborHex)
	tx, err := ledger.NewShelleyTransactionFromCbor(txCbor)
	if err != nil {
		b.Fatal(err)
	}
	if tx == nil {
		b.Fatal("tx is nil")
	}
	b.ResetTimer()
	for b.Loop() {
		_ = tx.Cbor()
	}
}

func BenchmarkTransactionDeserialization_Allegra(b *testing.B) {
	txCbor, _ := hex.DecodeString(allegraTxCborHex)
	b.ResetTimer()
	for b.Loop() {
		_, err := ledger.NewAllegraTransactionFromCbor(txCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransactionSerialization_Allegra(b *testing.B) {
	txCbor, _ := hex.DecodeString(allegraTxCborHex)
	tx, err := ledger.NewAllegraTransactionFromCbor(txCbor)
	if err != nil {
		b.Fatal(err)
	}
	if tx == nil {
		b.Fatal("tx is nil")
	}
	b.ResetTimer()
	for b.Loop() {
		_ = tx.Cbor()
	}
}

func BenchmarkTransactionDeserialization_Mary(b *testing.B) {
	txCbor, _ := hex.DecodeString(maryTxCborHex)
	b.ResetTimer()
	for b.Loop() {
		_, err := ledger.NewMaryTransactionFromCbor(txCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransactionSerialization_Mary(b *testing.B) {
	txCbor, _ := hex.DecodeString(maryTxCborHex)
	tx, err := ledger.NewMaryTransactionFromCbor(txCbor)
	if err != nil {
		b.Fatal(err)
	}
	if tx == nil {
		b.Fatal("tx is nil")
	}
	b.ResetTimer()
	for b.Loop() {
		_ = tx.Cbor()
	}
}

func BenchmarkTransactionDeserialization_Alonzo(b *testing.B) {
	txCbor, _ := hex.DecodeString(alonzoTxCborHex)
	b.ResetTimer()
	for b.Loop() {
		_, err := ledger.NewAlonzoTransactionFromCbor(txCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransactionSerialization_Alonzo(b *testing.B) {
	txCbor, _ := hex.DecodeString(alonzoTxCborHex)
	tx, err := ledger.NewAlonzoTransactionFromCbor(txCbor)
	if err != nil {
		b.Fatal(err)
	}
	if tx == nil {
		b.Fatal("tx is nil")
	}
	b.ResetTimer()
	for b.Loop() {
		_ = tx.Cbor()
	}
}

func BenchmarkTransactionDeserialization_Babbage(b *testing.B) {
	txCbor, _ := hex.DecodeString(babbageTxCborHex)
	b.ResetTimer()
	for b.Loop() {
		_, err := ledger.NewBabbageTransactionFromCbor(txCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTransactionSerialization_Babbage(b *testing.B) {
	txCbor, _ := hex.DecodeString(babbageTxCborHex)
	tx, err := ledger.NewBabbageTransactionFromCbor(txCbor)
	if err != nil {
		b.Fatal(err)
	}
	if tx == nil {
		b.Fatal("tx is nil")
	}
	b.ResetTimer()
	for b.Loop() {
		_ = tx.Cbor()
	}
}
