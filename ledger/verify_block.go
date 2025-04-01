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

	"github.com/blinklabs-io/gouroboros/cbor"
)

//nolint:staticcheck
func VerifyBlock(block BlockHexCbor) (error, bool, string, uint64, uint64) {
	headerCborHex := block.HeaderCbor
	epochNonceHex := block.Eta0
	bodyHex := block.BlockBodyCbor
	slotPerKesPeriod := uint64(block.Spk) // #nosec G115

	isValid := false
	vrfHex := ""

	// check is KES valid
	headerCborByte, headerDecodeError := hex.DecodeString(headerCborHex)
	if headerDecodeError != nil {
		return fmt.Errorf(
			"VerifyBlock: headerCborByte decode error, %v",
			headerDecodeError.Error(),
		), false, "", 0, 0
	}
	header, headerUnmarshalError := NewBabbageBlockHeaderFromCbor(
		headerCborByte,
	)
	if headerUnmarshalError != nil {
		return fmt.Errorf(
			"VerifyBlock: header unmarshal error, %v",
			headerUnmarshalError.Error(),
		), false, "", 0, 0
	}
	if header == nil {
		return errors.New("VerifyBlock: header returned empty"), false, "", 0, 0
	}
	isKesValid, errKes := VerifyKes(header, slotPerKesPeriod)
	if errKes != nil {
		return fmt.Errorf(
			"VerifyBlock: KES invalid, %v",
			errKes.Error(),
		), false, "", 0, 0
	}

	// check is VRF valid
	// Ref: https://github.com/IntersectMBO/ouroboros-consensus/blob/de74882102236fdc4dd25aaa2552e8b3e208448c/ouroboros-consensus-protocol/src/ouroboros-consensus-protocol/Ouroboros/Consensus/Protocol/Praos.hs#L541
	epochNonceByte, epochNonceDecodeError := hex.DecodeString(epochNonceHex)
	if epochNonceDecodeError != nil {
		return fmt.Errorf(
			"VerifyBlock: epochNonceByte decode error, %v",
			epochNonceDecodeError.Error(),
		), false, "", 0, 0
	}
	vrfBytes := header.Body.VrfKey[:]
	vrfResult := header.Body.VrfResult
	seed := MkInputVrf(int64(header.Body.Slot), epochNonceByte) // #nosec G115
	output, errVrf := VrfVerifyAndHash(vrfBytes, vrfResult.Proof, seed)
	if errVrf != nil {
		return fmt.Errorf(
			"VerifyBlock: vrf invalid, %v",
			errVrf.Error(),
		), false, "", 0, 0
	}
	isVrfValid := bytes.Equal(output, vrfResult.Output)

	// check if block data valid
	blockBodyHash := header.Body.BlockBodyHash
	blockBodyHashHex := hex.EncodeToString(blockBodyHash[:])
	isBodyValid, isBodyValidError := VerifyBlockBody(bodyHex, blockBodyHashHex)
	if isBodyValidError != nil {
		return fmt.Errorf(
			"VerifyBlock: VerifyBlockBody error, %v",
			isBodyValidError.Error(),
		), false, "", 0, 0
	}
	isValid = isKesValid && isVrfValid && isBodyValid
	vrfHex = hex.EncodeToString(vrfBytes)
	blockNo := header.Body.BlockNumber
	slotNo := header.Body.Slot
	return nil, isValid, vrfHex, blockNo, slotNo
}

func ExtractBlockData(
	bodyHex string,
) ([]UTXOOutput, []RegisCert, []DeRegisCert, error) {
	rawDataBytes, rawDataBytesError := hex.DecodeString(bodyHex)
	if rawDataBytesError != nil {
		return nil, nil, nil, fmt.Errorf(
			"ExtractBlockData: bodyHex decode error, %v",
			rawDataBytesError.Error(),
		)
	}
	var txsRaw [][]string
	_, err := cbor.Decode(rawDataBytes, &txsRaw)
	if err != nil {
		return nil, nil, nil, fmt.Errorf(
			"ExtractBlockData: txsRaw decode error, %v",
			err.Error(),
		)
	}
	txBodies, txBodiesError := GetTxBodies(txsRaw)
	if txBodiesError != nil {
		return nil, nil, nil, fmt.Errorf(
			"ExtractBlockData: GetTxBodies error, %v",
			txBodiesError.Error(),
		)
	}
	uTXOOutput, regisCerts, deRegisCerts, getBlockOutputError := GetBlockOutput(
		txBodies,
	)
	if getBlockOutputError != nil {
		return nil, nil, nil, fmt.Errorf(
			"ExtractBlockData: GetBlockOutput error, %v",
			getBlockOutputError.Error(),
		)
	}
	return uTXOOutput, regisCerts, deRegisCerts, nil
}

// These are copied from types.go

type BlockHexCbor struct {
	_             struct{} `cbor:",toarray"`
	Flag          int
	HeaderCbor    string
	Eta0          string
	Spk           int
	BlockBodyCbor string
}
