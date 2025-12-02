// Copyright 2024 Cardano Foundation
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

// This file is taken almost verbatim (including comments) from
// https://github.com/cardano-foundation/cardano-ibc-incubator

package ledger

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"golang.org/x/crypto/blake2b"
)

const (
	HeaderBodyLengthShelleyLike = 15
	HeaderBodyLengthBabbageLike = 10
	ProtoMajorShelley           = 2
	ProtoMajorAllegra           = 3
	ProtoMajorMary              = 4
	ProtoMajorAlonzo            = 5
	ProtoMajorBabbage           = 7
	ProtoMajorConway            = 9
)

// DetermineBlockType determines the block type from the header CBOR
func DetermineBlockType(headerCbor []byte) (uint, error) {
	var header any
	if _, err := cbor.Decode(headerCbor, &header); err != nil {
		return 0, fmt.Errorf("decode header error: %w", err)
	}
	h, ok := header.([]any)
	if !ok || len(h) != 2 {
		return 0, errors.New("invalid header structure")
	}
	body, ok := h[0].([]any)
	if !ok {
		return 0, errors.New("invalid header body")
	}
	lenBody := len(body)
	switch lenBody {
	case HeaderBodyLengthShelleyLike:
		// Shelley era
		protoMajor, ok := body[13].(uint64)
		if !ok {
			return 0, errors.New("invalid proto major")
		}
		switch protoMajor {
		case ProtoMajorShelley:
			return BlockTypeShelley, nil
		case ProtoMajorAllegra:
			return BlockTypeAllegra, nil
		case ProtoMajorMary:
			return BlockTypeMary, nil
		case ProtoMajorAlonzo:
			return BlockTypeAlonzo, nil
		default:
			return 0, fmt.Errorf(
				"unknown proto major %d for Shelley-like",
				protoMajor,
			)
		}
	case HeaderBodyLengthBabbageLike:
		// Babbage era
		if len(body) <= 9 {
			return 0, errors.New(
				"header body too short for proto version field",
			)
		}
		protoVersion, ok := body[9].([]any)
		if !ok || len(protoVersion) < 1 {
			return 0, errors.New("invalid proto version")
		}
		protoMajor, ok := protoVersion[0].(uint64)
		if !ok {
			return 0, errors.New("invalid proto major")
		}
		switch protoMajor {
		case ProtoMajorBabbage:
			return BlockTypeBabbage, nil
		case ProtoMajorConway:
			return BlockTypeConway, nil
		case ProtoMajorAlonzo:
			return BlockTypeAlonzo, nil
		case ProtoMajorMary:
			return BlockTypeMary, nil
		default:
			return 0, fmt.Errorf(
				"unknown proto major %d for 10-field header",
				protoMajor,
			)
		}
	default:
		return 0, fmt.Errorf("unknown header body length %d", lenBody)
	}
}

func VerifyBlock(
	block Block,
	eta0Hex string,
	slotsPerKesPeriod uint64,
) (bool, string, uint64, uint64, error) {
	isValid := false
	vrfHex := ""

	// Decode eta0
	eta0, err := hex.DecodeString(eta0Hex)
	if err != nil {
		return false, "", 0, 0, fmt.Errorf(
			"VerifyBlock: eta0 decode error, %v",
			err.Error(),
		)
	}

	// Get header
	header := block.Header()

	// Extract slot and block number from header
	slot := header.SlotNumber()
	blockNo := header.BlockNumber()

	// VRF verification
	var vrfValid bool
	var kesValid bool
	var vrfResult common.VrfResult
	var vrfKey []byte
	switch h := block.Header().(type) {
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
		return false, "", 0, 0, fmt.Errorf(
			"VerifyBlock: unsupported block type for VRF %T",
			block.Header(),
		)
	}

	// Verify VRF
	if slot > math.MaxInt64 {
		return false, "", 0, 0, fmt.Errorf(
			"VerifyBlock: slot value %d exceeds maximum int64 value",
			slot,
		)
	}
	vrfMsg := MkInputVrf(int64(slot), eta0)
	vrfValid, err = VerifyVrf(vrfKey, vrfResult.Proof, vrfResult.Output, vrfMsg)
	if err != nil {
		return false, "", 0, 0, fmt.Errorf(
			"VerifyBlock: VRF verification error, %v",
			err.Error(),
		)
	}

	vrfHex = hex.EncodeToString(vrfResult.Output)

	// KES verification
	var bodyCbor []byte
	var signature []byte
	var hotVkey []byte
	var kesPeriod uint64
	switch h := block.Header().(type) {
	case *shelley.ShelleyBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: failed to encode Shelley header body for KES, %w",
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCertHotVkey
		kesPeriod = uint64(h.Body.OpCertKesPeriod)
	case *allegra.AllegraBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: failed to encode Allegra header body for KES, %w",
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCertHotVkey
		kesPeriod = uint64(h.Body.OpCertKesPeriod)
	case *mary.MaryBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: failed to encode Mary header body for KES, %w",
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCertHotVkey
		kesPeriod = uint64(h.Body.OpCertKesPeriod)
	case *alonzo.AlonzoBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: failed to encode Alonzo header body for KES, %w",
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCertHotVkey
		kesPeriod = uint64(h.Body.OpCertKesPeriod)
	case *babbage.BabbageBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: failed to encode Babbage header body for KES, %w",
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCert.HotVkey
		kesPeriod = uint64(h.Body.OpCert.KesPeriod)
	case *conway.ConwayBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: failed to encode Conway header body for KES, %w",
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCert.HotVkey
		kesPeriod = uint64(h.Body.OpCert.KesPeriod)
	default:
		return false, "", 0, 0, fmt.Errorf(
			"VerifyBlock: unsupported block type for KES %T",
			block.Header(),
		)
	}

	kesValid, err = VerifyKesComponents(
		bodyCbor,
		signature,
		hotVkey,
		kesPeriod,
		slot,
		slotsPerKesPeriod,
	)
	if err != nil {
		return false, "", 0, 0, fmt.Errorf(
			"VerifyBlock: KES verification error, %v",
			err.Error(),
		)
	}

	// Verify block body hash (can be skipped via config)
	// Intended usage: production should keep this enabled for security.
	// Tests or environments lacking full block CBOR may set
	// VerifyConfig{SkipBodyHashValidation:true} to bypass this check.
	expectedBodyHash := block.BlockBodyHash()
	isBodyValid := true
	if block.Era() != byron.EraByron && !GetVerifyConfig().SkipBodyHashValidation {
		rawCbor := block.Cbor()
		if len(rawCbor) == 0 {
			return false, "", 0, 0, errors.New(
				"VerifyBlock: block CBOR is required for body hash verification",
			)
		} else {
			var raw []cbor.RawMessage
			if _, err := cbor.Decode(rawCbor, &raw); err != nil {
				return false, "", 0, 0, fmt.Errorf(
					"VerifyBlock: failed to decode block CBOR for body hash, %w",
					err,
				)
			}
			if len(raw) < 4 {
				return false, "", 0, 0, errors.New(
					"VerifyBlock: invalid block CBOR structure for body hash",
				)
			}
			// Compute body hash as per Cardano spec: blake2b_256(hash_tx || hash_wit || hash_aux || hash_invalid)
			emptyInvalidCbor, _ := cbor.Encode(cbor.IndefLengthList([]any{}))
			hashInvalidDefault := blake2b.Sum256(emptyInvalidCbor)
			var bodyHashes []byte
			hashTx := blake2b.Sum256(raw[1])
			bodyHashes = append(bodyHashes, hashTx[:]...)
			hashWit := blake2b.Sum256(raw[2])
			bodyHashes = append(bodyHashes, hashWit[:]...)
			hashAux := blake2b.Sum256(raw[3])
			bodyHashes = append(bodyHashes, hashAux[:]...)
			switch block.Header().(type) {
			case *shelley.ShelleyBlockHeader, *allegra.AllegraBlockHeader, *mary.MaryBlockHeader:
				bodyHashes = append(bodyHashes, hashInvalidDefault[:]...)
			case *alonzo.AlonzoBlockHeader, *babbage.BabbageBlockHeader, *conway.ConwayBlockHeader:
				hashInvalid := blake2b.Sum256(raw[4])
				bodyHashes = append(bodyHashes, hashInvalid[:]...)
			default:
				return false, "", 0, 0, fmt.Errorf(
					"VerifyBlock: unsupported block type for body hash %T",
					block.Header(),
				)
			}
			actualBodyHash := blake2b.Sum256(bodyHashes)
			if !bytes.Equal(actualBodyHash[:], expectedBodyHash.Bytes()) {
				return false, "", 0, 0, errors.New(
					"VerifyBlock: block body hash mismatch",
				)
			}
		}
	}

	isValid = isBodyValid && vrfValid && kesValid
	slotNo := slot
	return isValid, vrfHex, blockNo, slotNo, nil
}

type BlockHexCbor struct {
	cbor.StructAsArray
	Flag          int
	HeaderCbor    string
	Eta0          string
	Spk           int
	BlockBodyCbor string
}
