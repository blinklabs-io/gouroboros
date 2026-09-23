// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package ed25519byron_test

import (
	"crypto/ed25519"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/ed25519byron"
)

func TestVerifyAcceptsSReducedModuloOrder(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte("Byron signature scalar reduction")
	signature := ed25519.Sign(privateKey, message)

	order, ok := new(big.Int).SetString(
		"7237005577332262213973186563042994240857116359379907606001950938285454250989",
		10,
	)
	if !ok {
		t.Fatal("parse Ed25519 group order")
	}
	scalar := littleEndianInt(signature[32:])
	scalar.Add(scalar, order)
	reduced := intLittleEndian(scalar, 32)
	if reduced[31]&0xe0 != 0 {
		t.Fatal("S plus the group order exceeded Byron's high-bit bound")
	}
	copy(signature[32:], reduced)
	if ed25519.Verify(publicKey, message, signature) {
		t.Fatal("standard Ed25519 verifier unexpectedly accepted S + L")
	}
	if !ed25519byron.Verify(publicKey, message, signature) {
		t.Fatal("Byron verifier rejected a valid signature with S + L")
	}
}

func TestVerifyAcceptsSmallOrderAndNonCanonicalPoints(t *testing.T) {
	// The identity point is small-order and Byron's verifier does not reject
	// small-order public or R points.
	identity := make([]byte, 32)
	identity[0] = 1
	signature := make([]byte, ed25519.SignatureSize)
	copy(signature[:32], identity)
	copy(signature[32:], make([]byte, 32))

	if !ed25519byron.Verify(identity, []byte("legacy"), signature) {
		t.Fatal("Byron verifier rejected the accepted small-order identity")
	}

	// y = p + 1 is a non-canonical encoding of the identity point. Byron's
	// point decoder reduces field elements instead of requiring canonical bytes.
	identityNonCanonical := make([]byte, 32)
	identityNonCanonical[0] = 0xee
	for i := 1; i < 31; i++ {
		identityNonCanonical[i] = 0xff
	}
	identityNonCanonical[31] = 0x7f
	copy(signature[:32], identityNonCanonical)
	if ed25519byron.Verify(identity, []byte("legacy"), signature) {
		t.Fatal("Byron verifier accepted a non-canonical R point encoding")
	}
	copy(signature[:32], identity)
	if !ed25519byron.Verify(identityNonCanonical, []byte("legacy"), signature) {
		t.Fatal("Byron verifier rejected a non-canonical public key encoding")
	}
}

func TestVerifyRejectsSAboveHighBitBound(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	signature := ed25519.Sign(privateKey, []byte("message"))
	signature[63] |= 0x20
	if ed25519byron.Verify(publicKey, []byte("message"), signature) {
		t.Fatal("accepted S with a high bit outside Byron's bound")
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
