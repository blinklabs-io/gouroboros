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

package ledger_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

// decodeOpCertTestHeader returns the shared real mainnet Conway header
// (block 10882991) whose VRF and KES both verify against
// blockLimitsTestEta0Hex.
func decodeOpCertTestHeader(t *testing.T) *conway.ConwayBlockHeader {
	t.Helper()
	headerCborBytes, err := hex.DecodeString(blockLimitsTestHeaderHex)
	require.NoError(t, err)
	header, err := ledger.NewBlockHeaderFromCbor(
		ledger.BlockTypeConway,
		headerCborBytes,
	)
	require.NoError(t, err)
	conwayHeader, ok := header.(*conway.ConwayBlockHeader)
	require.True(t, ok)
	return conwayHeader
}

// opCertTestBlock wraps a header in a transaction-free Conway block and
// round-trips it through the public decoder so header.Cbor() and the stored
// header-body CBOR come from real serialized bytes.
func opCertTestBlock(
	t *testing.T,
	header *conway.ConwayBlockHeader,
) ledger.Block {
	t.Helper()
	crafted := &conway.ConwayBlock{
		BlockHeader:            header,
		TransactionBodies:      []conway.ConwayTransactionBody{},
		TransactionWitnessSets: []conway.ConwayTransactionWitnessSet{},
		TransactionMetadataSet: common.TransactionMetadataSet{},
		InvalidTransactions:    []uint{},
	}
	blockCbor, err := cbor.Encode(crafted)
	require.NoError(t, err)
	decoded, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeConway,
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	return decoded
}

func opCertTestVerifyConfig() common.VerifyConfig {
	return common.VerifyConfig{
		SkipBodyHashValidation:    true,
		SkipTransactionValidation: true,
		SkipStakePoolValidation:   true,
	}
}

// TestVerifyBlock_RealMainnetOpCertColdSignatureVerifies pins the operational
// certificate signable representation against real mainnet bytes: the cold
// signature in block 10882991's header must verify against that header's own
// issuer vkey. A signable encoding that differs from cardano-ledger's
// OCertSignable (KES vkey || counter BE64 || KES period BE64) would reject
// every real block, so this is the control for the rejection test below.
func TestVerifyBlock_RealMainnetOpCertColdSignatureVerifies(t *testing.T) {
	t.Parallel()

	header := decodeOpCertTestHeader(t)
	opCert := &ledger.OpCert{
		KesVkey:       header.Body.OpCert.HotVkey,
		IssueNumber:   header.Body.OpCert.SequenceNumber,
		KesPeriod:     header.Body.OpCert.KesPeriod,
		ColdSignature: header.Body.OpCert.Signature,
	}
	require.NoError(
		t,
		ledger.VerifyOpCertSignature(opCert, header.Body.IssuerVkey[:]),
	)

	valid, _, _, _, err := ledger.VerifyBlock(
		opCertTestBlock(t, header),
		blockLimitsTestEta0Hex,
		blockLimitsTestSlotsPerKesPeriod,
		opCertTestVerifyConfig(),
	)
	require.NoError(t, err)
	require.True(t, valid)
}

// TestVerifyBlock_RejectsSubstitutedOpCertHotKey drives the forgery the
// block-validation path must refuse: an attacker republishes a real pool's
// header under that pool's issuer vkey but swaps in a KES hot key they
// control and re-signs the header body with it.
//
// Every other check in VerifyBlock still passes. The KES signature verifies,
// because it is checked against the hot vkey carried in the same header the
// attacker wrote. The VRF proof verifies, because it is checked against the
// VrfKey carried in that header and VerifyBlock does not bind that key to a
// pool registration. Only the operational certificate's cold signature, which
// covers the real hot vkey, distinguishes the forgery -- so the assertion
// here is specifically that the block is refused, and the control above
// proves the same code accepts the untampered header.
func TestVerifyBlock_RejectsSubstitutedOpCertHotKey(t *testing.T) {
	t.Parallel()

	header := decodeOpCertTestHeader(t)
	originalHotVkey := bytes.Clone(header.Body.OpCert.HotVkey)

	seed := bytes.Repeat([]byte{0x2a}, 32)
	attackerSKey, attackerVKey, err := kes.KeyGen(kes.CardanoKesDepth, seed)
	require.NoError(t, err)
	require.NotEqual(t, originalHotVkey, attackerVKey)

	header.Body.OpCert.HotVkey = attackerVKey

	// The header encodes as [body, signature]; the body bytes the KES
	// signature covers do not depend on that signature, so encode once with a
	// placeholder to learn the body bytes, sign them, then encode again.
	header.Signature = make([]byte, kes.CardanoKesSignatureSize)
	bodyCbor := reencodedHeaderBodyCbor(t, header)

	// VerifyKesComponents checks the signature at the evolution implied by
	// the header's own slot and declared KES period, so the attacker key must
	// be evolved to exactly that index before signing.
	evolution := header.Body.Slot/blockLimitsTestSlotsPerKesPeriod -
		header.Body.OpCert.KesPeriod
	for range evolution {
		attackerSKey, err = kes.Update(attackerSKey)
		require.NoError(t, err)
	}
	forgedSig, err := kes.Sign(attackerSKey, evolution, bodyCbor)
	require.NoError(t, err)
	header.Signature = forgedSig

	forgedHeader := reencodeHeader(t, header)
	require.Equal(t, bodyCbor, forgedHeader.Body.Cbor(),
		"header body bytes must not change when only the KES signature does",
	)
	// Control: the forgery clears the KES check, so a rejection below cannot
	// be the KES guard firing early.
	kesValid, err := ledger.VerifyKes(
		forgedHeader,
		blockLimitsTestSlotsPerKesPeriod,
	)
	require.NoError(t, err)
	require.True(t, kesValid, "forged header must pass KES verification")

	valid, _, _, _, err := ledger.VerifyBlock(
		opCertTestBlock(t, forgedHeader),
		blockLimitsTestEta0Hex,
		blockLimitsTestSlotsPerKesPeriod,
		opCertTestVerifyConfig(),
	)
	require.Error(
		t,
		err,
		"VerifyBlock accepted a header whose operational certificate cold signature does not cover its KES hot key",
	)
	require.False(t, valid)
	var validationErr *common.ValidationError
	require.ErrorAs(t, err, &validationErr)
	require.Equal(t, common.ValidationErrorTypeOpCert, validationErr.Type)
}

// reencodedHeaderBodyCbor encodes and re-decodes header, returning the stored
// original body CBOR of the result -- the exact bytes VerifyKes signs over.
func reencodedHeaderBodyCbor(
	t *testing.T,
	header *conway.ConwayBlockHeader,
) []byte {
	t.Helper()
	return bytes.Clone(reencodeHeader(t, header).Body.Cbor())
}

func reencodeHeader(
	t *testing.T,
	header *conway.ConwayBlockHeader,
) *conway.ConwayBlockHeader {
	t.Helper()
	headerCbor, err := cbor.Encode(header)
	require.NoError(t, err)
	decoded, err := ledger.NewBlockHeaderFromCbor(
		ledger.BlockTypeConway,
		headerCbor,
	)
	require.NoError(t, err)
	conwayHeader, ok := decoded.(*conway.ConwayBlockHeader)
	require.True(t, ok)
	return conwayHeader
}
