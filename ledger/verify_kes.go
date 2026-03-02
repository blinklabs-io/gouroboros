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
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/leios"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// This module inspired by https://github.com/input-output-hk/kes,
// special thanks to https://github.com/iquerejeta, who helped me a lot on this journey

// ExtractKesFields extracts KES signature, hot verification key, and KES period
// from a block header of any supported era. Returns a ValidationError for
// unsupported header types.
func ExtractKesFields(
	header common.BlockHeader,
) (signature []byte, hotVkey []byte, kesPeriod uint64, err error) {
	switch h := header.(type) {
	case *shelley.ShelleyBlockHeader:
		return h.Signature, h.Body.OpCertHotVkey, uint64(h.Body.OpCertKesPeriod), nil
	case *allegra.AllegraBlockHeader:
		return h.Signature, h.Body.OpCertHotVkey, uint64(h.Body.OpCertKesPeriod), nil
	case *mary.MaryBlockHeader:
		return h.Signature, h.Body.OpCertHotVkey, uint64(h.Body.OpCertKesPeriod), nil
	case *alonzo.AlonzoBlockHeader:
		return h.Signature, h.Body.OpCertHotVkey, uint64(h.Body.OpCertKesPeriod), nil
	case *babbage.BabbageBlockHeader:
		return h.Signature, h.Body.OpCert.HotVkey, uint64(h.Body.OpCert.KesPeriod), nil
	case *conway.ConwayBlockHeader:
		return h.Signature, h.Body.OpCert.HotVkey, uint64(h.Body.OpCert.KesPeriod), nil
	case *leios.LeiosBlockHeader:
		return h.Signature, h.Body.OpCert.HotVkey, uint64(h.Body.OpCert.KesPeriod), nil
	default:
		return nil, nil, 0, common.NewValidationError(
			common.ValidationErrorTypeProtocol,
			"unsupported block type for KES verification",
			map[string]any{
				"block_type":   fmt.Sprintf("%T", header),
				"slot":         header.SlotNumber(),
				"block_number": header.BlockNumber(),
			},
			nil,
		)
	}
}

// VerifyKes verifies the KES signature on a block header.
func VerifyKes(
	header common.BlockHeader,
	slotsPerKesPeriod uint64,
) (bool, error) {
	// Ref: https://github.com/IntersectMBO/ouroboros-consensus/blob/de74882102236fdc4dd25aaa2552e8b3e208448c/ouroboros-consensus-cardano/src/shelley/Ouroboros/Consensus/Shelley/Protocol/Praos.hs#L125
	// Ref: https://github.com/IntersectMBO/cardano-ledger/blob/master/libs/cardano-protocol-tpraos/src/Cardano/Protocol/TPraos/BHeader.hs#L189

	// Extract the original body CBOR stored at decode time.
	// The KES signature is over the exact original CBOR encoding,
	// so we must NOT re-encode via cbor.Encode.
	bodyCbor, err := extractOriginalBodyCbor(header)
	if err != nil {
		return false, fmt.Errorf("VerifyKes: %w", err)
	}
	if len(bodyCbor) == 0 {
		return false, fmt.Errorf(
			"VerifyKes: no stored body CBOR for header type %T",
			header,
		)
	}

	signature, hotVkey, kesPeriod, err := ExtractKesFields(header)
	if err != nil {
		return false, fmt.Errorf("VerifyKes: %w", err)
	}

	return VerifyKesComponents(
		bodyCbor,
		signature,
		hotVkey,
		kesPeriod,
		header.SlotNumber(),
		slotsPerKesPeriod,
	)
}

// VerifyKesComponents verifies KES signature components directly.
func VerifyKesComponents(
	bodyCbor []byte,
	signature []byte,
	hotVkey []byte,
	kesPeriod uint64,
	slot uint64,
	slotsPerKesPeriod uint64,
) (bool, error) {
	if slotsPerKesPeriod == 0 {
		return false, errors.New("slotsPerKesPeriod must be greater than 0")
	}
	// Validate KES signature length
	if len(signature) != kes.CardanoKesSignatureSize {
		return false, fmt.Errorf(
			"invalid KES signature length: expected %d bytes, got %d",
			kes.CardanoKesSignatureSize,
			len(signature),
		)
	}
	currentKesPeriod := slot / slotsPerKesPeriod
	if currentKesPeriod < kesPeriod {
		// Certificate start period is in the future - invalid operational certificate
		return false, nil
	}
	t := currentKesPeriod - kesPeriod
	return kes.VerifySignedKES(hotVkey, t, bodyCbor, signature), nil
}

// GetHeaderBodyCbor returns re-encoded CBOR bytes of the block header body.
// WARNING: This re-encodes via cbor.Encode, which may produce different bytes
// than the original on-wire encoding. Do NOT use for KES signature verification;
// use extractOriginalBodyCbor (which returns the stored original CBOR) instead.
func GetHeaderBodyCbor(header common.BlockHeader) ([]byte, error) {
	switch h := header.(type) {
	case *shelley.ShelleyBlockHeader:
		return cbor.Encode(h.Body)
	case *allegra.AllegraBlockHeader:
		return cbor.Encode(h.Body)
	case *mary.MaryBlockHeader:
		return cbor.Encode(h.Body)
	case *alonzo.AlonzoBlockHeader:
		return cbor.Encode(h.Body)
	case *babbage.BabbageBlockHeader:
		return cbor.Encode(h.Body)
	case *conway.ConwayBlockHeader:
		return cbor.Encode(h.Body)
	case *leios.LeiosBlockHeader:
		return cbor.Encode(h.Body)
	default:
		return nil, fmt.Errorf("GetHeaderBodyCbor: unsupported block header type %T", header)
	}
}
