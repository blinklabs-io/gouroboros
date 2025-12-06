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
	config common.VerifyConfig,
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
	if block.Era() != byron.EraByron && !config.SkipBodyHashValidation {
		rawCbor := block.Cbor()
		if len(rawCbor) == 0 {
			return false, "", 0, 0, errors.New(
				"VerifyBlock: block CBOR is required for body hash verification",
			)
		}
		era := block.Era()
		eraName := era.Name
		minLength := 4
		if era.Id >= alonzo.EraAlonzo.Id {
			minLength = 5
		}
		if err := common.ValidateBlockBodyHash(rawCbor, expectedBodyHash, eraName, minLength); err != nil {
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: %w",
				err,
			)
		}
	}

	// Verify transactions (can be skipped via config)
	// Requires LedgerState and ProtocolParameters in config if enabled.
	if block.Era() != byron.EraByron && !config.SkipTransactionValidation {
		var validationRules []common.UtxoValidationRuleFunc
		switch block.Era().Id {
		case shelley.EraShelley.Id:
			validationRules = shelley.UtxoValidationRules
		case allegra.EraAllegra.Id:
			validationRules = allegra.UtxoValidationRules
		case mary.EraMary.Id:
			validationRules = mary.UtxoValidationRules
		case alonzo.EraAlonzo.Id:
			validationRules = alonzo.UtxoValidationRules
		case babbage.EraBabbage.Id:
			validationRules = babbage.UtxoValidationRules
		case conway.EraConway.Id:
			validationRules = conway.UtxoValidationRules
		default:
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: unsupported era for transaction validation %s",
				block.Era().Name,
			)
		}
		if config.LedgerState == nil || config.ProtocolParameters == nil {
			return false, "", 0, 0, errors.New(
				"VerifyBlock: missing required config field: LedgerState and ProtocolParameters must be set",
			)
		}
		for _, tx := range block.Transactions() {
			if err := common.VerifyTransaction(tx, slot, config.LedgerState, config.ProtocolParameters, validationRules); err != nil {
				return false, "", 0, 0, fmt.Errorf(
					"VerifyBlock: transaction validation failed: %w",
					err,
				)
			}
		}
	}

	// Verify stake pool registration (can be skipped via config)
	// Requires LedgerState in config if enabled.
	if block.Era() != byron.EraByron && !config.SkipStakePoolValidation {
		if config.LedgerState == nil {
			return false, "", 0, 0, errors.New(
				"VerifyBlock: missing required config field: LedgerState must be set for stake pool validation",
			)
		}

		// Extract pool key hash from issuer vkey
		var issuerVkey []byte
		switch h := block.Header().(type) {
		case *shelley.ShelleyBlockHeader:
			issuerVkey = h.Body.IssuerVkey[:]
		case *allegra.AllegraBlockHeader:
			issuerVkey = h.Body.IssuerVkey[:]
		case *mary.MaryBlockHeader:
			issuerVkey = h.Body.IssuerVkey[:]
		case *alonzo.AlonzoBlockHeader:
			issuerVkey = h.Body.IssuerVkey[:]
		case *babbage.BabbageBlockHeader:
			issuerVkey = h.Body.IssuerVkey[:]
		case *conway.ConwayBlockHeader:
			issuerVkey = h.Body.IssuerVkey[:]
		default:
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: unsupported block type for stake pool validation %T",
				block.Header(),
			)
		}

		poolKeyHash := common.Blake2b224Hash(issuerVkey)

		// Check if pool is registered
		poolCert, _, err := config.LedgerState.PoolCurrentState(poolKeyHash)
		if err != nil {
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: failed to query pool state: %w",
				err,
			)
		}
		if poolCert == nil {
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: pool %s is not registered",
				poolKeyHash.String(),
			)
		}

		// Check if VRF key matches registered pool's VRF key
		var blockVrfKey []byte
		switch h := block.Header().(type) {
		case *shelley.ShelleyBlockHeader:
			blockVrfKey = h.Body.VrfKey
		case *allegra.AllegraBlockHeader:
			blockVrfKey = h.Body.VrfKey
		case *mary.MaryBlockHeader:
			blockVrfKey = h.Body.VrfKey
		case *alonzo.AlonzoBlockHeader:
			blockVrfKey = h.Body.VrfKey
		case *babbage.BabbageBlockHeader:
			blockVrfKey = h.Body.VrfKey
		case *conway.ConwayBlockHeader:
			blockVrfKey = h.Body.VrfKey
		}

		registeredVrfKeyHash := poolCert.VrfKeyHash
		expectedVrfKeyHash := common.Blake2b256Hash(blockVrfKey)
		if registeredVrfKeyHash != expectedVrfKeyHash {
			return false, "", 0, 0, fmt.Errorf(
				"VerifyBlock: VRF key mismatch for pool %s: expected %s, got %s",
				poolKeyHash.String(),
				registeredVrfKeyHash.String(),
				expectedVrfKeyHash.String(),
			)
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
