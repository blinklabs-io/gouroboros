// Copyright 2024 Blink Labs Software
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

	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
	"golang.org/x/crypto/blake2b"
)

const (
	StakeCredentialTypeAddrKeyHash = 0
	StakeCredentialTypeScriptHash  = 1
)

type StakeCredential struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	CredType   uint
	Credential []byte
}

func (c *StakeCredential) Hash() Blake2b224 {
	hash, err := blake2b.New(28, nil)
	if err != nil {
		panic(
			fmt.Sprintf(
				"unexpected error creating empty blake2b hash: %s",
				err,
			),
		)
	}
	if c != nil {
		hash.Write(c.Credential[:])
	}
	return Blake2b224(hash.Sum(nil))
}

func (c *StakeCredential) Utxorpc() *utxorpc.StakeCredential {
	ret := &utxorpc.StakeCredential{}
	switch c.CredType {
	case StakeCredentialTypeAddrKeyHash:
		ret.StakeCredential = &utxorpc.StakeCredential_AddrKeyHash{
			AddrKeyHash: c.Credential[:],
		}
	case StakeCredentialTypeScriptHash:
		ret.StakeCredential = &utxorpc.StakeCredential_ScriptHash{
			ScriptHash: c.Credential[:],
		}
	}
	return ret
}
