// Copyright 2026 Blink Labs Software
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

package bench

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// Default Cardano mainnet parameters
const (
	// slotsPerKesPeriod is 129600 on mainnet (1.5 days in slots)
	defaultSlotsPerKesPeriod = 129600
)

// Mainnet eta0 - epoch nonce (using a realistic test value)
var defaultEta0Hex = "00000000000000000000000000000000" +
	"00000000000000000000000000000000"

// benchConfig returns a VerifyConfig suitable for benchmarks.
// It skips transaction and stake pool validation since we're focusing on
// block-level validation (VRF, KES, body hash).
func benchConfig() common.VerifyConfig {
	return common.VerifyConfig{
		SkipBodyHashValidation:    false,
		SkipTransactionValidation: true,
		SkipStakePoolValidation:   true,
	}
}

// blockData holds parsed block information for benchmarks.
type blockData struct {
	name      string
	blockType uint
	cbor      []byte
	block     ledger.Block
}

// loadTestBlocks loads all test blocks from testdata.
// It panics if any block fails to load.
func loadTestBlocks() []blockData {
	testBlocks := testdata.GetTestBlocks()
	result := make([]blockData, 0, len(testBlocks))

	for _, tb := range testBlocks {
		// Skip Byron for VRF/KES benchmarks as it uses different validation
		block, err := ledger.NewBlockFromCbor(
			tb.BlockType,
			tb.Cbor,
			benchConfig(),
		)
		if err != nil {
			panic("failed to load " + tb.Name + " block: " + err.Error())
		}
		result = append(result, blockData{
			name:      tb.Name,
			blockType: tb.BlockType,
			cbor:      tb.Cbor,
			block:     block,
		})
	}
	return result
}

// getPostByronBlocks filters out Byron blocks since they use different
// validation.
func getPostByronBlocks(blocks []blockData) []blockData {
	result := make([]blockData, 0, len(blocks))
	for _, b := range blocks {
		if b.blockType != ledger.BlockTypeByronMain &&
			b.blockType != ledger.BlockTypeByronEbb {
			result = append(result, b)
		}
	}
	return result
}

// BenchmarkBlockValidation benchmarks block decode + KES + body hash validation
// by era.
// Note: VRF verification is not included because test blocks lack matching
// epoch nonces.  Using a dummy eta0 would cause VerifyBlock to return on VRF
// failure and skip KES/body-hash checks entirely.  Instead we call the
// individual validation components so the benchmark measures real work.
func BenchmarkBlockValidation(b *testing.B) {
	blocks := loadTestBlocks()
	postByronBlocks := getPostByronBlocks(blocks)

	for _, bd := range postByronBlocks {
		b.Run("Era_"+bd.name, func(b *testing.B) {
			cfg := benchConfig()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				block, err := ledger.NewBlockFromCbor(
					bd.blockType,
					bd.cbor,
					cfg,
				)
				if err != nil {
					b.Fatal(err)
				}

				// KES verification
				_, err = ledger.VerifyKes(
					block.Header(),
					defaultSlotsPerKesPeriod,
				)
				if err != nil {
					b.Fatal(err)
				}

				// Body hash validation
				era := block.Era()
				minLength := 4
				if era.Id >= alonzo.EraAlonzo.Id {
					minLength = 5
				}
				err = common.ValidateBlockBodyHash(
					block.Cbor(),
					block.BlockBodyHash(),
					era.Name,
					minLength,
				)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkBlockValidationPreParsed benchmarks KES + body hash validation
// with pre-parsed blocks.
// This isolates the validation cost from parsing overhead.
// Note: VRF verification is excluded because test blocks lack matching epoch
// nonces, which would cause VerifyBlock to return on VRF failure and skip the
// KES/body-hash checks entirely.
func BenchmarkBlockValidationPreParsed(b *testing.B) {
	blocks := loadTestBlocks()
	postByronBlocks := getPostByronBlocks(blocks)

	for _, bd := range postByronBlocks {
		b.Run("Era_"+bd.name, func(b *testing.B) {
			// Pre-compute era-specific parameters
			era := bd.block.Era()
			minLength := 4
			if era.Id >= alonzo.EraAlonzo.Id {
				minLength = 5
			}
			blockCbor := bd.block.Cbor()
			bodyHash := bd.block.BlockBodyHash()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// KES verification
				_, err := ledger.VerifyKes(
					bd.block.Header(),
					defaultSlotsPerKesPeriod,
				)
				if err != nil {
					b.Fatal(err)
				}

				// Body hash validation
				err = common.ValidateBlockBodyHash(
					blockCbor,
					bodyHash,
					era.Name,
					minLength,
				)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkBlockVRFVerification benchmarks VRF verification isolated from other
// validation.
func BenchmarkBlockVRFVerification(b *testing.B) {
	blocks := loadTestBlocks()
	postByronBlocks := getPostByronBlocks(blocks)

	// Pre-extract VRF data for each block
	type vrfData struct {
		name   string
		vrfKey []byte
		proof  []byte
		output []byte
		slot   uint64
	}

	vrfDataList := make([]vrfData, 0, len(postByronBlocks))

	for _, bd := range postByronBlocks {
		header := bd.block.Header()
		var vrfResult common.VrfResult
		var vrfKey []byte

		switch h := header.(type) {
		case *shelley.ShelleyBlockHeader:
			vrfResult = h.Body.LeaderVrf
			vrfKey = h.Body.VrfKey
		case *allegra.AllegraBlockHeader:
			vrfResult = h.Body.LeaderVrf
			vrfKey = h.Body.VrfKey
		case *mary.MaryBlockHeader:
			vrfResult = h.Body.LeaderVrf
			vrfKey = h.Body.VrfKey
		case *alonzo.AlonzoBlockHeader:
			vrfResult = h.Body.LeaderVrf
			vrfKey = h.Body.VrfKey
		case *babbage.BabbageBlockHeader:
			vrfResult = h.Body.VrfResult
			vrfKey = h.Body.VrfKey
		case *conway.ConwayBlockHeader:
			vrfResult = h.Body.VrfResult
			vrfKey = h.Body.VrfKey
		default:
			continue
		}

		vrfDataList = append(vrfDataList, vrfData{
			name:   bd.name,
			vrfKey: vrfKey,
			proof:  vrfResult.Proof,
			output: vrfResult.Output,
			slot:   header.SlotNumber(),
		})
	}

	// Create test eta0 (32 bytes)
	eta0 := make([]byte, 32)

	for _, vd := range vrfDataList {
		b.Run("Era_"+vd.name, func(b *testing.B) {
			// Pre-compute the VRF input message
			vrfMsg := vrf.MkInputVrf(int64(vd.slot), eta0)

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = vrf.Verify(vd.vrfKey, vd.proof, vd.output, vrfMsg)
			}
		})
	}
}

// BenchmarkBlockKESVerification benchmarks KES verification isolated from other
// validation.
func BenchmarkBlockKESVerification(b *testing.B) {
	blocks := loadTestBlocks()
	postByronBlocks := getPostByronBlocks(blocks)

	// Pre-extract KES data for each block
	type kesData struct {
		name      string
		bodyCbor  []byte
		signature []byte
		hotVkey   []byte
		kesPeriod uint64
		slot      uint64
	}

	kesDataList := make([]kesData, 0, len(postByronBlocks))

	for _, bd := range postByronBlocks {
		header := bd.block.Header()
		var bodyCbor []byte
		var sig []byte
		var hotVkey []byte
		var kesPeriod uint64
		var err error

		switch h := header.(type) {
		case *shelley.ShelleyBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCertHotVkey
			kesPeriod = uint64(h.Body.OpCertKesPeriod)
		case *allegra.AllegraBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCertHotVkey
			kesPeriod = uint64(h.Body.OpCertKesPeriod)
		case *mary.MaryBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCertHotVkey
			kesPeriod = uint64(h.Body.OpCertKesPeriod)
		case *alonzo.AlonzoBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCertHotVkey
			kesPeriod = uint64(h.Body.OpCertKesPeriod)
		case *babbage.BabbageBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCert.HotVkey
			kesPeriod = uint64(h.Body.OpCert.KesPeriod)
		case *conway.ConwayBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCert.HotVkey
			kesPeriod = uint64(h.Body.OpCert.KesPeriod)
		default:
			continue
		}

		kesDataList = append(kesDataList, kesData{
			name:      bd.name,
			bodyCbor:  bodyCbor,
			signature: sig,
			hotVkey:   hotVkey,
			kesPeriod: kesPeriod,
			slot:      header.SlotNumber(),
		})
	}

	for _, kd := range kesDataList {
		b.Run("Era_"+kd.name, func(b *testing.B) {
			currentKesPeriod := kd.slot / defaultSlotsPerKesPeriod
			if currentKesPeriod < kd.kesPeriod {
				b.Skipf(
					"slot-derived KES period %d < cert KES period %d",
					currentKesPeriod,
					kd.kesPeriod,
				)
				return
			}
			t := currentKesPeriod - kd.kesPeriod

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = kes.VerifySignedKES(
					kd.hotVkey,
					t,
					kd.bodyCbor,
					kd.signature,
				)
			}
		})
	}
}

// BenchmarkBlockBodyHash benchmarks body hash validation isolated from other
// validation.
func BenchmarkBlockBodyHash(b *testing.B) {
	blocks := loadTestBlocks()
	postByronBlocks := getPostByronBlocks(blocks)

	// Pre-extract body hash data for each block
	type bodyHashData struct {
		name         string
		cbor         []byte
		expectedHash common.Blake2b256
		eraName      string
		minLength    int
	}

	bodyHashDataList := make([]bodyHashData, 0, len(postByronBlocks))

	for _, bd := range postByronBlocks {
		era := bd.block.Era()
		minLength := 4
		if era.Id >= alonzo.EraAlonzo.Id {
			minLength = 5
		}

		bodyHashDataList = append(bodyHashDataList, bodyHashData{
			name:         bd.name,
			cbor:         bd.cbor,
			expectedHash: bd.block.BlockBodyHash(),
			eraName:      era.Name,
			minLength:    minLength,
		})
	}

	for _, bhd := range bodyHashDataList {
		b.Run("Era_"+bhd.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				err := common.ValidateBlockBodyHash(
					bhd.cbor,
					bhd.expectedHash,
					bhd.eraName,
					bhd.minLength,
				)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkBlockDecode benchmarks block CBOR decoding by era.
// This isolates parsing performance from validation.
func BenchmarkBlockDecode(b *testing.B) {
	blocks := testdata.GetTestBlocks()

	for _, tb := range blocks {
		b.Run("Era_"+tb.Name, func(b *testing.B) {
			// Use config that skips body hash to isolate decode performance
			cfg := common.VerifyConfig{
				SkipBodyHashValidation:    true,
				SkipTransactionValidation: true,
				SkipStakePoolValidation:   true,
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ledger.NewBlockFromCbor(tb.BlockType, tb.Cbor, cfg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkBlockDecodeWithBodyHash benchmarks block decoding with body hash
// validation.
func BenchmarkBlockDecodeWithBodyHash(b *testing.B) {
	blocks := testdata.GetTestBlocks()

	for _, tb := range blocks {
		// Skip Byron as it doesn't have body hash validation
		if tb.BlockType == ledger.BlockTypeByronMain ||
			tb.BlockType == ledger.BlockTypeByronEbb {
			continue
		}

		b.Run("Era_"+tb.Name, func(b *testing.B) {
			cfg := common.VerifyConfig{
				SkipBodyHashValidation:    false,
				SkipTransactionValidation: true,
				SkipStakePoolValidation:   true,
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ledger.NewBlockFromCbor(tb.BlockType, tb.Cbor, cfg)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMkInputVrf benchmarks VRF input creation for leader election.
// This is the vrf.MkInputVrf function used in block validation.
func BenchmarkMkInputVrf(b *testing.B) {
	blocks := loadTestBlocks()
	postByronBlocks := getPostByronBlocks(blocks)

	eta0 := make([]byte, 32)
	for i := range eta0 {
		eta0[i] = byte(i)
	}

	for _, bd := range postByronBlocks {
		b.Run("Era_"+bd.name, func(b *testing.B) {
			slot := int64(bd.block.Header().SlotNumber())

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = vrf.MkInputVrf(slot, eta0)
			}
		})
	}
}

// BenchmarkVerifyKesComponents benchmarks the full KES components verification.
// This matches what VerifyBlock calls internally.
func BenchmarkVerifyKesComponents(b *testing.B) {
	blocks := loadTestBlocks()
	postByronBlocks := getPostByronBlocks(blocks)

	type kesData struct {
		name      string
		bodyCbor  []byte
		signature []byte
		hotVkey   []byte
		kesPeriod uint64
		slot      uint64
	}

	kesDataList := make([]kesData, 0, len(postByronBlocks))

	for _, bd := range postByronBlocks {
		header := bd.block.Header()
		var bodyCbor []byte
		var sig []byte
		var hotVkey []byte
		var kesPeriod uint64
		var err error

		switch h := header.(type) {
		case *shelley.ShelleyBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCertHotVkey
			kesPeriod = uint64(h.Body.OpCertKesPeriod)
		case *allegra.AllegraBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCertHotVkey
			kesPeriod = uint64(h.Body.OpCertKesPeriod)
		case *mary.MaryBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCertHotVkey
			kesPeriod = uint64(h.Body.OpCertKesPeriod)
		case *alonzo.AlonzoBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCertHotVkey
			kesPeriod = uint64(h.Body.OpCertKesPeriod)
		case *babbage.BabbageBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCert.HotVkey
			kesPeriod = uint64(h.Body.OpCert.KesPeriod)
		case *conway.ConwayBlockHeader:
			bodyCbor, err = cbor.Encode(h.Body)
			if err != nil {
				continue
			}
			sig = h.Signature
			hotVkey = h.Body.OpCert.HotVkey
			kesPeriod = uint64(h.Body.OpCert.KesPeriod)
		default:
			continue
		}

		kesDataList = append(kesDataList, kesData{
			name:      bd.name,
			bodyCbor:  bodyCbor,
			signature: sig,
			hotVkey:   hotVkey,
			kesPeriod: kesPeriod,
			slot:      header.SlotNumber(),
		})
	}

	for _, kd := range kesDataList {
		b.Run("Era_"+kd.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = ledger.VerifyKesComponents(
					kd.bodyCbor,
					kd.signature,
					kd.hotVkey,
					kd.kesPeriod,
					kd.slot,
					defaultSlotsPerKesPeriod,
				)
			}
		})
	}
}

// BenchmarkHeaderCborEncode benchmarks CBOR encoding of header bodies.
// This is required for KES verification.
func BenchmarkHeaderCborEncode(b *testing.B) {
	blocks := loadTestBlocks()
	postByronBlocks := getPostByronBlocks(blocks)

	for _, bd := range postByronBlocks {
		b.Run("Era_"+bd.name, func(b *testing.B) {
			header := bd.block.Header()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := ledger.GetHeaderBodyCbor(header)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
