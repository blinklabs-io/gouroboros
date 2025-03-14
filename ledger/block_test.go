package ledger

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/fxamacker/cbor/v2"
)

func mockShelleyCBOR() []byte {
	shelleyHeader := shelley.ShelleyBlockHeader{
		Body: shelley.ShelleyBlockHeaderBody{
			BlockNumber:          12345,
			Slot:                 67890,
			PrevHash:             common.Blake2b256{},
			IssuerVkey:           common.IssuerVkey{},
			VrfKey:               []byte{0x01, 0x02},
			NonceVrf:             common.VrfResult{},
			LeaderVrf:            common.VrfResult{},
			BlockBodySize:        512,
			BlockBodyHash:        common.Blake2b256{},
			OpCertHotVkey:        []byte{0x03, 0x04},
			OpCertSequenceNumber: 10,
			OpCertKesPeriod:      20,
			OpCertSignature:      []byte{0x05, 0x06},
			ProtoMajorVersion:    1,
			ProtoMinorVersion:    0,
		},
		Signature: []byte{0x07, 0x08},
	}

	// Convert to CBOR
	data, err := cbor.Marshal(shelleyHeader)
	if err != nil {
		fmt.Printf("CBOR Encoding Error: %v\n", err)
	}
	return data
}

func mockAllegraCBOR() []byte {
	allegraHeader := allegra.AllegraBlockHeader{ShelleyBlockHeader: ShelleyBlockHeader{}}
	data, _ := cbor.Marshal(allegraHeader)
	return data
}

func mockMaryCBOR() []byte {
	maryHeader := mary.MaryBlockHeader{ShelleyBlockHeader: ShelleyBlockHeader{}}
	data, _ := cbor.Marshal(maryHeader)
	return data
}

func mockAlonzoCBOR() []byte {
	alonzoHeader := AlonzoBlockHeader{ShelleyBlockHeader: ShelleyBlockHeader{}}
	data, _ := cbor.Marshal(alonzoHeader)
	return data
}

func mockBabbageCBOR() []byte {
	babbageHeader := babbage.BabbageBlockHeader{
		Body: babbage.BabbageBlockHeaderBody{
			BlockNumber:   54321,
			Slot:          98765,
			PrevHash:      common.Blake2b256{},
			IssuerVkey:    common.IssuerVkey{},
			VrfKey:        []byte{0x09, 0x10},
			VrfResult:     common.VrfResult{},
			BlockBodySize: 1024,
			BlockBodyHash: common.Blake2b256{},
			OpCert: babbage.BabbageOpCert{
				HotVkey:        []byte{0x11, 0x12},
				SequenceNumber: 30,
				KesPeriod:      40,
				Signature:      []byte{0x13, 0x14},
			},
			ProtoVersion: babbage.BabbageProtoVersion{
				Major: 2,
				Minor: 0,
			},
		},
		Signature: []byte{0x15, 0x16},
	}

	// Convert to CBOR
	data, err := cbor.Marshal(babbageHeader)
	if err != nil {
		fmt.Printf("CBOR Encoding Error for Babbage: %v\n", err)
	}
	return data
}

func mockConwayCBOR() []byte {
	conwayHeader := ConwayBlockHeader{BabbageBlockHeader: BabbageBlockHeader{}}
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
		{"Shelley Block", BlockTypeShelley, mockShelleyCBOR(), false, "NewShelleyBlockHeaderFromCbor"},
		{"Allegra Block", BlockTypeAllegra, mockAllegraCBOR(), false, "NewAllegraBlockHeaderFromCbor"},
		{"Mary Block", BlockTypeMary, mockMaryCBOR(), false, "NewMaryBlockHeaderFromCbor"},
		{"Alonzo Block", BlockTypeAlonzo, mockAlonzoCBOR(), false, "NewAlonzoBlockHeaderFromCbor"},
		{"Babbage Block", BlockTypeBabbage, mockBabbageCBOR(), false, "NewBabbageBlockHeaderFromCbor"},
		{"Conway Block", BlockTypeConway, mockConwayCBOR(), false, "NewConwayBlockHeaderFromCbor"},
		{"Invalid Block Type", 9999, []byte{0xFF, 0x00, 0x00}, true, "UnknownFunction"},
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
}
