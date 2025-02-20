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
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

type NativeScript struct {
	item any
}

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
