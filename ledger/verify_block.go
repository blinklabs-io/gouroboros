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

func extractHeaderFields(
	header BlockHeader,
) (issuerVkey, vrfKey []byte, err error) {
	switch h := header.(type) {
	case *shelley.ShelleyBlockHeader:
		return h.Body.IssuerVkey[:], h.Body.VrfKey, nil
	case *allegra.AllegraBlockHeader:
		return h.Body.IssuerVkey[:], h.Body.VrfKey, nil
	case *mary.MaryBlockHeader:
		return h.Body.IssuerVkey[:], h.Body.VrfKey, nil
	case *alonzo.AlonzoBlockHeader:
		return h.Body.IssuerVkey[:], h.Body.VrfKey, nil
	case *babbage.BabbageBlockHeader:
		return h.Body.IssuerVkey[:], h.Body.VrfKey, nil
	case *conway.ConwayBlockHeader:
		return h.Body.IssuerVkey[:], h.Body.VrfKey, nil
	default:
		return nil, nil, common.NewValidationError(
			common.ValidationErrorTypeProtocol,
			"unsupported block type for stake pool validation",
			map[string]any{
				"block_type":   fmt.Sprintf("%T", header),
				"slot":         header.SlotNumber(),
				"block_number": header.BlockNumber(),
			},
			nil,
		)
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
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeConfiguration,
			"invalid eta0 parameter",
			map[string]any{
				"eta0_hex": eta0Hex,
			},
			err,
		)
	}

	// Get header
	header := block.Header()

	// Extract slot and block number from header
	slot := header.SlotNumber()
	blockNo := header.BlockNumber()
	era := header.Era()

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
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeProtocol,
			"unsupported block type for VRF verification",
			map[string]any{
				"block_type":   fmt.Sprintf("%T", block.Header()),
				"slot":         slot,
				"block_number": blockNo,
				"era":          era,
			},
			nil,
		)
	}

	// Verify VRF
	if slot > math.MaxInt64 {
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeProtocol,
			"slot value exceeds maximum int64 value",
			map[string]any{
				"slot":         slot,
				"block_number": blockNo,
				"era":          era,
				"max_int64":    math.MaxInt64,
			},
			nil,
		)
	}
	vrfMsg := MkInputVrf(int64(slot), eta0)
	vrfValid, err = VerifyVrf(vrfKey, vrfResult.Proof, vrfResult.Output, vrfMsg)
	if err != nil {
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeVRF,
			"VRF verification failed",
			map[string]any{
				"slot":           slot,
				"block_number":   blockNo,
				"era":            era,
				"vrf_key_len":    len(vrfKey),
				"vrf_proof_len":  len(vrfResult.Proof),
				"vrf_output_len": len(vrfResult.Output),
			},
			err,
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
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeProtocol,
				"failed to encode Shelley header body for KES verification",
				map[string]any{
					"slot":         slot,
					"block_number": blockNo,
					"era":          era,
					"block_type":   "Shelley",
				},
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCertHotVkey
		kesPeriod = uint64(h.Body.OpCertKesPeriod)
	case *allegra.AllegraBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeProtocol,
				"failed to encode Allegra header body for KES verification",
				map[string]any{
					"slot":         slot,
					"block_number": blockNo,
					"era":          era,
					"block_type":   "Allegra",
				},
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCertHotVkey
		kesPeriod = uint64(h.Body.OpCertKesPeriod)
	case *mary.MaryBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeProtocol,
				"failed to encode Mary header body for KES verification",
				map[string]any{
					"slot":         slot,
					"block_number": blockNo,
					"era":          era,
					"block_type":   "Mary",
				},
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCertHotVkey
		kesPeriod = uint64(h.Body.OpCertKesPeriod)
	case *alonzo.AlonzoBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeProtocol,
				"failed to encode Alonzo header body for KES verification",
				map[string]any{
					"slot":         slot,
					"block_number": blockNo,
					"era":          era,
					"block_type":   "Alonzo",
				},
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCertHotVkey
		kesPeriod = uint64(h.Body.OpCertKesPeriod)
	case *babbage.BabbageBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeProtocol,
				"failed to encode Babbage header body for KES verification",
				map[string]any{
					"slot":         slot,
					"block_number": blockNo,
					"era":          era,
					"block_type":   "Babbage",
				},
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCert.HotVkey
		kesPeriod = uint64(h.Body.OpCert.KesPeriod)
	case *conway.ConwayBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeProtocol,
				"failed to encode Conway header body for KES verification",
				map[string]any{
					"slot":         slot,
					"block_number": blockNo,
					"era":          era,
					"block_type":   "Conway",
				},
				err,
			)
		}
		signature = h.Signature
		hotVkey = h.Body.OpCert.HotVkey
		kesPeriod = uint64(h.Body.OpCert.KesPeriod)
	default:
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeProtocol,
			"unsupported block type for KES verification",
			map[string]any{
				"block_type":   fmt.Sprintf("%T", block.Header()),
				"slot":         slot,
				"block_number": blockNo,
				"era":          era,
			},
			nil,
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
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeKES,
			"KES verification failed",
			map[string]any{
				"slot":                 slot,
				"block_number":         blockNo,
				"era":                  era,
				"kes_period":           kesPeriod,
				"slots_per_kes_period": slotsPerKesPeriod,
				"hot_vkey_len":         len(hotVkey),
				"signature_len":        len(signature),
				"body_cbor_len":        len(bodyCbor),
			},
			err,
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
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeConfiguration,
				"block CBOR is required for body hash verification",
				map[string]any{
					"slot":                      slot,
					"block_number":              blockNo,
					"era":                       era,
					"skip_body_hash_validation": config.SkipBodyHashValidation,
				},
				nil,
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
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeConfiguration,
				"missing required config fields for transaction validation",
				map[string]any{
					"has_ledger_state":     config.LedgerState != nil,
					"has_protocol_params":  config.ProtocolParameters != nil,
					"skip_tx_validation":   config.SkipTransactionValidation,
					"skip_pool_validation": config.SkipStakePoolValidation,
				},
				nil,
			)
		}
		for _, tx := range block.Transactions() {
			if err := common.VerifyTransaction(tx, slot, config.LedgerState, config.ProtocolParameters, validationRules); err != nil {
				return false, "", 0, 0, common.NewValidationError(
					common.ValidationErrorTypeTransaction,
					"block transaction validation failed",
					map[string]any{
						"block_slot":   slot,
						"block_number": blockNo,
						"era":          era,
					},
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

		issuerVkey, blockVrfKey, err := extractHeaderFields(block.Header())
		if err != nil {
			return false, "", 0, 0, err
		}

		poolKeyHash := common.Blake2b224Hash(issuerVkey)

		// Check if pool is registered
		poolCert, _, err := config.LedgerState.PoolCurrentState(poolKeyHash)
		if err != nil {
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeStakePool,
				"failed to query pool state",
				map[string]any{
					"pool_key_hash": poolKeyHash.String(),
					"block_slot":    slot,
					"block_number":  blockNo,
					"era":           era,
				},
				err,
			)
		}
		if poolCert == nil {
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeStakePool,
				"pool is not registered",
				map[string]any{
					"pool_key_hash": poolKeyHash.String(),
					"block_slot":    slot,
					"block_number":  blockNo,
					"era":           era,
				},
				nil,
			)
		}

		// Check if VRF key matches registered pool's VRF key
		registeredVrfKeyHash := poolCert.VrfKeyHash
		expectedVrfKeyHash := common.Blake2b256Hash(blockVrfKey)
		if registeredVrfKeyHash != expectedVrfKeyHash {
			return false, "", 0, 0, common.NewValidationError(
				common.ValidationErrorTypeVRF,
				"VRF key mismatch for pool",
				map[string]any{
					"pool_key_hash":       poolKeyHash.String(),
					"registered_vrf_hash": registeredVrfKeyHash.String(),
					"block_vrf_hash":      expectedVrfKeyHash.String(),
					"block_slot":          slot,
					"block_number":        blockNo,
					"era":                 era,
				},
				nil,
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
