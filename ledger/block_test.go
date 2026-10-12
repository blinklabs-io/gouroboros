package ledger

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/fxamacker/cbor/v2"
)

func mockShelleyHeader() ShelleyBlockHeader {
	return ShelleyBlockHeader{
		Body: shelley.ShelleyBlockHeaderBody{
			VrfKey: make([]byte, 32),
			NonceVrf: common.VrfResult{
				Output: make([]byte, 64),
				Proof:  make([]byte, 80),
			},
			LeaderVrf: common.VrfResult{
				Output: make([]byte, 64),
				Proof:  make([]byte, 80),
			},
			OpCertHotVkey:   make([]byte, 32),
			OpCertSignature: make([]byte, 64),
		},
		Signature: make([]byte, kes.CardanoKesSignatureSize),
	}
}

func mockShelleyCBOR() []byte {
	shelleyHeader := shelley.ShelleyBlockHeader{
		Body: shelley.ShelleyBlockHeaderBody{
			BlockNumber: 12345,
			Slot:        67890,
			PrevHash:    common.Blake2b256{},
			IssuerVkey:  common.IssuerVkey{},
			VrfKey:      make([]byte, 32),
			NonceVrf: common.VrfResult{
				Output: make([]byte, 64),
				Proof:  make([]byte, 80),
			},
			LeaderVrf: common.VrfResult{
				Output: make([]byte, 64),
				Proof:  make([]byte, 80),
			},
			BlockBodySize:        512,
			BlockBodyHash:        common.Blake2b256{},
			OpCertHotVkey:        make([]byte, 32),
			OpCertSequenceNumber: 10,
			OpCertKesPeriod:      20,
			OpCertSignature:      make([]byte, 64),
			ProtoMajorVersion:    1,
			ProtoMinorVersion:    0,
		},
		Signature: make([]byte, kes.CardanoKesSignatureSize),
	}

	// Convert to CBOR
	data, err := cbor.Marshal(shelleyHeader)
	if err != nil {
		fmt.Printf("CBOR Encoding Error: %v\n", err)
	}
	return data
}

func mockAllegraCBOR() []byte {
	allegraHeader := allegra.AllegraBlockHeader{
		ShelleyBlockHeader: mockShelleyHeader(),
	}
	data, _ := cbor.Marshal(allegraHeader)
	return data
}

func mockMaryCBOR() []byte {
	maryHeader := mary.MaryBlockHeader{ShelleyBlockHeader: mockShelleyHeader()}
	data, _ := cbor.Marshal(maryHeader)
	return data
}

func mockAlonzoCBOR() []byte {
	alonzoHeader := AlonzoBlockHeader{ShelleyBlockHeader: mockShelleyHeader()}
	data, _ := cbor.Marshal(alonzoHeader)
	return data
}

func mockBabbageHeader() babbage.BabbageBlockHeader {
	return babbage.BabbageBlockHeader{
		Body: babbage.BabbageBlockHeaderBody{
			BlockNumber: 54321,
			Slot:        98765,
			PrevHash:    common.Blake2b256{},
			IssuerVkey:  common.IssuerVkey{},
			VrfKey:      make([]byte, 32),
			VrfResult: common.VrfResult{
				Output: make([]byte, 64),
				Proof:  make([]byte, 80),
			},
			BlockBodySize: 1024,
			BlockBodyHash: common.Blake2b256{},
			OpCert: babbage.BabbageOpCert{
				HotVkey:        make([]byte, 32),
				SequenceNumber: 30,
				KesPeriod:      40,
				Signature:      make([]byte, 64),
			},
			ProtoVersion: babbage.BabbageProtoVersion{
				Major: 2,
				Minor: 0,
			},
		},
		Signature: make([]byte, kes.CardanoKesSignatureSize),
	}
}

func mockBabbageCBOR() []byte {
	// Convert to CBOR
	data, err := cbor.Marshal(mockBabbageHeader())
	if err != nil {
		fmt.Printf("CBOR Encoding Error for Babbage: %v\n", err)
	}
	return data
}

func mockConwayCBOR() []byte {
	conwayHeader := ConwayBlockHeader{
		BabbageBlockHeader: mockBabbageHeader(),
	}
	data, _ := cbor.Marshal(conwayHeader)
	return data
}

func TestNewBlockHeaderFromCbor(t *testing.T) {
	tests := []struct {
		name       string
		blockType  uint
		data       []byte
		expectErr  bool
		expectedFn string
	}{
		{
			"Shelley Block",
			BlockTypeShelley,
			mockShelleyCBOR(),
			false,
			"NewShelleyBlockHeaderFromCbor",
		},
		{
			"Allegra Block",
			BlockTypeAllegra,
			mockAllegraCBOR(),
			false,
			"NewAllegraBlockHeaderFromCbor",
		},
		{
			"Mary Block",
			BlockTypeMary,
			mockMaryCBOR(),
			false,
			"NewMaryBlockHeaderFromCbor",
		},
		{
			"Alonzo Block",
			BlockTypeAlonzo,
			mockAlonzoCBOR(),
			false,
			"NewAlonzoBlockHeaderFromCbor",
		},
		{
			"Babbage Block",
			BlockTypeBabbage,
			mockBabbageCBOR(),
			false,
			"NewBabbageBlockHeaderFromCbor",
		},
		{
			"Conway Block",
			BlockTypeConway,
			mockConwayCBOR(),
			false,
			"NewConwayBlockHeaderFromCbor",
		},
		{
			"Invalid Block Type",
			9999,
			[]byte{0xFF, 0x00, 0x00},
			true,
			"UnknownFunction",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fmt.Printf("\n Running Test: %s\n", test.name)

			header, err := NewBlockHeaderFromCbor(test.blockType, test.data)

			if test.expectErr {
				if err == nil {
					t.Errorf("Expected error for %s, but got none!", test.name)
				} else {
					fmt.Printf("Expected failure for %s: %v\n", test.name, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %s: %v", test.name, err)
				} else if header == nil {
					t.Errorf("Expected non-nil block header for %s, but got nil", test.name)
				} else {
					fmt.Printf("Test Passed: %s → %s executed successfully!\n", test.name, test.expectedFn)
				}
			}
		})
	}

	for _, test := range tests {
		if test.expectErr {
			continue
		}
		t.Run(test.name+" trailing CBOR", func(t *testing.T) {
			if _, err := NewBlockHeaderFromCbor(test.blockType, test.data); err != nil {
				t.Fatalf("decode block header fixture: %v", err)
			}
			data := append(append([]byte(nil), test.data...), 0x00)
			if _, err := NewBlockHeaderFromCbor(test.blockType, data); err == nil {
				t.Fatal("block header with trailing CBOR was accepted")
			}
		})
	}
}
