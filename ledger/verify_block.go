// Copyright 2024 Cardano Foundation
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

// This file is taken almost verbatim (including comments) from
// https://github.com/cardano-foundation/cardano-ibc-incubator

package ledger

import (
	"crypto/subtle"
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
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/gouroboros/vrf"
)

// The era packages own the protocol-major ranges their blocks may carry, and
// DetermineBlockType classifies against those. These single values are kept
// for callers that already reference them; they name the first major of each
// era and are not the authority on its extent.
const (
	HeaderBodyLengthShelleyLike = 15
	HeaderBodyLengthBabbageLike = 10
	ProtoMajorShelley           = 2
	ProtoMajorAllegra           = 3
	ProtoMajorMary              = 4
	ProtoMajorAlonzo            = 5
	ProtoMajorBabbage           = 7
	ProtoMajorConway            = 9
	ProtoMajorDijkstra          = 12
)

// inProtocolRange reports whether a header's protocol major falls within an
// era's declared range, inclusive at both ends.
func inProtocolRange(protoMajor, min, max uint64) bool {
	return protoMajor >= min && protoMajor <= max
}

func eraAtLeast(era common.Era, min common.Era) bool {
	currentOrder, ok := eraOrder(era)
	if !ok {
		return false
	}
	minOrder, ok := eraOrder(min)
	return ok && currentOrder >= minOrder
}

// validateDijkstraBlockBodyHash validates a Dijkstra block's body hash. The
// block is [header, block_body] and the header's block_body_hash is blake2b256
// over the block_body CBOR (confirmed against live prototype-2026w27 blocks),
// unlike the pre-Dijkstra segwit hash of concatenated per-segment hashes.
func validateDijkstraBlockBodyHash(
	rawCbor []byte,
	expectedBodyHash common.Blake2b256,
) error {
	var raw []cbor.RawMessage
	if _, err := cbor.Decode(rawCbor, &raw); err != nil {
		return common.NewValidationError(
			common.ValidationErrorTypeBodyHash,
			"failed to decode Dijkstra block CBOR for body hash validation",
			map[string]any{"era": dijkstra.EraNameDijkstra},
			err,
		)
	}
	if len(raw) != 2 {
		return common.NewValidationError(
			common.ValidationErrorTypeBodyHash,
			"invalid Dijkstra block CBOR structure for body hash validation",
			map[string]any{
				"era":           dijkstra.EraNameDijkstra,
				"expected":      2,
				"actual_length": len(raw),
			},
			nil,
		)
	}
	actualBodyHash := common.Blake2b256Hash(raw[1])
	if subtle.ConstantTimeCompare(actualBodyHash[:], expectedBodyHash[:]) != 1 {
		return common.NewValidationError(
			common.ValidationErrorTypeBodyHash,
			dijkstra.EraNameDijkstra+" block body hash mismatch during parsing",
			map[string]any{
				"era":           dijkstra.EraNameDijkstra,
				"expected_hash": expectedBodyHash.String(),
				"actual_hash":   actualBodyHash.String(),
			},
			nil,
		)
	}
	return nil
}

func eraOrder(era common.Era) (int, bool) {
	switch era {
	case byron.EraByron:
		return 0, true
	case shelley.EraShelley:
		return 1, true
	case allegra.EraAllegra:
		return 2, true
	case mary.EraMary:
		return 3, true
	case alonzo.EraAlonzo:
		return 4, true
	case babbage.EraBabbage:
		return 5, true
	case conway.EraConway:
		return 6, true
	case dijkstra.EraDijkstra:
		return 7, true
	default:
		return 0, false
	}
}

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
		switch {
		case inProtocolRange(
			protoMajor,
			shelley.MinProtocolVersionShelley,
			shelley.MaxProtocolVersionShelley,
		):
			return BlockTypeShelley, nil
		case inProtocolRange(
			protoMajor,
			allegra.MinProtocolVersionAllegra,
			allegra.MaxProtocolVersionAllegra,
		):
			return BlockTypeAllegra, nil
		case inProtocolRange(
			protoMajor,
			mary.MinProtocolVersionMary,
			mary.MaxProtocolVersionMary,
		):
			return BlockTypeMary, nil
		case inProtocolRange(
			protoMajor,
			alonzo.MinProtocolVersionAlonzo,
			alonzo.MaxProtocolVersionAlonzo,
		):
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
		switch {
		case inProtocolRange(
			protoMajor,
			babbage.MinProtocolVersionBabbage,
			babbage.MaxProtocolVersionBabbage,
		):
			return BlockTypeBabbage, nil
		case inProtocolRange(
			protoMajor,
			conway.MinProtocolVersionConway,
			conway.MaxProtocolVersionConway,
		):
			return BlockTypeConway, nil
		case inProtocolRange(
			protoMajor,
			dijkstra.MinProtocolVersionDijkstra,
			dijkstra.MaxProtocolVersionDijkstra,
		):
			return BlockTypeDijkstra, nil
		case inProtocolRange(
			protoMajor,
			alonzo.MinProtocolVersionAlonzo,
			alonzo.MaxProtocolVersionAlonzo,
		):
			return BlockTypeAlonzo, nil
		case inProtocolRange(
			protoMajor,
			mary.MinProtocolVersionMary,
			mary.MaxProtocolVersionMary,
		):
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

// extractOriginalBodyCbor returns the original CBOR bytes for the block
// header body. Each body type stores its raw CBOR at decode time via
// DecodeStoreCbor, so we just retrieve it here. This is critical for KES
// signature verification because the signature is computed over the original
// CBOR encoding, not a re-encoded version.
func extractOriginalBodyCbor(header BlockHeader) ([]byte, error) {
	switch h := header.(type) {
	case *shelley.ShelleyBlockHeader:
		return h.Body.Cbor(), nil
	case *allegra.AllegraBlockHeader:
		return h.Body.Cbor(), nil
	case *mary.MaryBlockHeader:
		return h.Body.Cbor(), nil
	case *alonzo.AlonzoBlockHeader:
		return h.Body.Cbor(), nil
	case *babbage.BabbageBlockHeader:
		return h.Body.Cbor(), nil
	case *conway.ConwayBlockHeader:
		return h.Body.Cbor(), nil
	case *dijkstra.DijkstraBlockHeader:
		return h.Body.Cbor(), nil
	default:
		return nil, fmt.Errorf(
			"unsupported header type for body CBOR extraction: %T",
			header,
		)
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
	case *dijkstra.DijkstraBlockHeader:
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

// blockLevelLimits extracts the block-wide maximum body size, maximum
// header size, and (for Alonzo+) maximum execution-unit budget from the
// era-specific protocol parameters. There is no shared getter across eras
// for these fields (each era owns its own concrete ProtocolParameters
// struct), so this mirrors the same type-assertion pattern the era's own
// UtxoValidateMaxTxSizeUtxo/UtxoValidateExUnitsTooBigUtxo per-transaction
// rules use. allegra.AllegraProtocolParameters is a type alias for
// shelley.ShelleyProtocolParameters, so it is covered by the Shelley case.
//
// ProtocolParameters is a public single-method interface, so callers may
// pass an implementation that isn't one of the era's concrete pparams
// structs (e.g. a mock or custom type). This fails closed: an unrecognized
// type returns an error rather than silently disabling block-wide limit
// enforcement, matching the per-era per-transaction rules' own
// "pparams are not the expected type" convention. Callers that legitimately
// don't want block-limit enforcement (e.g. because they pass a mock
// ProtocolParameters unrelated to block limits) must opt out explicitly via
// VerifyConfig.SkipBlockLimitsValidation rather than relying on an
// unrecognized type to silently disable the check.
func blockLevelLimits(
	pp common.ProtocolParameters,
) (
	maxBodySize, maxHeaderSize uint64,
	maxExUnits common.ExUnits,
	hasMaxExUnits bool,
	err error,
) {
	switch p := pp.(type) {
	case *shelley.ShelleyProtocolParameters:
		return uint64(p.MaxBlockBodySize),
			uint64(p.MaxBlockHeaderSize),
			common.ExUnits{},
			false,
			nil
	case *mary.MaryProtocolParameters:
		return uint64(p.MaxBlockBodySize),
			uint64(p.MaxBlockHeaderSize),
			common.ExUnits{},
			false,
			nil
	case *alonzo.AlonzoProtocolParameters:
		return uint64(p.MaxBlockBodySize),
			uint64(p.MaxBlockHeaderSize),
			p.MaxBlockExUnits,
			true,
			nil
	case *babbage.BabbageProtocolParameters:
		return uint64(p.MaxBlockBodySize),
			uint64(p.MaxBlockHeaderSize),
			p.MaxBlockExUnits,
			true,
			nil
	case *conway.ConwayProtocolParameters:
		return uint64(p.MaxBlockBodySize),
			uint64(p.MaxBlockHeaderSize),
			p.MaxBlockExUnits,
			true,
			nil
	case *dijkstra.DijkstraProtocolParameters:
		return uint64(p.MaxBlockBodySize),
			uint64(p.MaxBlockHeaderSize),
			p.MaxBlockExUnits,
			true,
			nil
	default:
		return 0, 0, common.ExUnits{}, false, fmt.Errorf(
			"unsupported protocol parameters type %T for block-limit validation",
			pp,
		)
	}
}

// sumBlockExUnits sums the ExUnits (memory, steps) across every redeemer in
// every transaction in the block. This reuses the same
// TransactionWitnessRedeemers mechanism that each era's per-transaction
// UtxoValidateExUnitsTooBigUtxo rule sums over, so the block-wide total is
// computed from the same source of truth as the per-transaction check.
func sumBlockExUnits(txs []common.Transaction) (common.ExUnits, error) {
	var totalMemory, totalSteps int64
	for _, tx := range txs {
		witnesses := tx.Witnesses()
		if witnesses == nil {
			continue
		}
		redeemers := witnesses.Redeemers()
		if redeemers == nil {
			continue
		}
		for _, value := range redeemers.Iter() {
			// Execution units are non-negative by protocol definition even
			// though ExUnits stores them as signed int64. Reject a negative
			// Memory or Steps before it is added to the running total: a
			// malformed redeemer with a negative value would otherwise
			// reduce the block-wide sum and could mask a budget that
			// actually exceeds ppMaxBlockExUnits.
			if value.ExUnits.Memory < 0 {
				return common.ExUnits{}, fmt.Errorf(
					"negative execution-unit memory in redeemer: %d",
					value.ExUnits.Memory,
				)
			}
			if value.ExUnits.Steps < 0 {
				return common.ExUnits{}, fmt.Errorf(
					"negative execution-unit steps in redeemer: %d",
					value.ExUnits.Steps,
				)
			}
			var ok bool
			totalMemory, ok = common.AddInt64Checked(
				totalMemory,
				value.ExUnits.Memory,
			)
			if !ok {
				return common.ExUnits{}, errors.New(
					"block total execution-unit memory overflow",
				)
			}
			totalSteps, ok = common.AddInt64Checked(
				totalSteps,
				value.ExUnits.Steps,
			)
			if !ok {
				return common.ExUnits{}, errors.New(
					"block total execution-unit steps overflow",
				)
			}
		}
	}
	return common.ExUnits{Memory: totalMemory, Steps: totalSteps}, nil
}

// VerifyBlock performs block-local structural, cryptographic, and ledger
// validation. It checks data available from the block and supplied verification
// config, including body hash, VRF proof bytes, KES signature, transactions,
// and optional stake pool registration.
//
// VerifyBlock is not full chain-context consensus validation. It does not
// receive the previous header, active stake distribution, active slot
// coefficient, max KES evolutions, or operational certificate sequence state,
// so callers must combine it with chain-state validation before using a block
// as a production consensus decision.
func VerifyBlock(
	block Block,
	eta0Hex string,
	slotsPerKesPeriod uint64,
	config common.VerifyConfig,
) (bool, string, uint64, uint64, error) {
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
	var isTPraos bool
	switch h := block.Header().(type) {
	case *shelley.ShelleyBlockHeader:
		vrfResult = h.Body.LeaderVrf
		vrfKey = h.Body.VrfKey
		isTPraos = true
	case *allegra.AllegraBlockHeader:
		vrfResult = h.Body.LeaderVrf
		vrfKey = h.Body.VrfKey
		isTPraos = true
	case *mary.MaryBlockHeader:
		vrfResult = h.Body.LeaderVrf
		vrfKey = h.Body.VrfKey
		isTPraos = true
	case *alonzo.AlonzoBlockHeader:
		vrfResult = h.Body.LeaderVrf
		vrfKey = h.Body.VrfKey
		isTPraos = true
	case *babbage.BabbageBlockHeader:
		vrfResult = h.Body.VrfResult
		vrfKey = h.Body.VrfKey
	case *conway.ConwayBlockHeader:
		vrfResult = h.Body.VrfResult
		vrfKey = h.Body.VrfKey
	case *dijkstra.DijkstraBlockHeader:
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
				"max_int64":    int64(math.MaxInt64), // int64 for 32-bit build
			},
			nil,
		)
	}
	// TPraos (Shelley-Alonzo) and CPraos (Babbage+) use different VRF
	// input constructions. TPraos applies an additional XOR with a seed
	// constant derived from mkNonceFromNumber. CPraos uses the raw
	// blake2b-256(slot || nonce) without any XOR step.
	//
	// For verifying the LeaderVrf (bheaderL), use seedL = mkNonceFromNumber(1).
	// For verifying the NonceVrf (bheaderEta), use seedEta = mkNonceFromNumber(0).
	// We verify the LeaderVrf here (the one that proves leader election).
	//
	// Ref: Cardano.Protocol.TPraos.Rules.Overlay.vrfChecks (TPraos)
	// Ref: Ouroboros.Consensus.Protocol.Praos.VRF.mkInputVRF (CPraos)
	if len(eta0) != 32 {
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeConfiguration,
			"eta0 must be exactly 32 bytes",
			map[string]any{
				"eta0_hex":     eta0Hex,
				"actual_len":   len(eta0),
				"expected_len": 32,
			},
			nil,
		)
	}
	var vrfMsg []byte
	if isTPraos {
		vrfMsg, err = vrf.MkSeedTPraos(int64(slot), eta0, vrf.SeedL())
	} else {
		vrfMsg, err = vrf.MkInputVrf(int64(slot), eta0)
	}
	if err != nil {
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeConfiguration,
			"invalid VRF input parameters",
			map[string]any{
				"slot":         slot,
				"block_number": blockNo,
				"era":          era,
			},
			err,
		)
	}
	vrfValid, err = vrf.Verify(
		vrfKey,
		vrfResult.Proof,
		vrfResult.Output,
		vrfMsg,
	)
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

	if !vrfValid {
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeVRF,
			"VRF output mismatch",
			map[string]any{
				"slot":           slot,
				"block_number":   blockNo,
				"era":            era,
				"vrf_output_len": len(vrfResult.Output),
				"vrf_proof_len":  len(vrfResult.Proof),
			},
			nil,
		)
	}

	vrfHex = hex.EncodeToString(vrfResult.Output)

	// KES verification
	// Extract the original body CBOR from the stored header CBOR.
	// The header is encoded as [body, signature] in CBOR. We must use
	// the original body bytes (not re-encoded) because the KES signature
	// is computed over the exact original CBOR encoding.
	bodyCbor, err := extractOriginalBodyCbor(block.Header())
	if err != nil {
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeProtocol,
			"failed to extract header body CBOR for KES verification",
			map[string]any{
				"slot":         slot,
				"block_number": blockNo,
				"era":          era,
			},
			err,
		)
	}
	if len(bodyCbor) == 0 {
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeProtocol,
			"empty body CBOR from extractOriginalBodyCbor",
			map[string]any{
				"slot":         slot,
				"block_number": blockNo,
				"era":          era,
			},
			nil,
		)
	}
	signature, hotVkey, kesPeriod, err := ExtractKesFields(block.Header())
	if err != nil {
		return false, "", 0, 0, err
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
	if !kesValid {
		return false, "", 0, 0, common.NewValidationError(
			common.ValidationErrorTypeKES,
			"KES signature invalid",
			map[string]any{
				"slot":         slot,
				"block_number": blockNo,
				"era":          era,
			},
			nil,
		)
	}

	// Verify block body hash (can be skipped via config)
	// Intended usage: production should keep this enabled for security.
	// Tests or environments lacking full block CBOR may set
	// VerifyConfig{SkipBodyHashValidation:true} to bypass this check.
	expectedBodyHash := block.BlockBodyHash()
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
		if era == dijkstra.EraDijkstra {
			// A Dijkstra (prototype-2026w27) block is [header, block_body];
			// its block_body_hash is blake2b256 over the block_body CBOR, not
			// the pre-Dijkstra concatenation of per-segment hashes.
			if err := validateDijkstraBlockBodyHash(rawCbor, expectedBodyHash); err != nil {
				return false, "", 0, 0, fmt.Errorf(
					"VerifyBlock: %w",
					err,
				)
			}
		} else {
			minLength := 4
			if eraAtLeast(era, alonzo.EraAlonzo) {
				minLength = 5
			}
			if err := common.ValidateBlockBodyHash(rawCbor, expectedBodyHash, eraName, minLength); err != nil {
				return false, "", 0, 0, fmt.Errorf(
					"VerifyBlock: %w",
					err,
				)
			}
		}
	}

	// Verify block-wide execution-unit budget (BBODY: sum of every
	// transaction's ExUnits must not exceed ppMaxBlockExUnits) and the block
	// body/header sizes against ppMaxBlockBodySize/ppMaxBlockHeaderSize,
	// using the exact serialized CBOR representation (can be skipped via
	// config).
	//
	// This is intentionally independent of SkipTransactionValidation: it is
	// a block-local structural/resource check, not a per-transaction UTxO
	// rule, so it needs neither LedgerState nor a full per-transaction pass.
	// It runs before the per-transaction validation loop below so that an
	// oversized or over-budget block is rejected cheaply, without paying
	// the cost of fully validating (and potentially executing Plutus
	// scripts for) every transaction in a block that is doomed regardless.
	// It only runs when there is something to check (at least one
	// transaction, for the ExUnits budget, or the block's raw CBOR is
	// available, for the size checks) and ProtocolParameters is set;
	// blockLevelLimits type-asserts config.ProtocolParameters to the era's
	// concrete pparams struct (there is no shared getter across eras, and
	// no other way to confirm it matches the block's actual era), mirroring
	// the same assumption the per-era rules make. An unrecognized
	// ProtocolParameters implementation is a hard configuration error (see
	// blockLevelLimits); callers who don't want block-limit enforcement must
	// opt out explicitly via config.SkipBlockLimitsValidation.
	if block.Era() != byron.EraByron && !config.SkipBlockLimitsValidation &&
		config.ProtocolParameters != nil {
		txs := block.Transactions()
		rawBlockCbor := block.Cbor()
		headerCbor := header.Cbor()
		if len(txs) > 0 || len(rawBlockCbor) > 0 || len(headerCbor) > 0 {
			maxBodySize, maxHeaderSize, maxExUnits, hasMaxExUnits, limitsErr := blockLevelLimits(
				config.ProtocolParameters,
			)
			if limitsErr != nil {
				return false, "", 0, 0, common.NewValidationError(
					common.ValidationErrorTypeConfiguration,
					"unable to determine block-wide limits from protocol parameters",
					map[string]any{
						"block_slot":   slot,
						"block_number": blockNo,
						"era":          era,
					},
					limitsErr,
				)
			}
			// A zero-value MaxBlockExUnits (both Memory and Steps unset)
			// is treated as "no limit" and skipped, consistent with the
			// maxBodySize > 0 / maxHeaderSize > 0 guards below.
			hasNonZeroMaxExUnits := maxExUnits.Memory > 0 ||
				maxExUnits.Steps > 0
			if hasMaxExUnits && hasNonZeroMaxExUnits && len(txs) > 0 {
				totalExUnits, sumErr := sumBlockExUnits(txs)
				if sumErr != nil {
					return false, "", 0, 0, common.NewValidationError(
						common.ValidationErrorTypeTransaction,
						"failed to sum block execution units",
						map[string]any{
							"block_slot":   slot,
							"block_number": blockNo,
							"era":          era,
						},
						sumErr,
					)
				}
				if totalExUnits.Memory > maxExUnits.Memory ||
					totalExUnits.Steps > maxExUnits.Steps {
					return false, "", 0, 0, common.NewValidationError(
						common.ValidationErrorTypeProtocol,
						"block total execution units exceed protocol maximum",
						map[string]any{
							"block_slot":   slot,
							"block_number": blockNo,
							"era":          era,
							"total_memory": totalExUnits.Memory,
							"total_steps":  totalExUnits.Steps,
							"max_memory":   maxExUnits.Memory,
							"max_steps":    maxExUnits.Steps,
						},
						common.BlockExUnitsTooBigError{
							TotalExUnits:    totalExUnits,
							MaxBlockExUnits: maxExUnits,
						},
					)
				}
			}
			// headerSize is derived from the header's own preserved CBOR
			// bytes, which is independent of whether the full block's raw
			// CBOR (rawBlockCbor) is available, so this check must not be
			// nested inside the rawBlockCbor guard below.
			if len(headerCbor) > 0 && maxHeaderSize > 0 {
				headerSize := uint64(len(headerCbor))
				if headerSize > maxHeaderSize {
					return false, "", 0, 0, common.NewValidationError(
						common.ValidationErrorTypeProtocol,
						"block header size exceeds protocol maximum",
						map[string]any{
							"block_slot":      slot,
							"block_number":    blockNo,
							"era":             era,
							"header_size":     headerSize,
							"max_header_size": maxHeaderSize,
						},
						common.BlockHeaderSizeTooBigError{
							HeaderSize:         headerSize,
							MaxBlockHeaderSize: maxHeaderSize,
						},
					)
				}
			}
			// The body-size calculation requires the full raw block CBOR
			// (it sums the lengths of the top-level elements after the
			// header). It is gated on maxBodySize > 0 ("no limit") here,
			// not just at the comparison below, so it is never wastefully
			// computed when the limit is disabled.
			if len(rawBlockCbor) > 0 && maxBodySize > 0 {
				bodySize, sizeErr := common.BlockBodySizeFromCbor(rawBlockCbor)
				if sizeErr != nil {
					return false, "", 0, 0, common.NewValidationError(
						common.ValidationErrorTypeProtocol,
						"failed to calculate block body size",
						map[string]any{
							"block_slot":   slot,
							"block_number": blockNo,
							"era":          era,
						},
						sizeErr,
					)
				}
				if bodySize > maxBodySize {
					return false, "", 0, 0, common.NewValidationError(
						common.ValidationErrorTypeProtocol,
						"block body size exceeds protocol maximum",
						map[string]any{
							"block_slot":    slot,
							"block_number":  blockNo,
							"era":           era,
							"body_size":     bodySize,
							"max_body_size": maxBodySize,
						},
						common.BlockBodySizeTooBigError{
							BlockBodySize:    bodySize,
							MaxBlockBodySize: maxBodySize,
						},
					)
				}
			}
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
		case dijkstra.EraDijkstra.Id:
			validationRules = dijkstra.UtxoValidationRules
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
		if dijkstraBlock, ok := block.(*dijkstra.DijkstraBlock); ok {
			if err := dijkstra.ValidateRefScriptSizePerBlock(dijkstraBlock, config.ProtocolParameters); err != nil {
				return false, "", 0, 0, common.NewValidationError(
					common.ValidationErrorTypeTransaction,
					"block reference-script size validation failed",
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

	return true, vrfHex, blockNo, slot, nil
}

type BlockHexCbor struct {
	cbor.StructAsArray
	Flag          int
	HeaderCbor    string
	Eta0          string
	Spk           int
	BlockBodyCbor string
}
