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

package common

import (
	"errors"
	"fmt"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/syn"
)

const (
	ScriptRefTypeNativeScript = 0
	ScriptRefTypePlutusV1     = 1
	ScriptRefTypePlutusV2     = 2
	ScriptRefTypePlutusV3     = 3
)

type ScriptHash = Blake2b224

type Script interface {
	isScript()
	Hash() ScriptHash
	RawScriptBytes() []byte
}

type ScriptRef struct {
	Type   uint
	Script Script
}

func (s *ScriptRef) UnmarshalCBOR(data []byte) error {
	// Unwrap outer CBOR tag
	var tmpTag cbor.Tag
	if _, err := cbor.Decode(data, &tmpTag); err != nil {
		return err
	}
	innerCbor, ok := tmpTag.Content.([]byte)
	if !ok {
		return errors.New("unexpected tag type")
	}
	// Determine script type
	var rawScript struct {
		cbor.StructAsArray
		Type uint
		Raw  cbor.RawMessage
	}
	if _, err := cbor.Decode(innerCbor, &rawScript); err != nil {
		return err
	}
	var tmpScript Script
	switch rawScript.Type {
	case ScriptRefTypeNativeScript:
		tmpScript = &NativeScript{}
	case ScriptRefTypePlutusV1:
		tmpScript = &PlutusV1Script{}
	case ScriptRefTypePlutusV2:
		tmpScript = &PlutusV2Script{}
	case ScriptRefTypePlutusV3:
		tmpScript = &PlutusV3Script{}
	default:
		return fmt.Errorf("unknown script type %d", rawScript.Type)
	}
	// Decode script
	if _, err := cbor.Decode(rawScript.Raw, tmpScript); err != nil {
		return err
	}
	s.Type = rawScript.Type
	s.Script = tmpScript
	return nil
}

func (s *ScriptRef) MarshalCBOR() ([]byte, error) {
	tmpData := []any{
		s.Type,
		s.Script,
	}
	tmpDataCbor, err := cbor.Encode(tmpData)
	if err != nil {
		return nil, err
	}
	tmpTag := cbor.Tag{
		Number:  24,
		Content: tmpDataCbor,
	}
	return cbor.Encode(tmpTag)
}

type PlutusV1Script []byte

func (PlutusV1Script) isScript() {}

func (s PlutusV1Script) Hash() ScriptHash {
	return Blake2b224Hash(
		slices.Concat(
			[]byte{ScriptRefTypePlutusV1},
			[]byte(s),
		),
	)
}

func (s PlutusV1Script) RawScriptBytes() []byte {
	return []byte(s)
}

type PlutusV2Script []byte

func (PlutusV2Script) isScript() {}

func (s PlutusV2Script) Hash() ScriptHash {
	return Blake2b224Hash(
		slices.Concat(
			[]byte{ScriptRefTypePlutusV2},
			[]byte(s),
		),
	)
}

func (s PlutusV2Script) RawScriptBytes() []byte {
	return []byte(s)
}

type PlutusV3Script []byte

func (PlutusV3Script) isScript() {}

func (s PlutusV3Script) Hash() ScriptHash {
	return Blake2b224Hash(
		slices.Concat(
			[]byte{ScriptRefTypePlutusV3},
			[]byte(s),
		),
	)
}

func (s PlutusV3Script) RawScriptBytes() []byte {
	return []byte(s)
}

func (s PlutusV3Script) Evaluate(
	scriptContext data.PlutusData,
	budget ExUnits,
) (ExUnits, error) {
	var usedExUnits ExUnits
	var err error
	program := &syn.Program[syn.DeBruijn]{}
	// Set budget
	machineBudget := cek.DefaultExBudget
	if budget.Steps > 0 || budget.Memory > 0 {
		machineBudget = cek.ExBudget{
			Cpu: budget.Steps,
			Mem: budget.Memory,
		}
	}
	// Decode raw script as bytestring to get actual script bytes
	var innerScript []byte
	if _, err = cbor.Decode([]byte(s), &innerScript); err != nil {
		return usedExUnits, err
	}
	// Decode program
	program, err = syn.Decode[syn.DeBruijn]([]byte(innerScript))
	if err != nil {
		return usedExUnits, fmt.Errorf("decode script: %w", err)
	}
	// Apply script context to program
	contextTerm := &syn.Constant{
		Con: &syn.Data{
			Inner: scriptContext,
		},
	}
	wrappedProgram := &syn.Apply[syn.DeBruijn]{
		Function: program.Term,
		Argument: contextTerm,
	}
	// Execute wrapped program
	machine := cek.NewMachine[syn.DeBruijn](200)
	machine.ExBudget = machineBudget
	_, err = machine.Run(wrappedProgram)
	if err != nil {
		return usedExUnits, fmt.Errorf("execute script: %w", err)
	}
	consumedBudget := machineBudget.Sub(&machine.ExBudget)
	usedExUnits.Memory = consumedBudget.Mem
	usedExUnits.Steps = consumedBudget.Cpu
	return usedExUnits, nil
}

type NativeScript struct {
	cbor.DecodeStoreCbor
	item any
}

func (NativeScript) isScript() {}

func (n *NativeScript) Item() any {
	return n.item
}

func (n *NativeScript) UnmarshalCBOR(data []byte) error {
	n.SetCbor(data)
	id, err := cbor.DecodeIdFromList(data)
	if err != nil {
		return err
	}
	var tmpData any
	switch id {
	case 0:
		tmpData = &NativeScriptPubkey{}
	case 1:
		tmpData = &NativeScriptAll{}
	case 2:
		tmpData = &NativeScriptAny{}
	case 3:
		tmpData = &NativeScriptNofK{}
	case 4:
		tmpData = &NativeScriptInvalidBefore{}
	case 5:
		tmpData = &NativeScriptInvalidHereafter{}
	default:
		return fmt.Errorf("unknown native script type %d", id)
	}
	if _, err := cbor.Decode(data, tmpData); err != nil {
		return err
	}
	n.item = tmpData
	return nil
}

func (s NativeScript) Hash() ScriptHash {
	return Blake2b224Hash(
		slices.Concat(
			[]byte{ScriptRefTypeNativeScript},
			[]byte(s.Cbor()),
		),
	)
}

func (s NativeScript) RawScriptBytes() []byte {
	return s.Cbor()
}

type NativeScriptPubkey struct {
	cbor.StructAsArray
	Type uint
	Hash []byte
}

type NativeScriptAll struct {
	cbor.StructAsArray
	Type    uint
	Scripts []NativeScript
}

type NativeScriptAny struct {
	cbor.StructAsArray
	Type    uint
	Scripts []NativeScript
}

type NativeScriptNofK struct {
	cbor.StructAsArray
	Type    uint
	N       uint
	Scripts []NativeScript
}

type NativeScriptInvalidBefore struct {
	cbor.StructAsArray
	Type uint
	Slot uint64
}

type NativeScriptInvalidHereafter struct {
	cbor.StructAsArray
	Type uint
	Slot uint64
}
