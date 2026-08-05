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

package common

import (
	"errors"
	"fmt"
)

// InvalidIsValidFlagError indicates a tx marked invalid but lacking Plutus scripts
type InvalidIsValidFlagError struct{}

func (InvalidIsValidFlagError) Error() string {
	return "transaction marked as invalid but has no Plutus scripts requiring phase-2 validation"
}

// PlutusScriptValidationUnsupportedError indicates a transaction needs phase-2
// Plutus execution in an era path that cannot perform it.
type PlutusScriptValidationUnsupportedError struct {
	Era string
}

func (e PlutusScriptValidationUnsupportedError) Error() string {
	return e.Era + " Plutus phase-2 validation is not supported"
}

// MissingCostModelError indicates a missing cost model for a Plutus version
type MissingCostModelError struct {
	Version uint
}

func (e MissingCostModelError) Error() string {
	return fmt.Sprintf("missing cost model for Plutus v%d", e.Version+1)
}

// InputResolutionError indicates a failure to resolve a regular input UTxO
type InputResolutionError struct {
	Input TransactionInput
	Err   error
}

func (e InputResolutionError) Error() string {
	return fmt.Sprintf(
		"failed to resolve input %s: %v",
		e.Input.String(),
		e.Err,
	)
}

func (e InputResolutionError) Unwrap() error { return e.Err }

// Sentinel error for input resolution failures so callers can use errors.Is
var ErrInputResolution = errors.New(
	"input resolution failed",
)

func (InputResolutionError) Is(target error) bool {
	return target == ErrInputResolution
}

// ReferenceInputResolutionError indicates a failure to resolve a reference input UTxO
type ReferenceInputResolutionError struct {
	Input TransactionInput
	Err   error
}

func (e ReferenceInputResolutionError) Error() string {
	return fmt.Sprintf(
		"failed to resolve reference input %s: %v",
		e.Input.String(),
		e.Err,
	)
}

func (e ReferenceInputResolutionError) Unwrap() error { return e.Err }

// Sentinel error for reference input resolution failures so callers can use errors.Is
var ErrReferenceInputResolution = errors.New(
	"reference input resolution failed",
)

func (ReferenceInputResolutionError) Is(target error) bool {
	return target == ErrReferenceInputResolution
}

// InlineDatumsNotSupportedError indicates inline datums used with PlutusV1 scripts
type InlineDatumsNotSupportedError struct {
	PlutusVersion string
}

func (e InlineDatumsNotSupportedError) Error() string {
	return fmt.Sprintf(
		"inline datums are not supported with %s scripts - inline datums are a Babbage feature only available for PlutusV2+",
		e.PlutusVersion,
	)
}

// MissingScriptDataHashError indicates the transaction is missing a required ScriptDataHash
type MissingScriptDataHashError struct{}

func (MissingScriptDataHashError) Error() string {
	return "transaction requires a script data hash but none was provided"
}

// Sentinel error for missing script data hash so callers can use errors.Is
var ErrMissingScriptDataHash = errors.New("missing script data hash")

func (MissingScriptDataHashError) Is(target error) bool {
	return target == ErrMissingScriptDataHash
}

// ExtraneousScriptDataHashError indicates the transaction has a ScriptDataHash when none is needed
type ExtraneousScriptDataHashError struct {
	Provided Blake2b256
}

func (e ExtraneousScriptDataHashError) Error() string {
	return fmt.Sprintf(
		"transaction has script data hash %x but no Plutus scripts require it",
		e.Provided[:],
	)
}

// Sentinel error for extraneous script data hash so callers can use errors.Is
var ErrExtraneousScriptDataHash = errors.New("extraneous script data hash")

func (ExtraneousScriptDataHashError) Is(target error) bool {
	return target == ErrExtraneousScriptDataHash
}

// ScriptDataHashMismatchError indicates the declared ScriptDataHash doesn't match the computed hash
type ScriptDataHashMismatchError struct {
	Declared Blake2b256
	Computed Blake2b256
}

func (e ScriptDataHashMismatchError) Error() string {
	return fmt.Sprintf(
		"script data hash mismatch: declared %x, computed %x",
		e.Declared[:],
		e.Computed[:],
	)
}

// Sentinel error for script data hash mismatch so callers can use errors.Is
var ErrScriptDataHashMismatch = errors.New("script data hash mismatch")

func (ScriptDataHashMismatchError) Is(target error) bool {
	return target == ErrScriptDataHashMismatch
}

// MalformedReferenceScriptsError indicates reference scripts in outputs that cannot be deserialized
type MalformedReferenceScriptsError struct {
	ScriptHashes []ScriptHash
}

func (e MalformedReferenceScriptsError) Error() string {
	return fmt.Sprintf("malformed reference scripts: %v", e.ScriptHashes)
}

// Sentinel error for malformed reference scripts so callers can use errors.Is
var ErrMalformedReferenceScripts = errors.New("malformed reference scripts")

func (MalformedReferenceScriptsError) Is(target error) bool {
	return target == ErrMalformedReferenceScripts
}

// RefScriptSizePerTxTooLargeError indicates the total reference-script size in a
// transaction's outputs exceeds the per-transaction limit.
type RefScriptSizePerTxTooLargeError struct {
	TxSize  uint64
	MaxSize uint64
}

func (e RefScriptSizePerTxTooLargeError) Error() string {
	return fmt.Sprintf(
		"reference-script size per transaction too large: %d > %d",
		e.TxSize, e.MaxSize,
	)
}

var ErrRefScriptSizePerTxTooLarge = errors.New(
	"reference-script size per transaction too large",
)

func (RefScriptSizePerTxTooLargeError) Is(target error) bool {
	return target == ErrRefScriptSizePerTxTooLarge
}

// RefScriptSizePerBlockTooLargeError indicates the total reference-script size
// in a block's transaction outputs exceeds the per-block limit.
type RefScriptSizePerBlockTooLargeError struct {
	BlockSize uint64
	MaxSize   uint64
}

func (e RefScriptSizePerBlockTooLargeError) Error() string {
	return fmt.Sprintf(
		"reference-script size per block too large: %d > %d",
		e.BlockSize, e.MaxSize,
	)
}

var ErrRefScriptSizePerBlockTooLarge = errors.New(
	"reference-script size per block too large",
)

func (RefScriptSizePerBlockTooLargeError) Is(target error) bool {
	return target == ErrRefScriptSizePerBlockTooLarge
}

// ExtraneousRedeemerError indicates a redeemer whose tag/index does not
// correspond to a valid script purpose in the transaction: the index is out
// of range for its purpose category (spend/mint/cert/reward/voting/
// proposing), or its tag is not one this validation recognizes as a valid
// purpose (e.g. RedeemerTagGuarding, unless the caller has already
// special-cased it).
type ExtraneousRedeemerError struct {
	RedeemerKey RedeemerKey
}

func (e ExtraneousRedeemerError) Error() string {
	return fmt.Sprintf(
		"extraneous redeemer: tag=%d, index=%d doesn't match any valid script purpose",
		e.RedeemerKey.Tag,
		e.RedeemerKey.Index,
	)
}

// BlockExUnitsTooBigError indicates the sum of transaction execution units
// across an entire block exceeds the protocol's maximum block execution-unit
// budget (ppMaxBlockExUnits). This is a block-wide (BBODY) check in addition
// to each transaction's own per-transaction ExUnits limit.
type BlockExUnitsTooBigError struct {
	TotalExUnits    ExUnits
	MaxBlockExUnits ExUnits
}

func (e BlockExUnitsTooBigError) Error() string {
	return fmt.Sprintf(
		"block ExUnits too big: total %d/%d steps/memory, maximum %d/%d steps/memory",
		e.TotalExUnits.Steps,
		e.TotalExUnits.Memory,
		e.MaxBlockExUnits.Steps,
		e.MaxBlockExUnits.Memory,
	)
}

// Sentinel error for block ExUnits too big so callers can use errors.Is
var ErrBlockExUnitsTooBig = errors.New("block ExUnits too big")

func (BlockExUnitsTooBigError) Is(target error) bool {
	return target == ErrBlockExUnitsTooBig
}

// BlockBodySizeTooBigError indicates a block's serialized body size (the
// exact CBOR bytes of the block minus its header) exceeds the protocol's
// maximum block body size (ppMaxBlockBodySize).
type BlockBodySizeTooBigError struct {
	BlockBodySize    uint64
	MaxBlockBodySize uint64
}

func (e BlockBodySizeTooBigError) Error() string {
	return fmt.Sprintf(
		"block body size too big: size %d, max %d",
		e.BlockBodySize,
		e.MaxBlockBodySize,
	)
}

// Sentinel error for block body size too big so callers can use errors.Is
var ErrBlockBodySizeTooBig = errors.New("block body size too big")

func (BlockBodySizeTooBigError) Is(target error) bool {
	return target == ErrBlockBodySizeTooBig
}

// BlockHeaderSizeTooBigError indicates a block header's serialized size
// exceeds the protocol's maximum block header size (ppMaxBlockHeaderSize).
type BlockHeaderSizeTooBigError struct {
	HeaderSize         uint64
	MaxBlockHeaderSize uint64
}

func (e BlockHeaderSizeTooBigError) Error() string {
	return fmt.Sprintf(
		"block header size too big: size %d, max %d",
		e.HeaderSize,
		e.MaxBlockHeaderSize,
	)
}

// Sentinel error for block header size too big so callers can use errors.Is
var ErrBlockHeaderSizeTooBig = errors.New("block header size too big")

func (BlockHeaderSizeTooBigError) Is(target error) bool {
	return target == ErrBlockHeaderSizeTooBig
}
