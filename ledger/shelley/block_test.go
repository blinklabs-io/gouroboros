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

package shelley_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// https://cexplorer.io/block/2308cdd4c0bf8b8bf92523bdd1dd31640c0f42ff079d985fcc07c36cbf915c2b
//
//hash:2308cdd4c0bf8b8bf92523bdd1dd31640c0f42ff079d985fcc07c36cbf915c2b
//slot:16156972
var shelleyBlockHex = "84828f1a004d4a6f1a00f6892c5820bc4766a289bb5d8ec86647a0aeed5dc3521f43db7a72abef3eed1debbfa9412f5820ddba672a2abc65da47537df8f190ba376512bc0283e731e3934b52f1a0abc1a558206abad5392188997c5e14b9f6e581129ec141cec17d1d2f0cfc1e9192d464716e825840df9856406b92387c9b138ef24a58d8c48a37bf5be8721eaec4702ca98df48edd2678f0a8c722982d354582b5da65d849f72077c880778982f71de60536c3ed4a5850edc5d9f0ea6965edf729548ff7cba2cea82425ceb4e42d1074a726cc9812eb5fcabb4aefa7f049bcc5209edbbe18763a080eb1a0bdfd8fb933a1763813dd3314c229847a880116bee73c97e24130f1058258400009fa6d8e070a89d1ce903f49868682ff8dec52a0756ef60cb99226b36ab781fa8e0fa296b286b964d9cac56517071501716b752dc11636a5f9b8c29193012d58505a32d1a3ce3a8a89f17c73aedbab2c299a52f90fd78adf78fe6b5ac8cfded66ebd6d2fb238a770b6d0f022954f59855138c8ec47dc1021b8b69dabf26592076ffd612d04bfbc8fbcfdaade7d7eda670f19089958204847b0787633b18df8724c920c7cc35172531f2756fe4d6ab17489e019c2c4f358201ee99223a40caf33ca63758235af29c1d53066a423249f5690c8f9e664924dcf00186158404e4b58717c4f894e34c41d5b3d68de6676f78d9a5d719a077e282974cb136bcc6287d3fbf76e175a76edce3eb934afcf7005f5a7cc9204adf14fa9d01c123a0902005901c09b1bb23288fa4ba5f8ad23d00d0d945e89958057c17df6557a37a331867a6ef252a7893bfcf7006e32a614a1ff9787e7af3575cfdeb5054214ded324fa850b090f292bd54599e88bd4efe12b49d520eb7a8e57270dbd532dc567fdb8e5cfc386e1e28c1812734dfcde9dede86845d8e66966089f47b410225886e125942b445f912d70e517d175e9ee6f4de13f0aadf3a16e01911875df1710b49ea063d17228297c9eabf6cf663e7f5f5e58f6e86bfe7117381070c86b42981bcc7d405a49108fdd760f61815e4957ad256136706e94587ccac9ded8f9459b9433d0b6d06df5e3a821a715ee5fd912bb63d26aa1365ddf0dbff435ab42252cb2e9549f41bda074201f824cd48b6dbc8c9d79961b9c7f0cab85caa2bedef282c585ee0cad01b771a48f6740e0115acc27b4a8bb998b0f05de1cfef0b2507cd2f4d65120389fb88513e03c8a1709628faf3e48bb3956de316198ee06d45ce84b218632dd53f3ab0087681c90287c14c37ad76b17bbbeb68bb4924363987355859d6b6098418b2117b01bab13a786555d4e996dff6e1510f2e11070aa9223a229f69f5fd503de22b09052cb6e4f371f8a8858258df1cc5ebde829a8befce981f4fbaae621f8917e86a5008182582045e8b0edbf6c930ba7a039e69194f0919655d6af8ed6e4d11f2a4d418f431bcd0001818258390101c06963664878bdac7fe9fb08ddf6f06807f5f354b862796b2dff106328fa4432ae48b03992c0917b9e56e1a23c3853deaea5c08ce90cfa1b000000160737188b021a0002a9e5031a00f6904f05a1581de16328fa4432ae48b03992c0917b9e56e1a23c3853deaea5c08ce90cfa1a0411647da50081825820264700b263e5adbd22757cda813ef4697317e44abfc1eee40e0af6bb3272495e00018182583901065b260095b86db3974361a08bfda1521f02698d02a57dcb135c1db24abee9a72b7daab743ccfac94aed66a24ef7a551282084cf700b7b861a0cb835a1021a0002a389031a00f6a521048183028200581c4abee9a72b7daab743ccfac94aed66a24ef7a551282084cf700b7b86581c3116c834a09b0060aef7284f63d3275456364e3309b3c19ec328af60a50081825820b5b1a6019e71ed2c9715ed18e597775e72f18c0f46fee5b060466fe90f36fb0503018182583901c34fba339a35ef4a2f9a3947a819a607facb331358cc062430c3bd22fdfef9b7fe07be4d5aa048ed971dd3d52cc0602f56f158dbbc16d6b91a3842b87c021a00062764031a00f6a49f048282008200581cfdfef9b7fe07be4d5aa048ed971dd3d52cc0602f56f158dbbc16d6b983028200581cfdfef9b7fe07be4d5aa048ed971dd3d52cc0602f56f158dbbc16d6b9581ca3ac6ca0694fc5825f831c8d0d97f202c4f298741ae60874730fafb8a4008182582080266661df8a01b93e9e20de7232e3b34b609a1ee4a630442db9209485d2293300018282583901491f94fa0985f2c34ccfb94085dcd9763c8eb947737dd380b52e37d8ab93b30d186d41b7b9bc22c336fdbb75dd1ab1ddf58ee9ba906bec821a713fb300825839013ada263c5ccda43559b21a9cd59c1e24f36993d8c579e1d7e8815aae7704a3cd5ae21fed6914ec6fc9d9879ee34008c541fa00d9ae571f5f1b000001748b231e6f021a0002a751031a00f68b55a500818258202476bfa426a7f5eeef94edb037b8a6cc6a42e253847d1aeb18c01a5842ad7dd900018182583901e2f94469a819e0f1a8524ad1575905f4c9aca0a1eafe52bfe4c74970f8d85dee99bffa2b5c68d591423d64b0940b6efc767ca94713efbc8d1b00000042309c8c8d021a0002bbc5031a00f68ce505a1581de1f8d85dee99bffa2b5c68d591423d64b0940b6efc767ca94713efbc8d1a3bea1c2ca5008182582052022c25dc42eaf28e2a9b3590d5bf1c5ae36107a06be1e9a0c6d42d1fc6b05100018182583901fb4244a608712927556cff2a1ef1128419da71f56665e2079e8b2c776b2e4ac9a9478dd18ce4585cb931304344d658172ac621aa98acb1be1b000000427bfd536e021a0002bbc5031a00f68ce505a1581de16b2e4ac9a9478dd18ce4585cb931304344d658172ac621aa98acb1be1a31ae080986a10082825820183e981064859999b673c07c267776004cecb190ff878dd98ad151f98eb1838458403370d27e2db1985bf798f968251611cc59e52341d9a3b0b2f517081e655c8398057d0c10af339b62a54038185bb9d7208bd5475fda792274c746c78c10dd3a00825820f9d6360f1fb984311692e20628bcd4755f86c8f78289e4e8b510265cce8719695840b2ed58bedfe4b8bdca7d0a87b0213ffc37a6b23bcb112592211f3152e05dacb6f021eed3e9af3ad8e6d50dc8c89f93a16e343e23186bbff50e02b33aa81d6c03a10082825820554c24c692dbab0bbc0a26fa6b654ee4455147bb8bab4c7c53cddaf20279f0f95840aa3c322564e2e200d9adba2f5de58fedaf3925c14176640980db56470e194368dbcfb06bcf6a7afbb80f79d2be64f084c03ceac63d5d9bb4ef738606dd2afe0c825820e2f26a35abec7298e8db510f792cef005eb287c52c15951b44f8463360acf56c58405c476c8e8cd600bb4489176af84b27f353cfdc51953f52127bf0870ab88ec637a8e352125c4e81a70a3a19afd462875f9be73cda762069740a1580257b49150ca100828258207892557e03cedfa4ea735334f0e46dd5a72208a920c4f3bddc4d0721eab5456b5840e7a3589732148670db6ed45b879564aeec70cce65e7333b4c313682d0b50c10d1ad78e39139824de18d7f5e11ab51b6c4d1867628362d2e919b347578134180482582054da01bb9c34c1881c8e1db7355acb8b861d3e41f5dca8b1188cbea58e296ba15840cd397780a3740c9325887610f5fb9d332d5a37cbdf5f228fcb5430f76e490233f5e164d8779df2409b552a3dd4fc67ed3cd06013037d4a715ad25441ea726600a10081825820e9f3aaf15c651471ae0d5fe633a0e995c52a7d10f1b842ca2ccee768e64c5c0f5840664e923c5ee2cf5bc3aab03c2ccd963c11606014b8934548f65bb449c396fe7aa737c0758ca5c6bbb0efc7b6b3d27f1b3089b7144eff291b641b48ccf0d10e03a1008282582004b6f122a042a97edded60ed15e5445964a58586900222f1dac365b8a63eb3fb5840bfa14d179ce2a25ca1e3dafe21f785da0a8d8b187370e4b42e98fbee779e26fddccf61cd1895a1723e5ffe53b09fd1b66f42a463427ec511e1f094de65a40503825820284389d7720675f845221d3a00920c4bb26185ab9f3f92afcefa21411e01a5855840e30ab960e666179bf89f76d55d6dd180d4d54d1da83de4be42257dcfc7736f5ca681c6eb3da4fb201b75a25682530c85ed4edd3083bcdc577d57ea0d78b8f901a10082825820616336fa9a07c4fceae37c66ff60ef25f739ce9d5815ac767f8f600f2dc82e885840efcbcd490b2e93889b68ac92228bb8436022ec2ba889b68f69e7f896c49202687edeadd4f225a2228f81dada70620e4428619d6c2aca9d33c23bd1331abf2e0f8258205d9efd320d1f90ecccd70fdd9caa328e3e589914cecb956cc93f9b5fd30b911258404bb7713750e71feda14f3f85bc3d11533eed9dac792812e3106e8f314c8a4f1a0b3797b372e27a2b0bb4a05de30ae27a298cf658141aa2ac26b4847066f2870ba0"

func TestShelleyBlock_CborRoundTrip_UsingCborEncode(t *testing.T) {
	hexStr := strings.TrimSpace(shelleyBlockHex)

	// Decode the hex string into CBOR bytes
	dataBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf(
			"Failed to decode Shelley block hex string into CBOR bytes: %v",
			err,
		)
	}

	// Deserialize CBOR bytes into ShelleyBlock struct
	block, err := shelley.NewShelleyBlockFromCbor(
		dataBytes,
		common.SkipBodyHashValidation(),
	)
	if err != nil {
		t.Fatalf("Failed to parse CBOR data into ShelleyBlock: %v", err)
	}

	// Re-encode using the cbor Encode function
	encoded, err := cbor.Encode(block)
	if err != nil {
		t.Fatalf(
			"Failed to marshal ShelleyBlock using custom encode function: %v",
			err,
		)
	}
	if len(encoded) == 0 {
		t.Fatal("Custom encoded CBOR from ShelleyBlock is nil or empty")
	}

	// Ensure the original and re-encoded CBOR bytes are identical
	if !bytes.Equal(dataBytes, encoded) {
		t.Errorf(
			"Custom CBOR round-trip mismatch for Shelley block\nOriginal CBOR (hex): %x\nCustom Encoded CBOR (hex): %x",
			dataBytes,
			encoded,
		)

		// Check from which byte it differs
		diffIndex := -1
		for i := 0; i < len(dataBytes) && i < len(encoded); i++ {
			if dataBytes[i] != encoded[i] {
				diffIndex = i
				break
			}
		}
		if diffIndex != -1 {
			t.Logf("First mismatch at byte index: %d", diffIndex)
			t.Logf(
				"Original byte: 0x%02x, Re-encoded byte: 0x%02x",
				dataBytes[diffIndex],
				encoded[diffIndex],
			)
		} else {
			t.Logf("Length mismatch: original length = %d, re-encoded length = %d", len(dataBytes), len(encoded))
		}
	}
}

func TestShelleyBlockUtxorpc(t *testing.T) {
	// Decode the test block CBOR
	blockCbor, err := hex.DecodeString(shelleyBlockHex)
	if err != nil {
		t.Fatalf("failed to decode block hex: %v", err)
	}

	block, err := shelley.NewShelleyBlockFromCbor(
		blockCbor,
		common.SkipBodyHashValidation(), // Skip validation since it's tested in verify_block_test.go
	)
	if err != nil {
		t.Fatalf("failed to parse block: %v", err)
	}

	// Convert to utxorpc format
	utxoBlock, err := block.Utxorpc()
	if err != nil {
		t.Fatalf("failed to convert to utxorpc: %v", err)
	}

	if utxoBlock.Header == nil {
		t.Error("block header is nil")
	}
	if utxoBlock.Body == nil {
		t.Error("block body is nil")
	}

	expectedTxCount := len(block.TransactionBodies)
	if len(utxoBlock.Body.Tx) != expectedTxCount {
		t.Errorf(
			"expected %d transactions, got %d",
			expectedTxCount,
			len(utxoBlock.Body.Tx),
		)
	}

	if utxoBlock.Header.Height != block.BlockNumber() {
		t.Errorf(
			"height mismatch: expected %d, got %d",
			block.BlockNumber(),
			utxoBlock.Header.Height,
		)
	}
	if utxoBlock.Header.Slot != block.SlotNumber() {
		t.Errorf(
			"slot mismatch: expected %d, got %d",
			block.SlotNumber(),
			utxoBlock.Header.Slot,
		)
	}
}

func BenchmarkShelleyBlockDeserialization(b *testing.B) {
	blockCbor, _ := hex.DecodeString(shelleyBlockHex)
	b.ResetTimer()
	for b.Loop() {
		var block shelley.ShelleyBlock
		err := block.UnmarshalCBOR(blockCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkShelleyBlockSerialization(b *testing.B) {
	blockCbor, _ := hex.DecodeString(shelleyBlockHex)
	var block shelley.ShelleyBlock
	err := block.UnmarshalCBOR(blockCbor)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = block.Cbor()
	}
}
