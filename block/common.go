package block

import (
	"encoding/hex"
	"fmt"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/blake2b"
)

type Blake2b256 [32]byte

func (b Blake2b256) String() string {
	return hex.EncodeToString([]byte(b[:]))
}

type Blake2b224 [28]byte

func (b Blake2b224) String() string {
	return hex.EncodeToString([]byte(b[:]))
}

func NewBlockFromCbor(blockType uint, data []byte) (interface{}, error) {
	var err error
	// Parse outer list to get at header CBOR
	var rawBlock []cbor.RawMessage
	if err := cbor.Unmarshal(data, &rawBlock); err != nil {
		return nil, err
	}
	switch blockType {
	case BLOCK_TYPE_BYRON_EBB:
		var byronEbbBlock ByronEpochBoundaryBlock
		if err := cbor.Unmarshal(data, &byronEbbBlock); err != nil {
			return nil, fmt.Errorf("chain-sync: decode error: %s", err)
		}
		// Prepend bytes for CBOR list wrapper
		// The block hash is calculated with these extra bytes, so we have to add them to
		// get the correct value
		byronEbbBlock.Header.id, err = generateBlockHeaderHash(rawBlock[0], []byte{0x82, BLOCK_TYPE_BYRON_EBB})
		return &byronEbbBlock, err
	case BLOCK_TYPE_BYRON_MAIN:
		var byronMainBlock ByronMainBlock
		if err := cbor.Unmarshal(data, &byronMainBlock); err != nil {
			return nil, fmt.Errorf("chain-sync: decode error: %s", err)
		}
		// Prepend bytes for CBOR list wrapper
		// The block hash is calculated with these extra bytes, so we have to add them to
		// get the correct value
		byronMainBlock.Header.id, err = generateBlockHeaderHash(rawBlock[0], []byte{0x82, BLOCK_TYPE_BYRON_MAIN})
		return &byronMainBlock, err
	case BLOCK_TYPE_SHELLEY:
		var shelleyBlock ShelleyBlock
		if err := cbor.Unmarshal(data, &shelleyBlock); err != nil {
			return nil, fmt.Errorf("chain-sync: decode error: %s", err)
		}
		shelleyBlock.Header.id, err = generateBlockHeaderHash(rawBlock[0], nil)
		return &shelleyBlock, err
	case BLOCK_TYPE_ALLEGRA:
		var allegraBlock AllegraBlock
		if err := cbor.Unmarshal(data, &allegraBlock); err != nil {
			return nil, fmt.Errorf("chain-sync: decode error: %s", err)
		}
		allegraBlock.Header.id, err = generateBlockHeaderHash(rawBlock[0], nil)
		return &allegraBlock, err
	case BLOCK_TYPE_MARY:
		var maryBlock MaryBlock
		if err := cbor.Unmarshal(data, &maryBlock); err != nil {
			return nil, fmt.Errorf("chain-sync: decode error: %s", err)
		}
		maryBlock.Header.id, err = generateBlockHeaderHash(rawBlock[0], nil)
		return &maryBlock, err
	case BLOCK_TYPE_ALONZO:
		var alonzoBlock AlonzoBlock
		if err := cbor.Unmarshal(data, &alonzoBlock); err != nil {
			return nil, fmt.Errorf("chain-sync: decode error: %s", err)
		}
		alonzoBlock.Header.id, err = generateBlockHeaderHash(rawBlock[0], nil)
		return &alonzoBlock, err
	}
	return nil, nil
}

func NewBlockHeaderFromCbor(blockType uint, data []byte) (interface{}, error) {
	var err error
	switch blockType {
	case BLOCK_TYPE_BYRON_EBB:
		var byronEbbBlockHeader ByronEpochBoundaryBlockHeader
		if err := cbor.Unmarshal(data, &byronEbbBlockHeader); err != nil {
			return nil, fmt.Errorf("chain-sync: decode error: %s", err)
		}
		// Prepend bytes for CBOR list wrapper
		// The block hash is calculated with these extra bytes, so we have to add them to
		// get the correct value
		byronEbbBlockHeader.id, err = generateBlockHeaderHash(data, []byte{0x82, BLOCK_TYPE_BYRON_EBB})
		return &byronEbbBlockHeader, err
	case BLOCK_TYPE_BYRON_MAIN:
		var byronMainBlockHeader ByronMainBlockHeader
		if err := cbor.Unmarshal(data, &byronMainBlockHeader); err != nil {
			return nil, fmt.Errorf("chain-sync: decode error: %s", err)
		}
		// Prepend bytes for CBOR list wrapper
		// The block hash is calculated with these extra bytes, so we have to add them to
		// get the correct value
		byronMainBlockHeader.id, err = generateBlockHeaderHash(data, []byte{0x82, BLOCK_TYPE_BYRON_MAIN})
		return &byronMainBlockHeader, err
	default:
		var shelleyBlockHeader ShelleyBlockHeader
		if err := cbor.Unmarshal(data, &shelleyBlockHeader); err != nil {
			return nil, fmt.Errorf("chain-sync: decode error: %s", err)
		}
		shelleyBlockHeader.id, err = generateBlockHeaderHash(data, nil)
		return &shelleyBlockHeader, err
	}
}

func generateBlockHeaderHash(data []byte, prefix []byte) (string, error) {
	tmpHash, err := blake2b.New256(nil)
	if err != nil {
		return "", err
	}
	if prefix != nil {
		tmpHash.Write(prefix)
	}
	tmpHash.Write(data)
	return hex.EncodeToString(tmpHash.Sum(nil)), nil
}
