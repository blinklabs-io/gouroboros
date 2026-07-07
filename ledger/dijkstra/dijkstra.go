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

package dijkstra

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"iter"
	"maps"
	"math/big"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

const (
	EraIdDijkstra   = 7
	EraNameDijkstra = "Dijkstra"

	// MinProtocolVersionDijkstra is the lowest protocol major version that
	// belongs to the Dijkstra era.
	MinProtocolVersionDijkstra = 12
	// MaxProtocolVersionDijkstra is the highest protocol major version that
	// belongs to the Dijkstra era.
	MaxProtocolVersionDijkstra = 13

	BlockTypeDijkstra = 8

	BlockHeaderTypeDijkstra = 7

	TxTypeDijkstra = 7

	// MaxTxSize is the decode-time Dijkstra transaction CBOR limit. It
	// mirrors the current Cardano max_tx_size until protocol parameters are
	// available for validation.
	MaxTxSize = 16 * 1024
)

var EraDijkstra = common.Era{
	Id:   EraIdDijkstra,
	Name: EraNameDijkstra,
}

func init() {
	common.RegisterEra(EraDijkstra)
}

type DijkstraBlock struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	BlockHeader *DijkstraBlockHeader
	BlockBody   DijkstraBlockBody
}

func (b *DijkstraBlock) UnmarshalCBOR(cborData []byte) error {
	// A Dijkstra (prototype-2026w27) block is a 2-element array:
	//   block = [header, block_body]
	// This differs from the pre-Dijkstra segwit layout that spread the body's
	// components (tx bodies, witnesses, metadata, invalid txs) as additional
	// top-level array elements.
	var items []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &items); err != nil {
		return err
	}
	if len(items) != 2 {
		return fmt.Errorf(
			"invalid Dijkstra block: expected 2 components, got %d",
			len(items),
		)
	}
	var header DijkstraBlockHeader
	if _, err := cbor.Decode(items[0], &header); err != nil {
		return fmt.Errorf("decode Dijkstra block header: %w", err)
	}
	var body DijkstraBlockBody
	if _, err := cbor.Decode(items[1], &body); err != nil {
		return fmt.Errorf("decode Dijkstra block body: %w", err)
	}
	b.BlockHeader = &header
	b.BlockBody = body
	b.SetCbor(cborData)
	return nil
}

func (b *DijkstraBlock) MarshalCBOR() ([]byte, error) {
	if b.Cbor() != nil {
		return b.Cbor(), nil
	}
	return cbor.Encode([]any{b.BlockHeader, b.BlockBody})
}

func (DijkstraBlock) Type() int {
	return BlockTypeDijkstra
}

func (b *DijkstraBlock) Hash() common.Blake2b256 {
	return b.BlockHeader.Hash()
}

func (b *DijkstraBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *DijkstraBlock) PrevHash() common.Blake2b256 {
	return b.BlockHeader.PrevHash()
}

func (b *DijkstraBlock) BlockNumber() uint64 {
	return b.BlockHeader.BlockNumber()
}

func (b *DijkstraBlock) SlotNumber() uint64 {
	return b.BlockHeader.SlotNumber()
}

func (b *DijkstraBlock) IssuerVkey() common.IssuerVkey {
	return b.BlockHeader.IssuerVkey()
}

func (b *DijkstraBlock) BlockBodySize() uint64 {
	return b.BlockHeader.BlockBodySize()
}

func (b *DijkstraBlock) Era() common.Era {
	return EraDijkstra
}

func (b *DijkstraBlock) Transactions() []common.Transaction {
	// Each transaction's IsValid() already reflects membership in the block
	// body's invalid_transactions index set (applied at decode time); a tx at
	// index i is invalid iff i is in that set.
	ret := make([]common.Transaction, len(b.BlockBody.Transactions))
	for idx := range b.BlockBody.Transactions {
		ret[idx] = &b.BlockBody.Transactions[idx]
	}
	return ret
}

func (b *DijkstraBlock) Utxorpc() (*utxorpc.Block, error) {
	tmpTxs := b.Transactions()
	txs := make([]*utxorpc.Tx, 0, len(tmpTxs))
	for _, t := range tmpTxs {
		tx, err := t.Utxorpc()
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	body := &utxorpc.BlockBody{
		Tx: txs,
	}
	header := &utxorpc.BlockHeader{
		Hash:   b.Hash().Bytes(),
		Height: b.BlockNumber(),
		Slot:   b.SlotNumber(),
	}
	return &utxorpc.Block{
		Body:   body,
		Header: header,
	}, nil
}

func (b *DijkstraBlock) BlockBodyHash() common.Blake2b256 {
	return b.Header().BlockBodyHash()
}

func (b *DijkstraBlock) CalculatedBlockBodyHash() common.Blake2b256 {
	return b.BlockBody.Hash()
}

// DijkstraLeiosCertificate is the in-body Leios certificate added in the
// ouroboros-leios prototype-2026w27 release (IntersectMBO/cardano-ledger #5872):
//
//	block_body = [..., leios_certificate : leios_certificate / nil, peras_certificate : peras_certificate / nil]
//	leios_certificate = [ signers : bytes ; committee signer bitfield
//	                    , aggregated_signature : leios_signature ]
//	leios_signature = bytes .size 48
//
// It certifies the endorser block announced by an earlier ranking block; the
// ranking block whose header sets leios_certified carries it in its body. Per
// CIP-0164 a block that carries a Leios certificate carries no Dijkstra-era
// transactions.
type DijkstraLeiosCertificate struct {
	cbor.DecodeStoreCbor
	Signers             []byte
	AggregatedSignature []byte
}

func (c *DijkstraLeiosCertificate) UnmarshalCBOR(cborData []byte) error {
	var items []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &items); err != nil {
		return fmt.Errorf("decode Dijkstra Leios certificate: %w", err)
	}
	if len(items) != 2 {
		return fmt.Errorf(
			"dijkstra Leios certificate must have 2 fields, got %d",
			len(items),
		)
	}
	var signers []byte
	if _, err := cbor.Decode(items[0], &signers); err != nil {
		return fmt.Errorf(
			"decode Dijkstra Leios certificate signers: %w",
			err,
		)
	}
	var aggSig []byte
	if _, err := cbor.Decode(items[1], &aggSig); err != nil {
		return fmt.Errorf(
			"decode Dijkstra Leios certificate aggregated signature: %w",
			err,
		)
	}
	if len(aggSig) != common.LeiosBlsSignatureSize {
		return fmt.Errorf(
			"invalid Dijkstra Leios certificate aggregated signature length: expected %d, got %d",
			common.LeiosBlsSignatureSize,
			len(aggSig),
		)
	}
	c.Signers = signers
	c.AggregatedSignature = aggSig
	c.SetCbor(cborData)
	return nil
}

func (c DijkstraLeiosCertificate) MarshalCBOR() ([]byte, error) {
	if raw := c.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	return cbor.Encode([]any{c.Signers, c.AggregatedSignature})
}

// DijkstraBlockBody is the Dijkstra (prototype-2026w27) block body. Per the
// authoritative cardano-ledger Dijkstra CDDL it is a 4-element array:
//
//	block_body =
//	  [ invalid_transactions : invalid_transactions / nil
//	  , transactions         : [* transaction]
//	  , leios_certificate     : leios_certificate / nil
//	  , peras_certificate     : peras_certificate / nil ]
//
// Unlike pre-Dijkstra eras, transactions are stored inline (each a full
// [transaction_body, transaction_witness_set, auxiliary_data/nil] array) rather
// than as parallel tx-body / witness-set / metadata segments, and per-tx
// validity is conveyed by the invalid_transactions index set rather than a
// per-transaction is_valid flag.
type DijkstraBlockBody struct {
	cbor.DecodeStoreCbor
	InvalidTransactions []uint
	Transactions        []DijkstraTransaction
	LeiosCertificate    *DijkstraLeiosCertificate
	// PerasCertificate is the block_body peras_certificate field, which the
	// Dijkstra CDDL defines as an opaque byte string (peras_certificate =
	// bytes; peras_certificate / nil). It is nil when the field is CBOR null.
	PerasCertificate []byte
}

func (b *DijkstraBlockBody) UnmarshalCBOR(cborData []byte) error {
	var items []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &items); err != nil {
		return err
	}
	if len(items) != 4 {
		return fmt.Errorf(
			"invalid Dijkstra block body: expected 4 components, got %d",
			len(items),
		)
	}
	// items[0]: invalid_transactions / nil
	invalidTxs, err := decodeInvalidTransactions(items[0])
	if err != nil {
		return err
	}
	// items[1]: transactions [* transaction]
	var txs []DijkstraTransaction
	if _, err := cbor.Decode(items[1], &txs); err != nil {
		return fmt.Errorf("decode Dijkstra transactions: %w", err)
	}
	// items[2]: leios_certificate / nil
	leiosCert, err := decodeDijkstraLeiosCertificate(items[2])
	if err != nil {
		return err
	}
	// items[3]: peras_certificate / nil
	perasCert, err := decodeDijkstraPerasCertificate(items[3])
	if err != nil {
		return err
	}
	// A transaction at index i is invalid iff i is in invalid_transactions;
	// every index must reference an existing transaction.
	invalidTxMap := make(map[uint]bool, len(invalidTxs))
	for _, invalidTxIdx := range invalidTxs {
		if invalidTxIdx >= uint(len(txs)) {
			return fmt.Errorf(
				"invalid transaction index %d outside transaction list length %d",
				invalidTxIdx,
				len(txs),
			)
		}
		invalidTxMap[invalidTxIdx] = true
	}
	for idx := range txs {
		txs[idx].TxIsValid = !invalidTxMap[uint(idx)]
	}
	b.InvalidTransactions = invalidTxs
	b.Transactions = txs
	b.LeiosCertificate = leiosCert
	b.PerasCertificate = perasCert
	b.SetCbor(cborData)
	return nil
}

func (b DijkstraBlockBody) MarshalCBOR() ([]byte, error) {
	if b.Cbor() != nil {
		return b.Cbor(), nil
	}
	// invalid_transactions is a nonempty_set / nil: an empty set encodes as
	// CBOR null, matching the reference node's on-wire encoding.
	var invalidField any
	if invalidTxs := b.invalidTransactionsForEncoding(); len(invalidTxs) > 0 {
		invalidField = invalidTxs
	}
	txs := b.Transactions
	if txs == nil {
		txs = []DijkstraTransaction{}
	}
	var leiosField any
	if b.LeiosCertificate != nil {
		leiosField = b.LeiosCertificate
	}
	var perasField any
	if b.PerasCertificate != nil {
		perasField = b.PerasCertificate
	}
	return cbor.Encode([]any{invalidField, txs, leiosField, perasField})
}

// Hash computes the Dijkstra block body hash. The prototype-2026w27 node stores
// blake2b256 over the block_body CBOR itself (verified against live blocks on
// network magic 164), replacing the pre-Dijkstra segwit hash-of-concatenated-
// segment-hashes.
func (b DijkstraBlockBody) Hash() common.Blake2b256 {
	cborData, err := b.MarshalCBOR()
	if err != nil {
		panic("CBOR encoding that should never fail has failed: " + err.Error())
	}
	return common.Blake2b256Hash(cborData)
}

// invalidTransactionsForEncoding derives the sorted invalid_transactions index
// set from the block body's transactions and any explicitly set indices. A
// transaction is invalid when its IsValid() is false. The result feeds the
// nonempty_set / nil field: an empty result is encoded as CBOR null.
func (b DijkstraBlockBody) invalidTransactionsForEncoding() []uint {
	invalidTxMap := make(map[uint]bool, len(b.InvalidTransactions))
	for _, invalidTxIdx := range b.InvalidTransactions {
		invalidTxMap[invalidTxIdx] = true
	}
	for idx, tx := range b.Transactions {
		if !tx.IsValid() {
			invalidTxMap[uint(idx)] = true
		}
	}
	if len(invalidTxMap) == 0 {
		return nil
	}
	ret := make([]uint, 0, len(invalidTxMap))
	for idx := range invalidTxMap {
		ret = append(ret, idx)
	}
	slices.Sort(ret)
	return ret
}

func decodeDijkstraLeiosCertificate(
	raw cbor.RawMessage,
) (*DijkstraLeiosCertificate, error) {
	if isCborNull(raw) {
		return nil, nil
	}
	var cert DijkstraLeiosCertificate
	if _, err := cbor.Decode(raw, &cert); err != nil {
		return nil, fmt.Errorf("decode Dijkstra Leios certificate: %w", err)
	}
	return &cert, nil
}

// decodeDijkstraPerasCertificate decodes the block_body peras_certificate
// field. The Dijkstra CDDL defines it as an opaque byte string
// (peras_certificate = bytes; peras_certificate / nil), so a null field
// returns nil and any present value is returned as its raw bytes.
func decodeDijkstraPerasCertificate(
	raw cbor.RawMessage,
) ([]byte, error) {
	if isCborNull(raw) {
		return nil, nil
	}
	var cert []byte
	if _, err := cbor.Decode(raw, &cert); err != nil {
		return nil, fmt.Errorf("decode Dijkstra Peras certificate: %w", err)
	}
	if cert == nil {
		// Present-but-empty (0x40) stays non-nil so it round-trips as bytes
		// rather than collapsing to the null field.
		cert = []byte{}
	}
	return cert, nil
}

// babbageHeaderBodyFieldCount is the number of fields in a Babbage block
// header body (block_number, slot, prev_hash, issuer_vkey, vrf_vkey,
// vrf_result, block_body_size, block_body_hash, operational_cert,
// protocol_version).
const babbageHeaderBodyFieldCount = 10

type DijkstraBlockHeader struct {
	babbage.BabbageBlockHeader
	// LeiosHeaderExtension holds the Dijkstra/Leios block-header fields that
	// follow Babbage's protocol_version field. As of the ouroboros-leios
	// prototype-2026w27 release (IntersectMBO/cardano-ledger #5889) these are
	// two fields: leios_certified (bool) and leios_announcement
	// ([announced_eb : hash32, announced_eb_size : uint .size 4] / nil). Earlier
	// Dijkstra blocks carry the plain 10-field Babbage header body, for which
	// this is nil. The elements are retained verbatim so the header round-trips
	// and hashes identically to the bytes received on the wire; use
	// LeiosCertified and LeiosAnnouncement for typed access.
	LeiosHeaderExtension []cbor.RawMessage
}

func (h *DijkstraBlockHeader) UnmarshalCBOR(cborData []byte) error {
	// Fast path: legacy Dijkstra headers are byte-for-byte Babbage headers
	// with a 10-field body.
	var tmp babbage.BabbageBlockHeader
	if _, err := cbor.Decode(cborData, &tmp); err == nil {
		h.BabbageBlockHeader = tmp
		h.LeiosHeaderExtension = nil
		h.SetCbor(cborData)
		return nil
	}
	// Leios-extended header: the header body array carries extra trailing
	// fields after protocol_version. Decode the leading Babbage fields and
	// retain the remainder verbatim. The full original header CBOR is stored
	// for hashing, so the trailing fields never need typed interpretation to
	// follow the chain.
	var top []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &top); err != nil {
		return err
	}
	if len(top) != 2 {
		return fmt.Errorf(
			"unexpected Dijkstra block header: expected 2 elements, got %d",
			len(top),
		)
	}
	var bodyElems []cbor.RawMessage
	if _, err := cbor.Decode(top[0], &bodyElems); err != nil {
		return err
	}
	if len(bodyElems) < babbageHeaderBodyFieldCount {
		return fmt.Errorf(
			"unexpected Dijkstra block header body: expected at least %d fields, got %d",
			babbageHeaderBodyFieldCount,
			len(bodyElems),
		)
	}
	babbageBodyCbor, err := cbor.Encode(bodyElems[:babbageHeaderBodyFieldCount])
	if err != nil {
		return err
	}
	var body babbage.BabbageBlockHeaderBody
	if _, err := cbor.Decode(babbageBodyCbor, &body); err != nil {
		return err
	}
	// The leading-10-field re-encoding above is only used to populate the
	// typed Babbage fields. The body's stored CBOR must remain the ORIGINAL
	// body bytes (the full Leios-extended array), because KES signature
	// verification is computed over the original header-body encoding -- see
	// ledger.extractOriginalBodyCbor. Using the re-encoded 10-field bytes here
	// makes KES verification fail on Leios-extended headers near the tip.
	body.SetCbor([]byte(top[0]))
	var signature []byte
	if _, err := cbor.Decode(top[1], &signature); err != nil {
		return err
	}
	h.Body = body
	h.Signature = signature
	h.LeiosHeaderExtension = bodyElems[babbageHeaderBodyFieldCount:]
	h.SetCbor(cborData)
	return nil
}

func (h *DijkstraBlockHeader) MarshalCBOR() ([]byte, error) {
	// Decoded headers retain their original wire bytes (including any Leios
	// extension), which must be reproduced verbatim so the header hash is
	// stable.
	if cborData := h.Cbor(); cborData != nil {
		return cborData, nil
	}
	// Headers constructed in-process (no stored CBOR) have no Leios extension
	// to preserve; fall back to the Babbage encoding.
	return cbor.Encode(&h.BabbageBlockHeader)
}

func (h *DijkstraBlockHeader) Era() common.Era {
	return EraDijkstra
}

// LeiosCertified reports whether this ranking block certifies the endorser
// block announced by an earlier ranking block. It is the leios_certified header
// field added in the ouroboros-leios prototype-2026w27 release
// (IntersectMBO/cardano-ledger #5889); when true, the block body carries the
// leios_certificate. present is false for pre-Leios-extension Dijkstra headers,
// which carry no extension.
func (h *DijkstraBlockHeader) LeiosCertified() (certified bool, present bool) {
	if len(h.LeiosHeaderExtension) == 0 {
		return false, false
	}
	if _, err := cbor.Decode(h.LeiosHeaderExtension[0], &certified); err != nil {
		return false, false
	}
	return certified, true
}

// LeiosAnnouncement returns the endorser block this ranking block announces via
// its Leios header extension. It is the leios_announcement header field added in
// the ouroboros-leios prototype-2026w27 release (IntersectMBO/cardano-ledger
// #5889):
//
//	leios_announcement = [announced_eb : hash32, announced_eb_size : uint .size 4]
//
// ok is false when the block announces no endorser block (leios_announcement is
// nil), for pre-Leios-extension Dijkstra headers, or if the field is not the
// expected [hash32, uint] shape.
func (h *DijkstraBlockHeader) LeiosAnnouncement() (
	hash common.Blake2b256,
	size uint64,
	ok bool,
) {
	if len(h.LeiosHeaderExtension) < 2 {
		return common.Blake2b256{}, 0, false
	}
	var pair []cbor.RawMessage
	if _, err := cbor.Decode(h.LeiosHeaderExtension[1], &pair); err != nil ||
		len(pair) != 2 {
		return common.Blake2b256{}, 0, false
	}
	var hashBytes []byte
	if _, err := cbor.Decode(pair[0], &hashBytes); err != nil ||
		len(hashBytes) != common.Blake2b256Size {
		return common.Blake2b256{}, 0, false
	}
	if _, err := cbor.Decode(pair[1], &size); err != nil {
		return common.Blake2b256{}, 0, false
	}
	return common.NewBlake2b256(hashBytes), size, true
}

type DijkstraTransactionOutput struct {
	cbor.DecodeStoreCbor
	Output common.TransactionOutput
}

func (o *DijkstraTransactionOutput) UnmarshalCBOR(cborData []byte) error {
	if len(cborData) == 0 {
		return errors.New("empty Dijkstra transaction output")
	}
	switch cborData[0] & cbor.CborTypeMask {
	case cbor.CborTypeArray:
		// Match Conway compatibility: historical array-form outputs may still
		// appear, while new outputs use Babbage-style map encoding.
		var tmp alonzo.AlonzoTransactionOutput
		if _, err := cbor.Decode(cborData, &tmp); err != nil {
			return err
		}
		o.Output = &tmp
	case cbor.CborTypeMap:
		var tmp babbage.BabbageTransactionOutput
		if _, err := cbor.Decode(cborData, &tmp); err != nil {
			return err
		}
		o.Output = &tmp
	default:
		return fmt.Errorf(
			"unknown Dijkstra transaction output type: 0x%x",
			cborData[0],
		)
	}
	if err := checkMultiAssetDuplicateKeys(o.Output.Assets()); err != nil {
		return err
	}
	o.SetCborReference(cborData)
	return nil
}

func (o DijkstraTransactionOutput) MarshalCBOR() ([]byte, error) {
	if raw := o.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	if o.Output == nil {
		return cbor.Encode(nil)
	}
	return cbor.Encode(o.Output)
}

func (o DijkstraTransactionOutput) Address() common.Address {
	return o.Output.Address()
}

func (o DijkstraTransactionOutput) Amount() *big.Int {
	return o.Output.Amount()
}

func (o DijkstraTransactionOutput) Assets() *common.MultiAsset[common.MultiAssetTypeOutput] {
	return o.Output.Assets()
}

func (o DijkstraTransactionOutput) Datum() *common.Datum {
	return o.Output.Datum()
}

func (o DijkstraTransactionOutput) DatumHash() *common.Blake2b256 {
	return o.Output.DatumHash()
}

func (o DijkstraTransactionOutput) ScriptRef() common.Script {
	return o.Output.ScriptRef()
}

func (o DijkstraTransactionOutput) Utxorpc() (*utxorpc.TxOutput, error) {
	return o.Output.Utxorpc()
}

func (o DijkstraTransactionOutput) ToPlutusData() data.PlutusData {
	return o.Output.ToPlutusData()
}

func (o DijkstraTransactionOutput) String() string {
	return o.Output.String()
}

type DijkstraGuards struct {
	cbor.DecodeStoreCbor
	KeyHashes   []common.Blake2b224
	Credentials []common.Credential
}

func (g *DijkstraGuards) UnmarshalCBOR(cborData []byte) error {
	g.SetCbor(cborData)
	var credentials cbor.SetType[common.Credential]
	if _, err := cbor.Decode(cborData, &credentials); err == nil {
		if err := credentials.CheckForDuplicates(); err != nil {
			return err
		}
		if len(credentials.Items()) == 0 {
			return errors.New("dijkstra guards must not be empty")
		}
		g.Credentials = credentials.Items()
		g.KeyHashes = nil
		return nil
	}
	var keyHashes cbor.SetType[common.Blake2b224]
	if _, err := cbor.Decode(cborData, &keyHashes); err != nil {
		return err
	}
	if err := keyHashes.CheckForDuplicates(); err != nil {
		return err
	}
	if len(keyHashes.Items()) == 0 {
		return errors.New("dijkstra guards must not be empty")
	}
	g.KeyHashes = keyHashes.Items()
	g.Credentials = nil
	return nil
}

func (g DijkstraGuards) MarshalCBOR() ([]byte, error) {
	if raw := g.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	if len(g.Credentials) > 0 {
		return cbor.Encode(cbor.NewSetType(g.Credentials, true))
	}
	return cbor.Encode(cbor.NewSetType(g.KeyHashes, true))
}

type DijkstraRawCbor struct {
	cbor.DecodeStoreCbor
}

func (r *DijkstraRawCbor) UnmarshalCBOR(cborData []byte) error {
	r.SetCbor(cborData)
	return nil
}

func (r DijkstraRawCbor) MarshalCBOR() ([]byte, error) {
	if raw := r.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	return cbor.Encode(nil)
}

type DijkstraTransactionBody struct {
	common.TransactionBodyBase
	TxInputs                conway.ConwayTransactionInputSet              `cbor:"0,keyasint,omitempty"`
	TxOutputs               []DijkstraTransactionOutput                   `cbor:"1,keyasint,omitempty"`
	TxFee                   uint64                                        `cbor:"2,keyasint,omitempty"`
	Ttl                     uint64                                        `cbor:"3,keyasint,omitempty"`
	TxCertificates          []common.CertificateWrapper                   `cbor:"4,keyasint,omitempty"`
	TxWithdrawals           map[*common.Address]uint64                    `cbor:"5,keyasint,omitempty"`
	TxAuxDataHash           *common.Blake2b256                            `cbor:"7,keyasint,omitempty"`
	TxValidityIntervalStart uint64                                        `cbor:"8,keyasint,omitempty"`
	TxMint                  *common.MultiAsset[common.MultiAssetTypeMint] `cbor:"9,keyasint,omitempty"`
	TxScriptDataHash        *common.Blake2b256                            `cbor:"11,keyasint,omitempty"`
	TxCollateral            cbor.SetType[shelley.ShelleyTransactionInput] `cbor:"13,keyasint,omitempty,omitzero"`
	TxGuards                *DijkstraGuards                               `cbor:"14,keyasint,omitempty"`
	TxNetworkId             *uint8                                        `cbor:"15,keyasint,omitempty"`
	TxCollateralReturn      *DijkstraTransactionOutput                    `cbor:"16,keyasint,omitempty"`
	TxTotalCollateral       uint64                                        `cbor:"17,keyasint,omitempty"`
	TxReferenceInputs       cbor.SetType[shelley.ShelleyTransactionInput] `cbor:"18,keyasint,omitempty,omitzero"`
	TxVotingProcedures      common.VotingProcedures                       `cbor:"19,keyasint,omitempty"`
	TxProposalProcedures    []DijkstraProposalProcedure                   `cbor:"20,keyasint,omitempty"`
	TxCurrentTreasuryValue  uint64                                        `cbor:"21,keyasint,omitempty"`
	TxDonation              uint64                                        `cbor:"22,keyasint,omitempty"`
	TxSubTransactions       cbor.SetType[DijkstraSubTransaction]          `cbor:"23,keyasint,omitempty,omitzero"`
	TxDirectDeposits        map[cbor.ByteString]uint64                    `cbor:"25,keyasint,omitempty"`
	TxBalanceIntervals      *DijkstraRawCbor                              `cbor:"26,keyasint,omitempty"`
}

func (b *DijkstraTransactionBody) UnmarshalCBOR(cborData []byte) error {
	type tDijkstraTransactionBody DijkstraTransactionBody
	var tmp tDijkstraTransactionBody
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	// Reject duplicate members in any tag-258 set field on the transaction body.
	type duplicateChecker interface {
		CheckForDuplicates() error
	}
	for _, c := range []duplicateChecker{
		&tmp.TxInputs,
		&tmp.TxCollateral,
		&tmp.TxReferenceInputs,
		&tmp.TxSubTransactions,
	} {
		if err := c.CheckForDuplicates(); err != nil {
			return err
		}
	}
	if err := checkMultiAssetDuplicateKeys(tmp.TxMint); err != nil {
		return err
	}
	if tmp.TxCollateralReturn != nil {
		if err := checkMultiAssetDuplicateKeys(
			tmp.TxCollateralReturn.Assets(),
		); err != nil {
			return fmt.Errorf("collateral return: %w", err)
		}
	}
	*b = DijkstraTransactionBody(tmp)
	b.SetCborReference(cborData)
	return nil
}

func checkMultiAssetDuplicateKeys[T int64 | uint64 | *big.Int](
	assets *common.MultiAsset[T],
) error {
	if assets == nil {
		return nil
	}
	return assets.CheckForDuplicateKeys()
}

func (b *DijkstraTransactionBody) Inputs() []common.TransactionInput {
	return dijkstraTransactionInputs(b.TxInputs.Items())
}

func (b *DijkstraTransactionBody) Outputs() []common.TransactionOutput {
	return dijkstraTransactionOutputs(b.TxOutputs)
}

func (b *DijkstraTransactionBody) Fee() *big.Int {
	return new(big.Int).SetUint64(b.TxFee)
}

func (b *DijkstraTransactionBody) TTL() uint64 {
	return b.Ttl
}

func (b *DijkstraTransactionBody) ValidityIntervalStart() uint64 {
	return b.TxValidityIntervalStart
}

func (b *DijkstraTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return 0, nil
}

func (b *DijkstraTransactionBody) Certificates() []common.Certificate {
	return dijkstraCertificates(b.TxCertificates)
}

func (b *DijkstraTransactionBody) Withdrawals() map[*common.Address]*big.Int {
	return dijkstraWithdrawals(b.TxWithdrawals)
}

func (b *DijkstraTransactionBody) AuxDataHash() *common.Blake2b256 {
	return b.TxAuxDataHash
}

func (b *DijkstraTransactionBody) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return b.TxMint
}

func (b *DijkstraTransactionBody) Collateral() []common.TransactionInput {
	return dijkstraTransactionInputs(b.TxCollateral.Items())
}

func (b *DijkstraTransactionBody) RequiredSigners() []common.Blake2b224 {
	return dijkstraRequiredSigners(b.TxGuards)
}

func (b *DijkstraTransactionBody) ScriptDataHash() *common.Blake2b256 {
	return b.TxScriptDataHash
}

func (b *DijkstraTransactionBody) ReferenceInputs() []common.TransactionInput {
	return dijkstraReferenceInputs(b.TxReferenceInputs)
}

func (b *DijkstraTransactionBody) CollateralReturn() common.TransactionOutput {
	if b.TxCollateralReturn == nil {
		return nil
	}
	return b.TxCollateralReturn.Output
}

func (b *DijkstraTransactionBody) TotalCollateral() *big.Int {
	return new(big.Int).SetUint64(b.TxTotalCollateral)
}

func (b *DijkstraTransactionBody) VotingProcedures() common.VotingProcedures {
	return b.TxVotingProcedures
}

func (b *DijkstraTransactionBody) ProposalProcedures() []common.ProposalProcedure {
	return dijkstraProposalProcedures(b.TxProposalProcedures)
}

func (b *DijkstraTransactionBody) NetworkId() *uint8 {
	return b.TxNetworkId
}

func (b *DijkstraTransactionBody) CurrentTreasuryValue() *big.Int {
	return new(big.Int).SetUint64(b.TxCurrentTreasuryValue)
}

func (b *DijkstraTransactionBody) Donation() *big.Int {
	return new(big.Int).SetUint64(b.TxDonation)
}

func (b *DijkstraTransactionBody) Utxorpc() (*utxorpc.Tx, error) {
	return common.TransactionBodyToUtxorpc(b)
}

func dijkstraTransactionInputs(
	inputs []shelley.ShelleyTransactionInput,
) []common.TransactionInput {
	ret := make([]common.TransactionInput, 0, len(inputs))
	for _, input := range inputs {
		ret = append(ret, input)
	}
	return ret
}

func dijkstraTransactionOutputs(
	outputs []DijkstraTransactionOutput,
) []common.TransactionOutput {
	ret := make([]common.TransactionOutput, 0, len(outputs))
	for i := range outputs {
		if outputs[i].Output != nil {
			ret = append(ret, outputs[i].Output)
		}
	}
	return ret
}

func dijkstraCertificates(
	certificates []common.CertificateWrapper,
) []common.Certificate {
	ret := make([]common.Certificate, len(certificates))
	for i, cert := range certificates {
		ret[i] = cert.Certificate
	}
	return ret
}

func dijkstraWithdrawals(
	withdrawals map[*common.Address]uint64,
) map[*common.Address]*big.Int {
	if withdrawals == nil {
		return nil
	}
	ret := make(map[*common.Address]*big.Int, len(withdrawals))
	for addr, amount := range withdrawals {
		ret[addr] = new(big.Int).SetUint64(amount)
	}
	return ret
}

func dijkstraRequiredSigners(guards *DijkstraGuards) []common.Blake2b224 {
	if guards == nil {
		return nil
	}
	ret := make([]common.Blake2b224, 0, len(guards.KeyHashes)+len(guards.Credentials))
	ret = append(ret, guards.KeyHashes...)
	for _, cred := range guards.Credentials {
		if cred.CredType == common.CredentialTypeAddrKeyHash {
			ret = append(ret, cred.Credential)
		}
	}
	return ret
}

func dijkstraReferenceInputs(
	referenceInputs cbor.SetType[shelley.ShelleyTransactionInput],
) []common.TransactionInput {
	items := referenceInputs.Items()
	ret := make([]common.TransactionInput, len(items))
	for i := range items {
		ret[i] = &items[i]
	}
	return ret
}

func dijkstraProposalProcedures(
	proposalProcedures []DijkstraProposalProcedure,
) []common.ProposalProcedure {
	ret := make([]common.ProposalProcedure, len(proposalProcedures))
	for i, item := range proposalProcedures {
		ret[i] = item
	}
	return ret
}

type DijkstraSubTransactionBody struct {
	common.TransactionBodyBase
	TxInputs                  conway.ConwayTransactionInputSet              `cbor:"0,keyasint,omitempty"`
	TxOutputs                 []DijkstraTransactionOutput                   `cbor:"1,keyasint,omitempty"`
	Ttl                       uint64                                        `cbor:"3,keyasint,omitempty"`
	TxCertificates            []common.CertificateWrapper                   `cbor:"4,keyasint,omitempty"`
	TxWithdrawals             map[*common.Address]uint64                    `cbor:"5,keyasint,omitempty"`
	TxAuxDataHash             *common.Blake2b256                            `cbor:"7,keyasint,omitempty"`
	TxValidityIntervalStart   uint64                                        `cbor:"8,keyasint,omitempty"`
	TxMint                    *common.MultiAsset[common.MultiAssetTypeMint] `cbor:"9,keyasint,omitempty"`
	TxScriptDataHash          *common.Blake2b256                            `cbor:"11,keyasint,omitempty"`
	TxGuards                  *DijkstraGuards                               `cbor:"14,keyasint,omitempty"`
	TxNetworkId               *uint8                                        `cbor:"15,keyasint,omitempty"`
	TxReferenceInputs         cbor.SetType[shelley.ShelleyTransactionInput] `cbor:"18,keyasint,omitempty,omitzero"`
	TxVotingProcedures        common.VotingProcedures                       `cbor:"19,keyasint,omitempty"`
	TxProposalProcedures      []DijkstraProposalProcedure                   `cbor:"20,keyasint,omitempty"`
	TxCurrentTreasuryValue    uint64                                        `cbor:"21,keyasint,omitempty"`
	TxDonation                uint64                                        `cbor:"22,keyasint,omitempty"`
	TxRequiredTopLevelGuards  *DijkstraRawCbor                              `cbor:"24,keyasint,omitempty"`
	TxDirectDeposits          map[cbor.ByteString]uint64                    `cbor:"25,keyasint,omitempty"`
	TxAccountBalanceIntervals *DijkstraRawCbor                              `cbor:"26,keyasint,omitempty"`
}

func (b *DijkstraSubTransactionBody) UnmarshalCBOR(cborData []byte) error {
	type tDijkstraSubTransactionBody DijkstraSubTransactionBody
	var tmp tDijkstraSubTransactionBody
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	if err := tmp.TxInputs.CheckForDuplicates(); err != nil {
		return err
	}
	if err := tmp.TxReferenceInputs.CheckForDuplicates(); err != nil {
		return err
	}
	if err := checkMultiAssetDuplicateKeys(tmp.TxMint); err != nil {
		return err
	}
	*b = DijkstraSubTransactionBody(tmp)
	b.SetCborReference(cborData)
	return nil
}

func (b *DijkstraSubTransactionBody) Inputs() []common.TransactionInput {
	return dijkstraTransactionInputs(b.TxInputs.Items())
}

func (b *DijkstraSubTransactionBody) Outputs() []common.TransactionOutput {
	return dijkstraTransactionOutputs(b.TxOutputs)
}

func (b *DijkstraSubTransactionBody) TTL() uint64 {
	return b.Ttl
}

func (b *DijkstraSubTransactionBody) ValidityIntervalStart() uint64 {
	return b.TxValidityIntervalStart
}

func (b *DijkstraSubTransactionBody) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return 0, nil
}

func (b *DijkstraSubTransactionBody) Certificates() []common.Certificate {
	return dijkstraCertificates(b.TxCertificates)
}

func (b *DijkstraSubTransactionBody) Withdrawals() map[*common.Address]*big.Int {
	return dijkstraWithdrawals(b.TxWithdrawals)
}

func (b *DijkstraSubTransactionBody) AuxDataHash() *common.Blake2b256 {
	return b.TxAuxDataHash
}

func (b *DijkstraSubTransactionBody) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return b.TxMint
}

func (b *DijkstraSubTransactionBody) RequiredSigners() []common.Blake2b224 {
	return dijkstraRequiredSigners(b.TxGuards)
}

func (b *DijkstraSubTransactionBody) ScriptDataHash() *common.Blake2b256 {
	return b.TxScriptDataHash
}

func (b *DijkstraSubTransactionBody) ReferenceInputs() []common.TransactionInput {
	return dijkstraReferenceInputs(b.TxReferenceInputs)
}

func (b *DijkstraSubTransactionBody) VotingProcedures() common.VotingProcedures {
	return b.TxVotingProcedures
}

func (b *DijkstraSubTransactionBody) ProposalProcedures() []common.ProposalProcedure {
	return dijkstraProposalProcedures(b.TxProposalProcedures)
}

func (b *DijkstraSubTransactionBody) CurrentTreasuryValue() *big.Int {
	return new(big.Int).SetUint64(b.TxCurrentTreasuryValue)
}

func (b *DijkstraSubTransactionBody) Donation() *big.Int {
	return new(big.Int).SetUint64(b.TxDonation)
}

func (b *DijkstraSubTransactionBody) Utxorpc() (*utxorpc.Tx, error) {
	return common.TransactionBodyToUtxorpc(b)
}

type DijkstraRedeemers struct {
	cbor.DecodeStoreCbor
	Redeemers map[common.RedeemerKey]common.RedeemerValue
}

func (r *DijkstraRedeemers) UnmarshalCBOR(cborData []byte) error {
	if len(cborData) == 0 || (cborData[0]&cbor.CborTypeMask) != cbor.CborTypeMap {
		return errors.New("dijkstra redeemers must use map encoding")
	}
	var redeemers map[common.RedeemerKey]common.RedeemerValue
	if _, err := cbor.Decode(cborData, &redeemers); err != nil {
		return err
	}
	if len(redeemers) == 0 {
		return errors.New("dijkstra redeemers must not be empty")
	}
	r.Redeemers = redeemers
	r.SetCbor(cborData)
	return nil
}

func (r DijkstraRedeemers) MarshalCBOR() ([]byte, error) {
	if raw := r.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	return cbor.Encode(r.Redeemers)
}

func (r DijkstraRedeemers) Len() int {
	return len(r.Redeemers)
}

func (r DijkstraRedeemers) Iter() iter.Seq2[common.RedeemerKey, common.RedeemerValue] {
	return func(yield func(common.RedeemerKey, common.RedeemerValue) bool) {
		sorted := slices.Collect(maps.Keys(r.Redeemers))
		slices.SortFunc(sorted, common.CompareRedeemerKeys)
		for _, redeemerKey := range sorted {
			tmpVal := r.Redeemers[redeemerKey]
			if !yield(redeemerKey, tmpVal) {
				return
			}
		}
	}
}

func (r DijkstraRedeemers) Indexes(tag common.RedeemerTag) []uint {
	ret := []uint{}
	for key := range r.Redeemers {
		if key.Tag == tag {
			ret = append(ret, uint(key.Index))
		}
	}
	return ret
}

func (r DijkstraRedeemers) Value(index uint, tag common.RedeemerTag) common.RedeemerValue {
	redeemerVal, ok := r.Redeemers[common.RedeemerKey{
		Tag:   tag,
		Index: uint32(index), // #nosec G115
	}]
	if ok {
		return redeemerVal
	}
	return common.RedeemerValue{}
}

type DijkstraTransactionWitnessSet struct {
	cbor.DecodeStoreCbor
	VkeyWitnesses      cbor.SetType[common.VkeyWitness]      `cbor:"0,keyasint,omitempty,omitzero"`
	WsNativeScripts    cbor.SetType[common.NativeScript]     `cbor:"1,keyasint,omitempty,omitzero"`
	BootstrapWitnesses cbor.SetType[common.BootstrapWitness] `cbor:"2,keyasint,omitempty,omitzero"`
	WsPlutusV1Scripts  cbor.SetType[common.PlutusV1Script]   `cbor:"3,keyasint,omitempty,omitzero"`
	WsPlutusData       cbor.SetType[common.Datum]            `cbor:"4,keyasint,omitempty,omitzero"`
	WsRedeemers        DijkstraRedeemers                     `cbor:"5,keyasint,omitempty,omitzero"`
	WsPlutusV2Scripts  cbor.SetType[common.PlutusV2Script]   `cbor:"6,keyasint,omitempty,omitzero"`
	WsPlutusV3Scripts  cbor.SetType[common.PlutusV3Script]   `cbor:"7,keyasint,omitempty,omitzero"`
	WsPlutusV4Scripts  cbor.SetType[common.PlutusV4Script]   `cbor:"8,keyasint,omitempty,omitzero"`
}

func (w *DijkstraTransactionWitnessSet) UnmarshalCBOR(cborData []byte) error {
	type tDijkstraTransactionWitnessSet DijkstraTransactionWitnessSet
	var tmp tDijkstraTransactionWitnessSet
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	// Reject duplicate members in any tag-258 witness set field.
	// Untagged array fields are left unchecked so pre-Dijkstra encodings remain valid.
	type duplicateChecker interface {
		CheckForDuplicates() error
	}
	for _, c := range []duplicateChecker{
		&tmp.VkeyWitnesses,
		&tmp.WsNativeScripts,
		&tmp.BootstrapWitnesses,
		&tmp.WsPlutusV1Scripts,
		&tmp.WsPlutusData,
		&tmp.WsPlutusV2Scripts,
		&tmp.WsPlutusV3Scripts,
		&tmp.WsPlutusV4Scripts,
	} {
		if err := c.CheckForDuplicates(); err != nil {
			return err
		}
	}
	*w = DijkstraTransactionWitnessSet(tmp)
	w.SetCbor(cborData)
	return nil
}

func (w DijkstraTransactionWitnessSet) Vkey() []common.VkeyWitness {
	return w.VkeyWitnesses.Items()
}

func (w DijkstraTransactionWitnessSet) Bootstrap() []common.BootstrapWitness {
	return w.BootstrapWitnesses.Items()
}

func (w DijkstraTransactionWitnessSet) NativeScripts() []common.NativeScript {
	return w.WsNativeScripts.Items()
}

func (w DijkstraTransactionWitnessSet) PlutusV1Scripts() []common.PlutusV1Script {
	return w.WsPlutusV1Scripts.Items()
}

func (w DijkstraTransactionWitnessSet) PlutusV2Scripts() []common.PlutusV2Script {
	return w.WsPlutusV2Scripts.Items()
}

func (w DijkstraTransactionWitnessSet) PlutusV3Scripts() []common.PlutusV3Script {
	return w.WsPlutusV3Scripts.Items()
}

func (w DijkstraTransactionWitnessSet) PlutusV4Scripts() []common.PlutusV4Script {
	return w.WsPlutusV4Scripts.Items()
}

func (w DijkstraTransactionWitnessSet) PlutusData() []common.Datum {
	return w.WsPlutusData.Items()
}

func (w DijkstraTransactionWitnessSet) Redeemers() common.TransactionWitnessRedeemers {
	return w.WsRedeemers
}

type DijkstraTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash       *common.Blake2b256
	Body       DijkstraTransactionBody
	WitnessSet DijkstraTransactionWitnessSet
	TxIsValid  bool
	TxMetadata common.TransactionMetadatum
	auxData    common.AuxiliaryData
}

func (t *DijkstraTransaction) UnmarshalCBOR(cborData []byte) error {
	tmpTx, err := newDijkstraTransactionFromCbor(cborData, true)
	if err != nil {
		return err
	}
	*t = *tmpTx
	return nil
}

func (t *DijkstraTransaction) Metadata() common.TransactionMetadatum {
	return t.TxMetadata
}

func (t *DijkstraTransaction) AuxiliaryData() common.AuxiliaryData {
	return t.auxData
}

func (DijkstraTransaction) Type() int {
	return TxTypeDijkstra
}

func (t DijkstraTransaction) Hash() common.Blake2b256 {
	return t.Id()
}

func (t DijkstraTransaction) Id() common.Blake2b256 {
	return t.Body.Id()
}

func (t *DijkstraTransaction) LeiosHash() common.Blake2b256 {
	if t.hash == nil {
		tmpHash := common.Blake2b256Hash(t.Cbor())
		t.hash = &tmpHash
	}
	return *t.hash
}

func (t DijkstraTransaction) Inputs() []common.TransactionInput {
	return t.Body.Inputs()
}

func (t DijkstraTransaction) Outputs() []common.TransactionOutput {
	return t.Body.Outputs()
}

func (t DijkstraTransaction) Fee() *big.Int {
	return t.Body.Fee()
}

func (t DijkstraTransaction) TTL() uint64 {
	return t.Body.TTL()
}

func (t DijkstraTransaction) ValidityIntervalStart() uint64 {
	return t.Body.ValidityIntervalStart()
}

func (t DijkstraTransaction) ProtocolParameterUpdates() (uint64, map[common.Blake2b224]common.ProtocolParameterUpdate) {
	return t.Body.ProtocolParameterUpdates()
}

func (t DijkstraTransaction) ReferenceInputs() []common.TransactionInput {
	return t.Body.ReferenceInputs()
}

func (t DijkstraTransaction) Collateral() []common.TransactionInput {
	return t.Body.Collateral()
}

func (t DijkstraTransaction) CollateralReturn() common.TransactionOutput {
	return t.Body.CollateralReturn()
}

func (t DijkstraTransaction) TotalCollateral() *big.Int {
	return t.Body.TotalCollateral()
}

func (t DijkstraTransaction) Certificates() []common.Certificate {
	return t.Body.Certificates()
}

func (t DijkstraTransaction) Withdrawals() map[*common.Address]*big.Int {
	return t.Body.Withdrawals()
}

func (t DijkstraTransaction) AuxDataHash() *common.Blake2b256 {
	return t.Body.AuxDataHash()
}

func (t DijkstraTransaction) RequiredSigners() []common.Blake2b224 {
	return t.Body.RequiredSigners()
}

func (t DijkstraTransaction) AssetMint() *common.MultiAsset[common.MultiAssetTypeMint] {
	return t.Body.AssetMint()
}

func (t DijkstraTransaction) ScriptDataHash() *common.Blake2b256 {
	return t.Body.ScriptDataHash()
}

func (t DijkstraTransaction) VotingProcedures() common.VotingProcedures {
	return t.Body.VotingProcedures()
}

func (t DijkstraTransaction) ProposalProcedures() []common.ProposalProcedure {
	return t.Body.ProposalProcedures()
}

func (t DijkstraTransaction) CurrentTreasuryValue() *big.Int {
	return t.Body.CurrentTreasuryValue()
}

func (t DijkstraTransaction) Donation() *big.Int {
	return t.Body.Donation()
}

func (t DijkstraTransaction) NetworkId() *uint8 {
	return t.Body.NetworkId()
}

func (t DijkstraTransaction) IsValid() bool {
	return t.TxIsValid
}

func (t DijkstraTransaction) Consumed() []common.TransactionInput {
	if t.IsValid() {
		return t.Inputs()
	}
	return t.Collateral()
}

func (t DijkstraTransaction) Produced() []common.Utxo {
	if t.IsValid() {
		outputs := t.Outputs()
		ret := make([]common.Utxo, 0, len(outputs))
		for idx, output := range outputs {
			ret = append(
				ret,
				common.Utxo{
					Id: shelley.NewShelleyTransactionInput(
						t.Hash().String(),
						idx,
					),
					Output: output,
				},
			)
		}
		return ret
	}
	if t.CollateralReturn() == nil {
		return []common.Utxo{}
	}
	return []common.Utxo{
		{
			Id: shelley.NewShelleyTransactionInput(
				t.Hash().String(),
				len(t.Outputs()),
			),
			Output: t.CollateralReturn(),
		},
	}
}

func (t DijkstraTransaction) Witnesses() common.TransactionWitnessSet {
	return t.WitnessSet
}

func (t *DijkstraTransaction) MarshalCBOR() ([]byte, error) {
	if cborData := t.DecodeStoreCbor.Cbor(); cborData != nil {
		return cborData, nil
	}
	var aux any
	if t.auxData != nil && len(t.auxData.Cbor()) > 0 {
		aux = cbor.RawMessage(t.auxData.Cbor())
	} else if t.TxMetadata != nil {
		aux = cbor.RawMessage(t.TxMetadata.Cbor())
	}
	return cbor.Encode([]any{t.Body, t.WitnessSet, aux})
}

func (t *DijkstraTransaction) Cbor() []byte {
	if cborData := t.DecodeStoreCbor.Cbor(); cborData != nil {
		return cborData[:]
	}
	if t.Body.Cbor() == nil {
		return nil
	}
	cborData, err := t.MarshalCBOR()
	if err != nil {
		panic("CBOR encoding that should never fail has failed: " + err.Error())
	}
	return cborData
}

func (t *DijkstraTransaction) Utxorpc() (*utxorpc.Tx, error) {
	tx, err := t.Body.Utxorpc()
	if err != nil {
		return nil, fmt.Errorf("failed to convert Dijkstra transaction: %w", err)
	}
	return tx, nil
}

type DijkstraSubTransaction struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	Body       DijkstraSubTransactionBody
	WitnessSet DijkstraTransactionWitnessSet
	TxMetadata common.TransactionMetadatum
	auxData    common.AuxiliaryData
}

func (t *DijkstraSubTransaction) UnmarshalCBOR(cborData []byte) error {
	var txArray []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &txArray); err != nil {
		return err
	}
	if len(txArray) != 3 {
		return fmt.Errorf(
			"invalid Dijkstra sub-transaction: expected 3 components, got %d",
			len(txArray),
		)
	}
	if _, err := cbor.Decode(txArray[0], &t.Body); err != nil {
		return fmt.Errorf("failed to decode sub-transaction body: %w", err)
	}
	if _, err := cbor.Decode(txArray[1], &t.WitnessSet); err != nil {
		return fmt.Errorf(
			"failed to decode sub-transaction witness set: %w",
			err,
		)
	}
	if err := decodeAuxiliaryDataInto(txArray[2], &t.TxMetadata, &t.auxData); err != nil {
		return err
	}
	t.SetCbor(cborData)
	return nil
}

func (t *DijkstraSubTransaction) MarshalCBOR() ([]byte, error) {
	if raw := t.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	var aux any
	if t.auxData != nil && len(t.auxData.Cbor()) > 0 {
		aux = cbor.RawMessage(t.auxData.Cbor())
	} else if t.TxMetadata != nil {
		aux = cbor.RawMessage(t.TxMetadata.Cbor())
	}
	return cbor.Encode([]any{t.Body, t.WitnessSet, aux})
}

func NewDijkstraBlockFromCbor(
	data []byte,
	config ...common.VerifyConfig,
) (*DijkstraBlock, error) {
	var cfg common.VerifyConfig
	if len(config) > 0 {
		cfg = config[0]
	}
	var dijkstraBlock DijkstraBlock
	if _, err := cbor.Decode(data, &dijkstraBlock); err != nil {
		return nil, fmt.Errorf("decode Dijkstra block error: %w", err)
	}
	if !cfg.SkipBodyHashValidation {
		if dijkstraBlock.BlockHeader == nil {
			return nil, errors.New("dijkstra block header is nil")
		}
		expected := dijkstraBlock.BlockHeader.BlockBodyHash()
		actual := dijkstraBlock.CalculatedBlockBodyHash()
		actualBytes := actual[:]
		expectedBytes := expected[:]
		if len(actualBytes) != len(expectedBytes) ||
			subtle.ConstantTimeCompare(actualBytes, expectedBytes) != 1 {
			return nil, common.NewValidationError(
				common.ValidationErrorTypeBodyHash,
				"Dijkstra block body hash mismatch during parsing",
				map[string]any{
					"era":           EraNameDijkstra,
					"expected_hash": expected.String(),
					"actual_hash":   actual.String(),
				},
				nil,
			)
		}
	}
	return &dijkstraBlock, nil
}

func NewDijkstraBlockHeaderFromCbor(data []byte) (*DijkstraBlockHeader, error) {
	var dijkstraBlockHeader DijkstraBlockHeader
	if _, err := cbor.Decode(data, &dijkstraBlockHeader); err != nil {
		return nil, fmt.Errorf("decode Dijkstra block header error: %w", err)
	}
	return &dijkstraBlockHeader, nil
}

func NewDijkstraTransactionBodyFromCbor(
	data []byte,
) (*DijkstraTransactionBody, error) {
	var dijkstraTx DijkstraTransactionBody
	if _, err := cbor.Decode(data, &dijkstraTx); err != nil {
		return nil, fmt.Errorf("decode Dijkstra transaction body error: %w", err)
	}
	return &dijkstraTx, nil
}

func NewDijkstraTransactionFromCbor(data []byte) (*DijkstraTransaction, error) {
	return newDijkstraTransactionFromCbor(data, true)
}

// NewDijkstraTransactionFromCborComponents decodes a Dijkstra transaction
// from an already-decoded top-level transaction array.
func NewDijkstraTransactionFromCborComponents(
	data []byte,
	txArray []cbor.RawMessage,
) (*DijkstraTransaction, error) {
	if err := validateDijkstraTransactionCborSize(data); err != nil {
		return nil, err
	}
	return newDijkstraTransactionFromCborComponents(data, txArray, true)
}

func newDijkstraTransactionFromCbor(
	data []byte,
	allowIsValid bool,
) (*DijkstraTransaction, error) {
	if err := validateDijkstraTransactionCborSize(data); err != nil {
		return nil, err
	}
	var txArray []cbor.RawMessage
	if _, err := cbor.Decode(data, &txArray); err != nil {
		return nil, err
	}
	return newDijkstraTransactionFromCborComponents(data, txArray, allowIsValid)
}

func validateDijkstraTransactionCborSize(data []byte) error {
	if len(data) <= MaxTxSize {
		return nil
	}
	return fmt.Errorf(
		"newDijkstraTransactionFromCbor: transaction size %d exceeds MaxTxSize %d",
		len(data),
		MaxTxSize,
	)
}

func newDijkstraTransactionFromCborComponents(
	data []byte,
	txArray []cbor.RawMessage,
	allowIsValid bool,
) (*DijkstraTransaction, error) {
	var ret DijkstraTransaction
	if len(txArray) != 3 && len(txArray) != 4 {
		return nil, fmt.Errorf(
			"invalid Dijkstra transaction: expected 3 or 4 components, got %d",
			len(txArray),
		)
	}
	if len(txArray) == 4 && !allowIsValid {
		return nil, errors.New(
			"dijkstra transactions in blocks cannot include is_valid",
		)
	}
	if _, err := cbor.Decode(txArray[0], &ret.Body); err != nil {
		return nil, fmt.Errorf("failed to decode transaction body: %w", err)
	}
	if _, err := cbor.Decode(txArray[1], &ret.WitnessSet); err != nil {
		return nil, fmt.Errorf("failed to decode transaction witness set: %w", err)
	}
	auxIdx := 2
	if len(txArray) == 4 {
		var txIsValid bool
		if _, err := cbor.Decode(txArray[2], &txIsValid); err != nil {
			return nil, fmt.Errorf("failed to decode TxIsValid: %w", err)
		}
		if !txIsValid {
			return nil, errors.New("dijkstra transactions cannot encode is_valid=false")
		}
		auxIdx = 3
	}
	ret.TxIsValid = true
	if err := decodeAuxiliaryDataInto(
		txArray[auxIdx],
		&ret.TxMetadata,
		&ret.auxData,
	); err != nil {
		return nil, err
	}
	ret.SetCbor(data)
	return &ret, nil
}

func decodeInvalidTransactions(raw cbor.RawMessage) ([]uint, error) {
	if isCborNull(raw) {
		return nil, nil
	}
	// invalid_transactions = nonempty_set<transaction_index>, which the CDDL
	// allows as either a tag-258 set or a plain array; SetType accepts both.
	var txIndices cbor.SetType[uint64]
	if _, err := cbor.Decode(raw, &txIndices); err != nil {
		return nil, fmt.Errorf("decode Dijkstra invalid transactions: %w", err)
	}
	items := txIndices.Items()
	ret := make([]uint, len(items))
	for i, idx := range items {
		ret[i] = uint(idx)
	}
	return ret, nil
}

func decodeAuxiliaryDataInto(
	raw cbor.RawMessage,
	metadata *common.TransactionMetadatum,
	auxData *common.AuxiliaryData,
) error {
	*metadata = nil
	*auxData = nil
	if isCborNull(raw) {
		return nil
	}
	aux, err := common.DecodeAuxiliaryData(raw)
	if err == nil && aux != nil {
		*auxData = aux
		md, _ := aux.Metadata()
		if md != nil {
			*metadata = md
		}
		return nil
	}
	md, err := common.DecodeAuxiliaryDataToMetadata(raw)
	if err == nil && md != nil {
		*metadata = md
		return nil
	}
	return errors.New("decode Dijkstra auxiliary data")
}

func isCborNull(raw cbor.RawMessage) bool {
	return len(raw) == 1 && raw[0] == 0xf6
}
