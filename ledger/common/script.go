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

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	ScriptRefTypeNativeScript = 0
	ScriptRefTypePlutusV1     = 1
	ScriptRefTypePlutusV2     = 2
	ScriptRefTypePlutusV3     = 3
)

type Script interface {
	isScript()
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

type PlutusV1Script []byte

func (PlutusV1Script) isScript() {}

type PlutusV2Script []byte

func (PlutusV2Script) isScript() {}

type PlutusV3Script []byte

func (PlutusV3Script) isScript() {}

type NativeScript struct {
	item any
}

func (NativeScript) isScript() {}

func (n *NativeScript) Item() any {
	return n.item
}

func (n *NativeScript) UnmarshalCBOR(data []byte) error {
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
	return nil
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
