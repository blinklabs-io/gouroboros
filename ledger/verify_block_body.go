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
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"golang.org/x/crypto/blake2b"
)

func convertToAnySlice[T any](slice []T) []any {
	result := make([]any, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}

const (
	LOVELACE_TOKEN              = "lovelace"
	BLOCK_BODY_HASH_ZERO_TX_HEX = "29571d16f081709b3c48651860077bebf9340abb3fc7133443c54f1f5a5edcf1"
	MAX_LIST_LENGTH_CBOR        = 23
)

// MaxVerifyBlockBodyBytes bounds the decoded CBOR passed to VerifyBlockBody.
// The input carries each transaction field as hex text, twice its raw size,
// so this admits a 2,097,154-byte block (the largest block size the
// reference implementation's block-fetch limits allow for) with room for
// the per-transaction CBOR headers.
const MaxVerifyBlockBodyBytes = 4 * 2_097_154

// MaxVerifyBlockBodyTxs bounds the transaction entries VerifyBlockBody
// accepts, in definite- and indefinite-length bodies alike. The smallest
// transaction body, one input with no outputs and a zero fee, is 43 bytes, so
// a 2,097,154-byte block cannot hold more.
const MaxVerifyBlockBodyTxs = 2_097_154 / 43

// minBlockBodyTxBytes is the smallest encoding of one transaction entry, an
// array header and three empty text strings. It bounds how many entries a
// definite-length body of a given size can hold.
const minBlockBodyTxBytes = 4

// blockBodyTxFields is the number of fields in a transaction entry: body,
// witness set and auxiliary data.
const blockBodyTxFields = 3

// VerifyBlockBody verifies that the block body hash matches the expected hash.
// The invalidTxIndices parameter contains indices of transactions that failed
// phase-2 validation (Plutus script failures). For blocks with no invalid
// transactions, pass an empty slice.
func VerifyBlockBody(
	data string,
	blockBodyHash string,
	invalidTxIndices []uint,
) (bool, error) {
	// hex.DecodeString returns the bytes decoded before the offending
	// character alongside its error, so discarding the error would verify a
	// silently truncated body against the caller's hash.
	if len(data) > 2*MaxVerifyBlockBodyBytes {
		return false, fmt.Errorf(
			"VerifyBlockBody: data size %d exceeds maximum %d bytes",
			len(data)/2,
			MaxVerifyBlockBodyBytes,
		)
	}
	rawDataBytes, decodeDataError := hex.DecodeString(data)
	if decodeDataError != nil {
		return false, fmt.Errorf(
			"VerifyBlockBody: data decode error, %v",
			decodeDataError.Error(),
		)
	}
	txsRaw, err := decodeBlockBodyTxs(rawDataBytes)
	if err != nil {
		return false, fmt.Errorf(
			"VerifyBlockBody: txs decode error, %v",
			err.Error(),
		)
	}

	blockBodyHashByte, decodeBBHError := hex.DecodeString(blockBodyHash)
	if decodeBBHError != nil {
		return false, fmt.Errorf(
			"VerifyBlockBody: blockBodyHashByte decode error, %v",
			decodeBBHError.Error(),
		)
	}

	var calculateBlockBodyHashByte [32]byte
	if len(txsRaw) == 0 {
		zeroTxHash, decodeZTHError := hex.DecodeString(
			BLOCK_BODY_HASH_ZERO_TX_HEX,
		)
		if decodeZTHError != nil {
			return false, fmt.Errorf(
				"VerifyBlockBody: zeroTxHash decode error, %v",
				decodeZTHError.Error(),
			)
		}
		copy(calculateBlockBodyHashByte[:], zeroTxHash[:32])
	} else {
		calculateBlockBodyHash, calculateHashError := CalculateBlockBodyHash(txsRaw, invalidTxIndices)
		if calculateHashError != nil {
			return false, fmt.Errorf("VerifyBlockBody: CalculateBlockBodyHash error, %v", calculateHashError.Error())
		}
		calculateBlockBodyHashByte = blake2b.Sum256(calculateBlockBodyHash)
	}
	if !bytes.Equal(calculateBlockBodyHashByte[:32], blockBodyHashByte) {
		return false, fmt.Errorf(
			"body hash mismatch, derived bodyHex: %s",
			hex.EncodeToString(calculateBlockBodyHashByte[:]),
		)
	}
	return true, nil
}

// decodeBlockBodyTxs decodes the transaction entries of a block body. A
// definite-length array's declared count is checked against the bytes that
// follow before anything is allocated for it, and each entry's shape is
// checked before it is decoded, so storage grows only with validated entries.
func decodeBlockBodyTxs(data []byte) ([][]string, error) {
	count, headerSize, indefinite := cbor.ArrayInfo(data)
	if count < 0 {
		return nil, errors.New("block body is not a CBOR array")
	}
	body := data[headerSize:]
	if count > MaxVerifyBlockBodyTxs {
		return nil, fmt.Errorf(
			"block body declares %d transactions, maximum %d",
			count,
			MaxVerifyBlockBodyTxs,
		)
	}
	if !indefinite && count > len(body)/minBlockBodyTxBytes {
		return nil, fmt.Errorf(
			"block body declares %d transactions in %d bytes",
			count,
			len(body),
		)
	}
	dec, err := cbor.NewStreamDecoder(body)
	if err != nil {
		return nil, err
	}
	var txs [][]string
	for index := 0; indefinite || index < count; index++ {
		if indefinite {
			pos := dec.Position()
			if pos >= len(body) {
				return nil, errors.New(
					"block body indefinite-length array is not terminated",
				)
			}
			if body[pos] == 0xff {
				break
			}
			if index >= MaxVerifyBlockBodyTxs {
				return nil, fmt.Errorf(
					"block body exceeds maximum %d transactions",
					MaxVerifyBlockBodyTxs,
				)
			}
		}
		entry := body[dec.Position():]
		entryCount, entryHeaderSize, entryIndefinite := cbor.ArrayInfo(entry)
		if entryIndefinite {
			entryCount, err = countIndefiniteItems(
				entry[entryHeaderSize:],
				blockBodyTxFields+1,
			)
			if err != nil {
				return nil, fmt.Errorf("tx index %d: %w", index, err)
			}
		}
		if entryCount != blockBodyTxFields {
			return nil, fmt.Errorf(
				"tx index %d: expected 3 fields, got %d",
				index,
				entryCount,
			)
		}
		var tx []string
		if _, _, err := dec.Decode(&tx); err != nil {
			return nil, fmt.Errorf("tx index %d: %w", index, err)
		}
		txs = append(txs, tx)
	}
	return txs, nil
}

// countIndefiniteItems counts the items of an indefinite-length array whose
// header has already been consumed, stopping once limit items are seen.
func countIndefiniteItems(data []byte, limit int) (int, error) {
	dec, err := cbor.NewStreamDecoder(data)
	if err != nil {
		return 0, err
	}
	count := 0
	for count < limit {
		pos := dec.Position()
		if pos >= len(data) {
			return 0, errors.New("indefinite-length array is not terminated")
		}
		if data[pos] == 0xff {
			break
		}
		if _, _, err := dec.Skip(); err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

func encodeCborSequence[T any](data []T) ([]byte, error) {
	if len(data) == 0 {
		data = make([]T, 0)
	}
	if len(data) <= MAX_LIST_LENGTH_CBOR {
		return cbor.Encode(data)
	}
	return cbor.Encode(cbor.IndefLengthList(convertToAnySlice(data)))
}

func EncodeCborTxSeq(data []uint) ([]byte, error) {
	// Cardano base uses an indefinite-length list for more than 23 items.
	// https://github.com/IntersectMBO/cardano-base/blob/e86a25c54389ddd0f77fdbc3f3615c57bd91d543/cardano-binary/src/Cardano/Binary/ToCBOR.hs#L708C10-L708C28
	return encodeCborSequence(data)
}

type AuxData struct {
	index uint64
	data  []byte
}

// EncodeCborMap manual build aux bytes data
func encodeAuxData(data []AuxData) ([]byte, error) {
	dataLen := len(data)
	if dataLen == 0 {
		txSeqMetadata := make(map[uint64]any)
		return cbor.Encode(txSeqMetadata)
	}
	if dataLen <= MAX_LIST_LENGTH_CBOR {
		// Use definite length map
		metadataMap := make(map[uint64]any)
		for _, aux := range data {
			metadataMap[aux.index] = cbor.RawMessage(aux.data)
		}
		return cbor.Encode(metadataMap)
	}
	// Use indefinite length map
	metadataMap := make(map[any]any)
	for _, aux := range data {
		metadataMap[aux.index] = cbor.RawMessage(aux.data)
	}
	return cbor.Encode(cbor.IndefLengthMap(metadataMap))
}

// CalculateBlockBodyHash computes the block body hash from raw transaction data.
// The invalidTxIndices parameter contains indices of transactions that failed
// phase-2 validation (Plutus script failures). For blocks with no invalid
// transactions, pass an empty slice.
func CalculateBlockBodyHash(
	txsRaw [][]string,
	invalidTxIndices []uint,
) ([]byte, error) {
	// Check every entry before reserving storage sized by the entry count.
	for index, tx := range txsRaw {
		if len(tx) != 3 {
			return nil, fmt.Errorf(
				"CalculateBlockBodyHash: tx len error, tx index %v length = %v",
				index,
				len(tx),
			)
		}
	}
	txSeqBody := make([]cbor.RawMessage, 0, len(txsRaw))
	txSeqWit := make([]cbor.RawMessage, 0, len(txsRaw))
	var auxRawData []AuxData
	for index, tx := range txsRaw {
		bodyTmpHex := tx[0]
		bodyTmpBytes, bodyTmpBytesError := hex.DecodeString(bodyTmpHex)
		if bodyTmpBytesError != nil {
			return nil, fmt.Errorf(
				"CalculateBlockBodyHash: decode body tx[%v] error, %v",
				index,
				bodyTmpBytesError.Error(),
			)
		}
		txSeqBody = append(txSeqBody, bodyTmpBytes)

		witTmpHex := tx[1]
		witTmpBytes, witTmpBytesError := hex.DecodeString(witTmpHex)
		if witTmpBytesError != nil {
			return nil, fmt.Errorf(
				"CalculateBlockBodyHash: decode wit tx[%v] error, %v",
				index,
				witTmpBytesError.Error(),
			)
		}

		txSeqWit = append(txSeqWit, witTmpBytes)

		auxTmpHex := tx[2]
		if auxTmpHex != "" {
			auxBytes, auxBytesError := hex.DecodeString(auxTmpHex)
			if auxBytesError != nil {
				return nil, fmt.Errorf(
					"CalculateBlockBodyHash: decode aux tx[%v] error, %v",
					index,
					auxBytesError.Error(),
				)
			}
			// #nosec G115
			auxRawData = append(auxRawData, AuxData{
				index: uint64(index),
				data:  auxBytes,
			})
		}
	}
	txSeqBodyBytes, txSeqBodyBytesError := encodeCborSequence(txSeqBody)
	if txSeqBodyBytesError != nil {
		return nil, fmt.Errorf(
			"CalculateBlockBodyHash: encode txSeqBody error, %v",
			txSeqBodyBytesError.Error(),
		)
	}

	txSeqBodySum32Bytes := blake2b.Sum256(txSeqBodyBytes)
	txSeqBodySumBytes := txSeqBodySum32Bytes[:]

	txSeqWitsBytes, txSeqWitsBytesError := encodeCborSequence(txSeqWit)
	if txSeqWitsBytesError != nil {
		return nil, fmt.Errorf(
			"CalculateBlockBodyHash: encode txSeqWit error, %v",
			txSeqWitsBytesError.Error(),
		)
	}
	txSeqWitsSum32Bytes := blake2b.Sum256(txSeqWitsBytes)
	txSeqWitsSumBytes := txSeqWitsSum32Bytes[:]

	txSeqMetadataBytes, txSeqMetadataBytesError := encodeAuxData(auxRawData)
	if txSeqMetadataBytesError != nil {
		return nil, fmt.Errorf(
			"CalculateBlockBodyHash: encode txSeqMetadata error, %v",
			txSeqMetadataBytesError.Error(),
		)
	}
	txSeqMetadataSum32Bytes := blake2b.Sum256(txSeqMetadataBytes)
	txSeqMetadataSumBytes := txSeqMetadataSum32Bytes[:]

	// Encode the list of invalid transaction indices (failed phase-2 validation)
	// Use EncodeCborTxSeq to ensure correct encoding: definite-length for ≤23 items,
	// indefinite-length for >23 items (per Cardano CBOR spec)
	txSeqNonValidBytes, txSeqNonValidBytesError := EncodeCborTxSeq(
		invalidTxIndices,
	)
	if txSeqNonValidBytesError != nil {
		return nil, fmt.Errorf(
			"CalculateBlockBodyHash: encode txSeqNonValid error, %v",
			txSeqNonValidBytesError.Error(),
		)
	}
	txSeqIsValidSum32Bytes := blake2b.Sum256(txSeqNonValidBytes)
	txSeqIsValidSumBytes := txSeqIsValidSum32Bytes[:]

	serializeBytes := make(
		[]byte,
		0,
		len(
			txSeqBodySumBytes,
		)+len(
			txSeqWitsSumBytes,
		)+len(
			txSeqMetadataSumBytes,
		)+len(
			txSeqIsValidSumBytes,
		),
	)
	// Ref: https://github.com/IntersectMBO/cardano-ledger/blob/9cc766a31ad6fb31f88e25a770c902d24fa32499/eras/alonzo/impl/src/Cardano/Ledger/Alonzo/TxSeq.hs#L183
	serializeBytes = append(serializeBytes, txSeqBodySumBytes...)
	serializeBytes = append(serializeBytes, txSeqWitsSumBytes...)
	serializeBytes = append(serializeBytes, txSeqMetadataSumBytes...)
	serializeBytes = append(serializeBytes, txSeqIsValidSumBytes...)

	return serializeBytes, nil
}

func GetTxBodies(txsRaw [][]string) ([]BabbageTransactionBody, error) {
	bodies := make([]BabbageTransactionBody, 0, len(txsRaw))
	for index, tx := range txsRaw {
		if len(tx) == 0 {
			return nil, fmt.Errorf(
				"CalculateBlockBodyHash: transaction at index %d: expected at least 1 field, got 0",
				index,
			)
		}
		var tmp BabbageTransactionBody
		bodyTmpHex := tx[0]
		bodyTmpBytes, bodyTmpBytesError := hex.DecodeString(bodyTmpHex)
		if bodyTmpBytesError != nil {
			return nil, fmt.Errorf(
				"CalculateBlockBodyHash: decode bodyTmpBytesError, index %v, error, %v",
				index,
				bodyTmpBytesError.Error(),
			)
		}
		_, err := cbor.Decode(bodyTmpBytes, &tmp)
		if err != nil {
			return nil, fmt.Errorf(
				"CalculateBlockBodyHash: decode bodyTmpBytes, index %v, error, %v",
				index,
				err.Error(),
			)
		}
		bodies = append(bodies, tmp)
	}
	return bodies, nil
}

func GetBlockOutput(
	txBodies []BabbageTransactionBody,
) ([]UTXOOutput, []common.PoolRegistrationCertificate, []common.PoolRetirementCertificate, error) {
	var outputs []UTXOOutput
	var regisCerts []common.PoolRegistrationCertificate
	var deRegisCerts []common.PoolRetirementCertificate
	for txIndex, tx := range txBodies {
		txId := tx.Id().String()
		txOutputs := tx.Outputs()
		for outputIndex, txOutput := range txOutputs {
			cborDatum := []byte{}
			if txOutput.Datum() != nil {
				cborDatum = txOutput.Datum().Cbor()
			}
			cborDatumHex := hex.EncodeToString(cborDatum)
			tokens, extractTokensError := ExtractTokens(txOutput)
			if extractTokensError != nil {
				return nil, nil, nil, fmt.Errorf(
					"GetBlockOutput: ExtractTokens error, tx index %v, outputIndex %v, error, %v",
					txIndex,
					outputIndex,
					extractTokensError.Error(),
				)
			}
			tmpOutput := UTXOOutput{
				TxHash:      txId,
				OutputIndex: strconv.Itoa(outputIndex),
				Tokens:      tokens,
				DatumHex:    cborDatumHex,
			}
			outputs = append(outputs, tmpOutput)
		}

		// Ref: https://github.com/IntersectMBO/cardano-ledger/blob/master/eras/babbage/impl/cddl-files/babbage.cddl#L193
		// We will only focus on:
		// pool_registration = (3, pool_params)
		// pool_retirement = (4, pool_keyhash, epoch)
		for _, cert := range tx.Certificates() {
			switch v := cert.(type) {
			case *PoolRegistrationCertificate:
				regisCerts = append(regisCerts, *v)

			case *PoolRetirementCertificate:
				// pool_retirement
				deRegisCerts = append(deRegisCerts, *v)
			}
		}
	}

	return outputs, regisCerts, deRegisCerts, nil
}

func ExtractTokens(output TransactionOutput) ([]UTXOOutputToken, error) {
	var outputTokens []UTXOOutputToken
	// append lovelace first
	tokenValue := "0"
	if output.Amount() != nil {
		tokenValue = output.Amount().String()
	}
	outputTokens = append(outputTokens, UTXOOutputToken{
		TokenAssetName: LOVELACE_TOKEN,
		TokenValue:     tokenValue,
	})
	if output.Assets() != nil {
		tmpAssets := output.Assets()
		for _, policyId := range tmpAssets.Policies() {
			for _, assetName := range tmpAssets.Assets(policyId) {
				amount := tmpAssets.Asset(policyId, assetName)
				tokenValue := "0"
				if amount != nil {
					tokenValue = amount.String()
				}
				outputTokens = append(outputTokens, UTXOOutputToken{
					TokenAssetName: policyId.String() + hex.EncodeToString(
						assetName,
					),
					TokenValue: tokenValue,
				})
			}
		}
	}
	return outputTokens, nil
}

// These are copied from types.go

type UTXOOutputToken struct {
	cbor.StructAsArray
	Flag           int
	TokenAssetName string
	TokenValue     string
}

type UTXOOutput struct {
	cbor.StructAsArray
	Flag        int
	TxHash      string
	OutputIndex string
	Tokens      []UTXOOutputToken
	DatumHex    string
}

func GetListRegisCertPoolId(
	regisCerts []common.PoolRegistrationCertificate,
) []string {
	poolId := make([]string, 0)
	if len(regisCerts) > 0 {
		for _, cert := range regisCerts {
			poolId = append(poolId, NewBlake2b224(cert.Operator[:]).String())
		}
	}
	return poolId
}

func GetListUnregisCertPoolId(
	deRegisCerts []common.PoolRetirementCertificate,
) []string {
	poolId := make([]string, 0)
	if len(deRegisCerts) > 0 {
		for _, cert := range deRegisCerts {
			poolId = append(poolId, NewBlake2b224(cert.PoolKeyHash[:]).String())
		}
	}
	return poolId
}
