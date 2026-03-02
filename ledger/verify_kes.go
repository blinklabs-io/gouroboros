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

func extractKesFieldsShelleyAlonzo(
	header any,
) (bodyCbor []byte, sig []byte, hotVkey []byte, kesPeriod uint64, slot uint64, err error) {
	switch h := header.(type) {
	case *shelley.ShelleyBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return nil, nil, nil, 0, 0, err
		}
		return bodyCbor, h.Signature, h.Body.OpCertHotVkey, uint64(h.Body.OpCertKesPeriod), h.Body.Slot, nil
	case *allegra.AllegraBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return nil, nil, nil, 0, 0, err
		}
		return bodyCbor, h.Signature, h.Body.OpCertHotVkey, uint64(h.Body.OpCertKesPeriod), h.Body.Slot, nil
	case *mary.MaryBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return nil, nil, nil, 0, 0, err
		}
		return bodyCbor, h.Signature, h.Body.OpCertHotVkey, uint64(h.Body.OpCertKesPeriod), h.Body.Slot, nil
	case *alonzo.AlonzoBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return nil, nil, nil, 0, 0, err
		}
		return bodyCbor, h.Signature, h.Body.OpCertHotVkey, uint64(h.Body.OpCertKesPeriod), h.Body.Slot, nil
	default:
		return nil, nil, nil, 0, 0, fmt.Errorf("unsupported header type: %T", header)
	}
}

func extractKesFieldsBabbagePlus(
	header any,
) (bodyCbor []byte, sig []byte, hotVkey []byte, kesPeriod uint64, slot uint64, err error) {
	switch h := header.(type) {
	case *babbage.BabbageBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return nil, nil, nil, 0, 0, err
		}
		return bodyCbor, h.Signature, h.Body.OpCert.HotVkey, uint64(h.Body.OpCert.KesPeriod), h.Body.Slot, nil
	case *conway.ConwayBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return nil, nil, nil, 0, 0, err
		}
		return bodyCbor, h.Signature, h.Body.OpCert.HotVkey, uint64(h.Body.OpCert.KesPeriod), h.Body.Slot, nil
	case *leios.LeiosBlockHeader:
		bodyCbor, err = cbor.Encode(h.Body)
		if err != nil {
			return nil, nil, nil, 0, 0, err
		}
		return bodyCbor, h.Signature, h.Body.OpCert.HotVkey, uint64(h.Body.OpCert.KesPeriod), h.Body.Slot, nil
	default:
		return nil, nil, nil, 0, 0, fmt.Errorf("unsupported header type: %T", header)
	}
}

// VerifyKes verifies the KES signature on a block header.
func VerifyKes(
	header common.BlockHeader,
	slotsPerKesPeriod uint64,
) (bool, error) {
	// Ref: https://github.com/IntersectMBO/ouroboros-consensus/blob/de74882102236fdc4dd25aaa2552e8b3e208448c/ouroboros-consensus-cardano/src/shelley/Ouroboros/Consensus/Shelley/Protocol/Praos.hs#L125
	// Ref: https://github.com/IntersectMBO/cardano-ledger/blob/master/libs/cardano-protocol-tpraos/src/Cardano/Protocol/TPraos/BHeader.hs#L189
	var msgBytes []byte
	var signature []byte
	var hotVkey []byte
	var kesPeriod uint64
	var slot uint64
	var err error

	switch h := header.(type) {
	case *shelley.ShelleyBlockHeader, *allegra.AllegraBlockHeader, *mary.MaryBlockHeader, *alonzo.AlonzoBlockHeader:
		msgBytes, signature, hotVkey, kesPeriod, slot, err = extractKesFieldsShelleyAlonzo(h)
		if err != nil {
			return false, fmt.Errorf("VerifyKes: %w", err)
		}
	case *babbage.BabbageBlockHeader, *conway.ConwayBlockHeader, *leios.LeiosBlockHeader:
		msgBytes, signature, hotVkey, kesPeriod, slot, err = extractKesFieldsBabbagePlus(h)
		if err != nil {
			return false, fmt.Errorf("VerifyKes: %w", err)
		}
	default:
		return false, fmt.Errorf("VerifyKes: unsupported block header type %T", header)
	}

	return VerifyKesComponents(
		msgBytes,
		signature,
		hotVkey,
		kesPeriod,
		slot,
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

// GetHeaderBodyCbor returns the CBOR-encoded bytes of the block header body.
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
