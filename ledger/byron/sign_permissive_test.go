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

package byron

import (
	"crypto/ed25519"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// TestVerifyEd25519StaysPermissive is the guard against a well-meaning sweep.
//
// Byron's reference is Cardano.Crypto.Signing.Signature.verifySignatureRaw ->
// CC.verify in cardano-crypto-wallet, whose bundled ed25519-donna
// ed25519_sign_open checks only the high bits of S and performs no small-order
// test. It accepts the edwards25519 identity pair. Byron blocks are immutable,
// so tightening this boundary to match the non-Byron ones would reject chain
// the node accepts and break sync from genesis.
func TestVerifyEd25519StaysPermissive(t *testing.T) {
	// A Byron extended verification key is the 32-byte Ed25519 key followed by
	// a 32-byte chain code; only the first half is verified against.
	verificationKey := make([]byte, VerificationKeySize)
	verificationKey[0] = 0x01
	signature := append(
		append([]byte{0x01}, make([]byte, 31)...),
		make([]byte, 32)...,
	)
	if !verifyEd25519(
		verificationKey,
		[]byte("any Byron payload"),
		signature,
	) {
		t.Error(
			"Byron verification rejected the identity pair; ed25519-donna accepts it, " +
				"so this boundary must not use the strict criteria",
		)
	}
}

func TestByronMainnetRedeemWitnessValidatesWireTransactionHash(t *testing.T) {
	txCbor, err := hex.DecodeString(
		"82839f8200d8185824825820a12a839c25a01fa5d118167db5acdbd9e38172ae8f00e5ac0a4997ef792a200700ff9f8282d818584283581c6c9982e7f2b6dcc5eaa880e8014568913c8868d9f0f86eb687b2633ca101581e581c010d876783fb2b4d0d17c86df29af8d35356ed3d1827bf4744f06700001a8dc672c11a000f4240ffa0818202d81858658258208c0bdedfbbab26a1308300512ffb1b220f068ee13f7612afb076c22de3fb764158406cc41635a9794234966629ccfa2a5b089a20ae392f0e92154ff97eda30ff7a082a65fc4b362c24cf58c27f30103b1f1345e15479cf4b80cd4134c0f9dca83109",
	)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewByronTransactionFromCbor(txCbor)
	if err != nil {
		t.Fatal(err)
	}
	// The historical witness covers the original body bytes, while UTxO
	// identity uses the canonical transaction encoding.
	if tx.Body.Id() == tx.Body.WireId() {
		t.Fatal("real Byron transaction did not preserve the wire/ID distinction")
	}
	if err := tx.ValidateVKeyWitnesses(MainnetProtocolMagic); err != nil {
		t.Fatalf("real Mainnet transaction witness did not validate: %v", err)
	}
}

func TestByronMainnetExtendedVKeyWitnessUsesFirst32KeyBytes(t *testing.T) {
	txCbor, err := hex.DecodeString(
		"82839f8200d81858248258206497b33b10fa2619c6efbd9f874ecd1c91badb10bf70850732aab45b90524d9e00ff9f8282d818584283581c37f1f51e41efe8713f9755e78bb61af0bb822af6fb31788dba18e27ba101581e581c010d876783fb2b59f088db6d41359ae0a3868a0e411b4dde5713f870001a570841701a000b20128282d818584283581c5d4704fc22524e98ea5b9580ab2a29396b8ad2a92764d08ce23ea1e5a101581e581cd2c9d85d9e2ce454557363216e45b9f015e9b5c2617f0294ac5bc2d0001ae8c1444d1a000186a0ffa0818200d818588582584042a2100a4bce0f08ed211f980d7a848915fd48953be80b4b4fb3a9bbf8aea206cc8a84c83896f3d716fe0fc6ae8d5ae5554109c1fff5b6ca6c53cc74741dcad25840c26a80389d8bee813ed786d4cf395bbc304f43bef1b75eb5f989e915451cbe5610f8bf7dc843392070e4a470ebb7614da37f78c8a879da8eb0fc2f7f8ffd0107",
	)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := NewByronTransactionFromCbor(txCbor)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.ValidateVKeyWitnesses(MainnetProtocolMagic); err != nil {
		t.Fatalf("real Mainnet extended-key witness did not validate: %v", err)
	}
}

func TestVerifyEd25519AcceptsHonestSignature(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	verificationKey := make([]byte, VerificationKeySize)
	copy(verificationKey, pub)
	msg := []byte("a Byron payload this key actually signed")
	sig := ed25519.Sign(priv, msg)
	if !verifyEd25519(verificationKey, msg, sig) {
		t.Error("rejected an honest Byron signature")
	}
	if verifyEd25519(verificationKey, []byte("a different payload"), sig) {
		t.Error("accepted a Byron signature over a different payload")
	}
}

func TestVerifyEd25519ReducesSModuloOrder(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("Byron signature scalar reduction")
	sig := ed25519.Sign(priv, msg)
	order, ok := new(big.Int).SetString(
		"7237005577332262213973186563042994240857116359379907606001950938285454250989",
		10,
	)
	if !ok {
		t.Fatal("parse Ed25519 group order")
	}
	scalar := littleEndianInt(sig[32:])
	scalar.Add(scalar, order)
	copy(sig[32:], intLittleEndian(scalar, 32))
	if !verifyEd25519(append(pub, make([]byte, 32)...), msg, sig) {
		t.Fatal("Byron verifier rejected a valid signature with S + L")
	}
}

func TestByronTransactionVKeyWitnessUsesLegacyVerifier(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	tx := &ByronTransaction{
		Body: ByronTransactionBody{
			TxInputs:   []ByronTransactionInput{},
			TxOutputs:  []ByronTransactionOutput{},
			Attributes: cbor.RawMessage{0xa0},
		},
	}
	const protocolMagic = uint32(42)
	magic, err := cbor.Encode(protocolMagic)
	if err != nil {
		t.Fatal(err)
	}
	txHash := tx.Body.WireId()
	txPayload, err := cbor.Encode(txHash[:])
	if err != nil {
		t.Fatal(err)
	}
	message := append([]byte{0x01}, magic...)
	message = append(message, txPayload...)
	sig := ed25519.Sign(priv, message)
	order, ok := new(big.Int).SetString(
		"7237005577332262213973186563042994240857116359379907606001950938285454250989",
		10,
	)
	if !ok {
		t.Fatal("parse Ed25519 group order")
	}
	scalar := littleEndianInt(sig[32:])
	scalar.Add(scalar, order)
	copy(sig[32:], intLittleEndian(scalar, 32))
	extendedKey := append(append([]byte(nil), pub...), make([]byte, 32)...)
	inner, err := cbor.Encode([]any{extendedKey, sig})
	if err != nil {
		t.Fatal(err)
	}
	witness, err := cbor.Encode([]any{uint64(0), cbor.WrappedCbor(inner)})
	if err != nil {
		t.Fatal(err)
	}
	var value cbor.Value
	if err := value.UnmarshalCBOR(witness); err != nil {
		t.Fatal(err)
	}
	tx.Twit = []cbor.Value{value}
	if err := tx.ValidateVKeyWitnesses(protocolMagic); err != nil {
		t.Fatalf("Byron transaction rejected S + L witness: %v", err)
	}
}

func TestByronTransactionVKeyWitnessUsesDomainSeparatedSigningData(t *testing.T) {
	for _, tc := range []struct {
		name        string
		constructor uint64
		publicKey   []byte
		tag         byte
	}{
		{
			name:        "verification key",
			constructor: 0,
			publicKey:   make([]byte, 64),
			tag:         0x01,
		},
		{
			name:        "redeem key",
			constructor: 2,
			publicKey:   make([]byte, ed25519.PublicKeySize),
			tag:         0x02,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			publicKey, privateKey, err := ed25519.GenerateKey(nil)
			if err != nil {
				t.Fatal(err)
			}
			copy(tc.publicKey, publicKey)
			const protocolMagic = uint32(764824073)
			tx := &ByronTransaction{Body: ByronTransactionBody{
				TxInputs:   []ByronTransactionInput{},
				TxOutputs:  []ByronTransactionOutput{},
				Attributes: cbor.RawMessage{0xa0},
			}}
			magic, err := cbor.Encode(protocolMagic)
			if err != nil {
				t.Fatal(err)
			}
			txId := tx.Body.WireId()
			txPayload, err := cbor.Encode(txId[:])
			if err != nil {
				t.Fatal(err)
			}
			message := append([]byte{tc.tag}, magic...)
			message = append(message, txPayload...)
			signature := ed25519.Sign(privateKey, message)
			inner, err := cbor.Encode([]any{tc.publicKey, signature})
			if err != nil {
				t.Fatal(err)
			}
			encodedWitness, err := cbor.Encode([]any{
				tc.constructor, cbor.WrappedCbor(inner),
			})
			if err != nil {
				t.Fatal(err)
			}
			var witness cbor.Value
			if err := witness.UnmarshalCBOR(encodedWitness); err != nil {
				t.Fatal(err)
			}
			tx.Twit = []cbor.Value{witness}
			if err := tx.ValidateVKeyWitnesses(protocolMagic); err != nil {
				t.Fatalf("valid Byron witness rejected: %v", err)
			}
			if err := tx.ValidateVKeyWitnesses(protocolMagic + 1); err == nil {
				t.Fatal("accepted witness signed for a different protocol magic")
			}
		})
	}
}

func littleEndianInt(encoded []byte) *big.Int {
	be := make([]byte, len(encoded))
	for i := range encoded {
		be[len(encoded)-1-i] = encoded[i]
	}
	return new(big.Int).SetBytes(be)
}

func intLittleEndian(value *big.Int, length int) []byte {
	be := value.Bytes()
	out := make([]byte, length)
	for i := 0; i < len(be) && i < length; i++ {
		out[i] = be[len(be)-1-i]
	}
	return out
}
