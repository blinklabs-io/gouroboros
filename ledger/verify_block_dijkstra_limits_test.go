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
	"crypto/ed25519"
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func dijkstraBlockLimitWitnesses(
	exUnits common.ExUnits,
) dijkstra.DijkstraTransactionWitnessSet {
	return dijkstra.DijkstraTransactionWitnessSet{
		WsRedeemers: dijkstra.DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagGuarding, Index: 0}: {
					Data: common.Datum{
						Data: data.NewInteger(big.NewInt(0)),
					},
					ExUnits: exUnits,
				},
			},
		},
	}
}

func dijkstraBlockLimitExUnitsTx(
	topLevel common.ExUnits,
	subtransaction common.ExUnits,
) dijkstra.DijkstraTransaction {
	return dijkstra.DijkstraTransaction{
		Body: dijkstra.DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]dijkstra.DijkstraSubTransaction{{
					WitnessSet: dijkstraBlockLimitWitnesses(subtransaction),
				}},
				true,
			),
		},
		WitnessSet: dijkstraBlockLimitWitnesses(topLevel),
		TxIsValid:  true,
	}
}

// signedDijkstraLimitsHeader builds a 12-field Dijkstra header whose VRF proof,
// operational certificate and KES signature are valid for
// blockLimitsTestEta0Hex. The Conway fixture's own signatures cannot be reused:
// the current header body has two more fields, so its KES signature no longer
// covers the body. Only the slot and block number are taken from the fixture.
func signedDijkstraLimitsHeader(t *testing.T) *dijkstra.DijkstraBlockHeader {
	t.Helper()
	fixtureCbor, err := hex.DecodeString(blockLimitsTestHeaderHex)
	require.NoError(t, err)
	fixture, err := ledger.NewBlockHeaderFromCbor(
		ledger.BlockTypeConway,
		fixtureCbor,
	)
	require.NoError(t, err)
	slot := fixture.SlotNumber()

	coldPub, coldPriv, err := ed25519.GenerateKey(bytes.NewReader(
		bytes.Repeat([]byte{0x11}, 64),
	))
	require.NoError(t, err)
	kesSk, kesVkey, err := kes.KeyGen(6, bytes.Repeat([]byte{0x22}, 32))
	require.NoError(t, err)
	vrfVkey, vrfSk, err := vrf.KeyGen(bytes.Repeat([]byte{0x33}, 32))
	require.NoError(t, err)
	eta0, err := hex.DecodeString(blockLimitsTestEta0Hex)
	require.NoError(t, err)
	vrfMsg, err := vrf.MkInputVrf(int64(slot), eta0) // #nosec G115
	require.NoError(t, err)
	vrfProof, vrfOutput, err := vrf.Prove(vrfSk, vrfMsg)
	require.NoError(t, err)

	kesPeriod := slot / blockLimitsTestSlotsPerKesPeriod
	opCertSig := ed25519.Sign(
		coldPriv,
		common.OpCertSignableBytes(kesVkey, 0, kesPeriod),
	)
	header := &dijkstra.DijkstraBlockHeader{
		BabbageBlockHeader: babbage.BabbageBlockHeader{
			Body: babbage.BabbageBlockHeaderBody{
				BlockNumber: fixture.BlockNumber(),
				Slot:        slot,
				IssuerVkey:  common.IssuerVkey(coldPub),
				VrfKey:      vrfVkey,
				VrfResult: common.VrfResult{
					Output: vrfOutput,
					Proof:  vrfProof,
				},
				OpCert: babbage.BabbageOpCert{
					HotVkey:        kesVkey,
					SequenceNumber: 0,
					KesPeriod:      kesPeriod,
					Signature:      opCertSig,
				},
				ProtoVersion: babbage.BabbageProtoVersion{
					Major: dijkstra.MinProtocolVersionDijkstra,
				},
			},
			Signature: make([]byte, kes.CardanoKesSignatureSize),
		},
	}
	unsigned, err := header.MarshalCBOR()
	require.NoError(t, err)
	var top []cbor.RawMessage
	_, err = cbor.Decode(unsigned, &top)
	require.NoError(t, err)
	kesSig, err := kes.Sign(kesSk, 0, top[0])
	require.NoError(t, err)
	header.Signature = kesSig
	signed, err := header.MarshalCBOR()
	require.NoError(t, err)
	decoded := &dijkstra.DijkstraBlockHeader{}
	_, err = cbor.Decode(signed, decoded)
	require.NoError(t, err)
	return decoded
}

func buildDijkstraLimitsTestBlock(
	t *testing.T,
	txs []dijkstra.DijkstraTransaction,
) ledger.Block {
	t.Helper()
	dijkstraHeader := signedDijkstraLimitsHeader(t)

	crafted := &dijkstra.DijkstraBlock{
		BlockHeader: dijkstraHeader,
		BlockBody: dijkstra.DijkstraBlockBody{
			Transactions: txs,
		},
	}
	blockCbor, err := cbor.Encode(crafted)
	require.NoError(t, err)
	decoded, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeDijkstra,
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	return decoded
}

func TestVerifyBlockDijkstraExUnitsIncludesEveryTransactionLevel(
	t *testing.T,
) {
	block := buildDijkstraLimitsTestBlock(
		t,
		[]dijkstra.DijkstraTransaction{
			dijkstraBlockLimitExUnitsTx(
				common.ExUnits{Memory: 5, Steps: 7},
				common.ExUnits{Memory: 11, Steps: 13},
			),
			dijkstraBlockLimitExUnitsTx(
				common.ExUnits{Memory: 17, Steps: 19},
				common.ExUnits{Memory: 23, Steps: 29},
			),
		},
	)
	wantTotal := common.ExUnits{Memory: 56, Steps: 68}
	tests := []struct {
		name      string
		max       common.ExUnits
		wantError bool
	}{
		{name: "below limit", max: common.ExUnits{Memory: 57, Steps: 69}},
		{name: "at limit", max: wantTotal},
		{
			name:      "over limit",
			max:       common.ExUnits{Memory: 55, Steps: 67},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pp := &dijkstra.DijkstraProtocolParameters{
				ConwayProtocolParameters: conway.ConwayProtocolParameters{
					MaxBlockExUnits: test.max,
				},
			}
			valid, _, _, _, err := ledger.VerifyBlock(
				block,
				blockLimitsTestEta0Hex,
				blockLimitsTestSlotsPerKesPeriod,
				common.VerifyConfig{
					SkipBodyHashValidation:    true,
					SkipTransactionValidation: true,
					SkipStakePoolValidation:   true,
					ProtocolParameters:        pp,
				},
			)
			if !test.wantError {
				require.NoError(t, err)
				require.True(t, valid)
				return
			}
			require.False(t, valid)
			var target common.BlockExUnitsTooBigError
			require.ErrorAs(t, err, &target)
			require.Equal(t, wantTotal, target.TotalExUnits)
		})
	}
}
