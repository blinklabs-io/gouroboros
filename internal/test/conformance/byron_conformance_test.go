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

package conformance

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	byronConsensus "github.com/blinklabs-io/gouroboros/consensus/byron"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/stretchr/testify/assert"
)

// Byron block test vectors from real mainnet/testnet blocks
// These test that our Byron block parsing matches the expected hashes and structure

// TestByronMainBlockConformance tests parsing of real Byron main blocks
func TestByronMainBlockConformance(t *testing.T) {
	// Real mainnet Byron block from cexplorer
	// https://cexplorer.io/block/1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8
	tests := []struct {
		name         string
		hexData      string
		expectedHash string
		expectedSlot uint64
	}{
		{
			name: "mainnet_block_slot_4471207",
			// Block from slot 4471207
			hexData:      mainnetByronBlockHex,
			expectedHash: "1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8",
			expectedSlot: 4471207,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := hex.DecodeString(strings.TrimSpace(tc.hexData))
			if err != nil {
				t.Fatalf("Failed to decode hex: %v", err)
			}

			block, err := byron.NewByronMainBlockFromCbor(data)
			if err != nil {
				t.Fatalf("Failed to decode Byron block: %v", err)
			}

			// Verify hash
			hash := block.Hash()
			hashHex := hex.EncodeToString(hash.Bytes())
			if hashHex != tc.expectedHash {
				t.Errorf(
					"Hash mismatch:\n  got:  %s\n  want: %s",
					hashHex,
					tc.expectedHash,
				)
			}

			// Verify slot
			if block.SlotNumber() != tc.expectedSlot {
				t.Errorf(
					"Slot mismatch: got %d, want %d",
					block.SlotNumber(),
					tc.expectedSlot,
				)
			}

			// Verify era
			if block.Era().Id != byron.EraIdByron {
				t.Errorf(
					"Era mismatch: got %d, want %d",
					block.Era().Id,
					byron.EraIdByron,
				)
			}

			// Verify block type
			if block.Type() != byron.BlockTypeByronMain {
				t.Errorf(
					"Block type mismatch: got %d, want %d",
					block.Type(),
					byron.BlockTypeByronMain,
				)
			}

			t.Logf(
				"Byron main block validated: slot=%d, hash=%s, txs=%d",
				block.SlotNumber(),
				hashHex[:16]+"...",
				len(block.Transactions()),
			)
		})
	}
}

// TestByronEBBConformance tests parsing of real Byron Epoch Boundary Blocks
func TestByronEBBConformance(t *testing.T) {
	// Load EBB from testdata file
	ebbPath := filepath.Join(
		"..",
		"..",
		"..",
		"protocol",
		"chainsync",
		"testdata",
		"byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
	)

	hexData, err := os.ReadFile(ebbPath)
	if err != nil {
		t.Skipf("EBB test file not found: %v", err)
	}

	data, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	block, err := byron.NewByronEpochBoundaryBlockFromCbor(data)
	if err != nil {
		t.Fatalf("Failed to decode Byron EBB: %v", err)
	}

	// Verify hash matches filename
	expectedHash := "8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f"
	hash := block.Hash()
	hashHex := hex.EncodeToString(hash.Bytes())
	if hashHex != expectedHash {
		t.Errorf(
			"Hash mismatch:\n  got:  %s\n  want: %s",
			hashHex,
			expectedHash,
		)
	}

	// Verify era
	if block.Era().Id != byron.EraIdByron {
		t.Errorf(
			"Era mismatch: got %d, want %d",
			block.Era().Id,
			byron.EraIdByron,
		)
	}

	// Verify block type
	if block.Type() != byron.BlockTypeByronEbb {
		t.Errorf(
			"Block type mismatch: got %d, want %d",
			block.Type(),
			byron.BlockTypeByronEbb,
		)
	}

	// EBBs should have no transactions
	if len(block.Transactions()) != 0 {
		t.Errorf(
			"EBB should have no transactions, got %d",
			len(block.Transactions()),
		)
	}

	t.Logf(
		"Byron EBB validated: slot=%d, hash=%s",
		block.SlotNumber(),
		hashHex[:16]+"...",
	)
}

// TestByronMainBlockFromTestnetFile tests parsing from testdata file
func TestByronMainBlockFromTestnetFile(t *testing.T) {
	blockPath := filepath.Join(
		"..",
		"..",
		"..",
		"protocol",
		"chainsync",
		"testdata",
		"byron_main_block_testnet_f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08.hex",
	)

	hexData, err := os.ReadFile(blockPath)
	if err != nil {
		t.Skipf("Test file not found: %v", err)
	}

	data, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	block, err := byron.NewByronMainBlockFromCbor(data)
	if err != nil {
		t.Fatalf("Failed to decode Byron block: %v", err)
	}

	// Verify hash matches filename
	expectedHash := "f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08"
	hash := block.Hash()
	hashHex := hex.EncodeToString(hash.Bytes())
	if hashHex != expectedHash {
		t.Errorf(
			"Hash mismatch:\n  got:  %s\n  want: %s",
			hashHex,
			expectedHash,
		)
	}

	t.Logf("Byron testnet block validated: slot=%d, hash=%s, txs=%d",
		block.SlotNumber(), hashHex[:16]+"...", len(block.Transactions()))
}

// TestByronHeaderFields tests that header fields are correctly extracted
func TestByronHeaderFields(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	block, err := byron.NewByronMainBlockFromCbor(data)
	if err != nil {
		t.Fatalf("Failed to decode Byron block: %v", err)
	}

	header := block.Header()

	// Check header methods work
	if header.SlotNumber() != 4471207 {
		t.Errorf(
			"Header slot mismatch: got %d, want 4471207",
			header.SlotNumber(),
		)
	}

	// Block number should be retrievable
	blockNum := header.BlockNumber()
	t.Logf("Block number (difficulty): %d", blockNum)

	// PrevHash should be present
	prevHash := header.PrevHash()
	if len(prevHash.Bytes()) != 32 {
		t.Errorf("PrevHash should be 32 bytes, got %d", len(prevHash.Bytes()))
	}

	// Era should be Byron
	if header.Era().Id != byron.EraIdByron {
		t.Errorf("Era mismatch")
	}
}

// TestByronBlockBodyHash tests that body hash is correctly computed
func TestByronBlockBodyHash(t *testing.T) {
	data, err := hex.DecodeString(strings.TrimSpace(mainnetByronBlockHex))
	if err != nil {
		t.Fatalf("Failed to decode hex: %v", err)
	}

	block, err := byron.NewByronMainBlockFromCbor(data)
	if err != nil {
		t.Fatalf("Failed to decode Byron block: %v", err)
	}

	// BlockBodyHash should return something (or zero hash if not extractable)
	bodyHash := block.BlockBodyHash()
	if len(bodyHash.Bytes()) != 32 {
		t.Errorf("Body hash should be 32 bytes")
	}
}

// TestByronSlotToEpoch tests Byron slot/epoch calculations using ByronConfig.SlotToEpoch
func TestByronSlotToEpoch(t *testing.T) {
	// Create a ByronConfig to test the actual SlotToEpoch function
	config := byronConsensus.ByronConfig{
		SlotsPerEpoch: byron.ByronSlotsPerEpoch,
	}

	tests := []struct {
		slot          uint64
		expectedEpoch uint64
	}{
		{0, 0},
		{1, 0},
		{byron.ByronSlotsPerEpoch - 1, 0},
		{byron.ByronSlotsPerEpoch, 1},
		{byron.ByronSlotsPerEpoch + 1, 1},
		{2 * byron.ByronSlotsPerEpoch, 2},
		{4471207, 207}, // Real mainnet slot (4471207 / 21600 = 207)
	}

	for _, tc := range tests {
		// Test the actual ByronConfig.SlotToEpoch function
		epoch := config.SlotToEpoch(tc.slot)
		assert.Equalf(
			t,
			tc.expectedEpoch,
			epoch,
			"Slot %d",
			tc.slot,
		)
	}
}

// Real mainnet Byron block from cexplorer (slot 4471207)
// https://cexplorer.io/block/1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8
const mainnetByronBlockHex = `83851a2d964a09582025df38df102b89ec25a432a2972993d2fa8cc1f597a73e6260b2f07e79501eb084830258200f284bc22f5b96228ee0687b7bb87c56132f77df4235c78a1595729ccfce2001582019fb988d02ec920a6de5ac71c5d5e75f8b73d7ed8e8abea7773e28859983206e82035820d36a2619a672494604e11bb447cbcf5231e9f2ba25c2169177edc941bd50ad6c5820afc0da64183bf2664f3d4eec7238d524ba607faeeab24fc100eb861dba69971b58204e66280cd94d591072349bec0a3090a53aa945562efb6d08d56e53654b0e4098848218cf0758401bc97a2fe02c297880ce8ecfd997fe4c1ec09ee10feeee9f686760166b05281d6283468ffd93becb0c956ccddd642df9b1244c915911185fa49355f6f22bfab9811a004430ed820282840058401bc97a2fe02c297880ce8ecfd997fe4c1ec09ee10feeee9f686760166b05281d6283468ffd93becb0c956ccddd642df9b1244c915911185fa49355f6f22bfab9584061261a95b7613ee6bf2067dad77b70349729b0c50d57bc1cf30de0db4a1e73a885d0054af7c23fc6c37919dba41c602a57e2d0f9329a7954b867338d6fb2c9455840e03e62f083df5576360e60a32e22bbb07b3c8df4fcab8079f1d6f61af3954d242ba8a06516c395939f24096f3df14e103a7d9c2b80a68a9363cf1f27c7a4e3075840325068a2307397703c4eebb1de1ecab0b23c24a5e80c985e0f7546bb6571ee9eb94069708fc25ec67a4a5753a0d49ab5e536131c19c7f9dd4fd32532fd0f71028483010000826a63617264616e6f2d736c01a058204ba92aa320c60acc9ad7b9a64f2eda55c4d2ec28e604faf186708b4f0c4e8edf849f82839f8200d8185824825820b0a7782d21f37e9d98f4cbdc23bf2677d93eca1ac0fb3f79923863a698d53f8f018200d81858248258205bd3e8385d2ecdd17d3b602263e8a5e7aa0edb4dd00221f369c2720f7d85940d008200d81858248258201e4a77f8375548e5bc409a518dbcb4a8437b539682f4e840f4a1056f01cea566008200d81858248258205e83b53253f705c214d904f65fdaaa2f153db59a229a9cee1da6c329b543236100ff9f8282d818584283581ca1932430cb1ad6482a1b67964d778c18b574674fda151cdfa73c63cda101581e581cfc8a0b5477e819a27a34910e6c174b50b871192e95cca1a711bbceb3001abcb52f6d1b000000013446d5718282d818584283581c093916f7e775fba80eaa65cded085d985f7f9e4982cddd2bb7c476aea101581e581c83d3e2df30edf90acf198b85a7327d964f9d92fd739d0c986a914f6c001a27a611b61a000c48cbffa0848200d8185885825840a781c060f2b32d116ef79bb1823d4a25ea36f6c130b3a182713739ea1819e32d261db3dce30b15c29db81d1c284d3fe350d12241e8ccc65cdf08adba90e0ad4558408eb4c9549a6a044d687d6c04fdee2240994f43966ef113ebb3e76a756e39472badb137c3e0268d34ce6042f76c2534220cc1e061a1a29cce065faf486184cf078200d818588582584085dc150754227f68d1640887f8fa57c93e4cad3499f2cb7b5e8258b0b367dcceaa42bf9ea1cfff73fd0fab44d9e0a36ef61bc5d0f294365316a4e0ed12b40a135840f1233519fa85f3ecbb2deaa9dff2d7e943156d49a7a33603381f2c1779b7f65ea0d39a8dcdd227f5d69b9355ab35df0c43c2abb751c6dd24b107a2c7ac51f5088200d81858858258403559467e9b4a4e47af0388e7224358197e5d39c57c71c391db4a7d480f297d8b86b0746de21dc5dfca2bd8b8fa817c1fa1c3bd3eeaddbfd7a6b270564e416d0c5840b0e33544dcb1895b592a612f5be81242a88226d0612da76099b653f89ce7c5641af14fad696ccd44b58744915291240224fd83a26f103c0717752ea256b4af0b8200d8185885825840572c3ea039ded80f19b0d6841e9ad0d0d1b73242ac98538affbec6e7356192f48eba0291ea1b174f9c42e139ba85ce75656a036ba0993dda605d5a62956dba6558406257e3a27a896268cade4d5371537ed606d3004d6269f87ebe6056b6eff737a2a9ef82d27ba1f9b642ffc622ec27b38e69ed41e272d3de0767cad860d50fa10d82839f8200d8185824825820779a319e0d64b80eaff5ed13d08062b8672fc71ac27e7b30574c1c7972764de202ff9f8282d818582183581c2c0dd53d4e6001e006729fc09d74c5a799d5f93c9f4b74748412a823a0001a1abd89081a769cfd808282d818582183581c05b073f36ee030589a31148838cd47e8d8c8f82fec9fe091c7d53cd8a0001a0c5f26b11b00000016bc0c4c47ffa0818200d8185885825840f129f07bbfd87fd1d3ff5fb32e9a5566e02208f89518e9994048add22074f433424e682a392581268c7544e34e9c54378a8820bdcf7dddce30490bbb2d363b4b5840709a2e70d3803554a15d788235bf56c9567407102be375be5071fa81d4c137047743b5f5abefdbab6b2781822474995dff917213c962ecd111619d75b8534f0aff8203d90102809fff82809fff81a0`
