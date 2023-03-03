package cbor

import (
	"fmt"
	"reflect"

	_cbor "github.com/fxamacker/cbor/v2"
	"github.com/jinzhu/copier"
)

const (
	CBOR_TYPE_BYTE_STRING uint8 = 0x40
	CBOR_TYPE_TEXT_STRING uint8 = 0x60
	CBOR_TYPE_ARRAY       uint8 = 0x80
	CBOR_TYPE_MAP         uint8 = 0xa0

	// Only the top 3 bytes are used to specify the type
	CBOR_TYPE_MASK uint8 = 0xe0

	// Max value able to be stored in a single byte without type prefix
	CBOR_MAX_UINT_SIMPLE uint8 = 0x17
)

// Create an alias for RawMessage for convenience
type RawMessage = _cbor.RawMessage

// Alias for Tag for convenience
type Tag = _cbor.Tag

// Useful for embedding and easier to remember
type StructAsArray struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_ struct{} `cbor:",toarray"`
}

type DecodeStoreCborInterface interface {
	Cbor() []byte
}

type DecodeStoreCbor struct {
	cborData []byte
}

// Cbor returns the original CBOR for the object
func (d *DecodeStoreCbor) Cbor() []byte {
	return d.cborData
}

// UnmarshalCborGeneric decodes the specified CBOR into the destination object without using the
// destination object's UnmarshalCBOR() function
func (d *DecodeStoreCbor) UnmarshalCborGeneric(cborData []byte, dest DecodeStoreCborInterface) error {
	// Create a duplicate(-ish) struct from the destination
	// We do this so that we can bypass any custom UnmarshalCBOR() function on the
	// destination object
	valueDest := reflect.ValueOf(dest)
	if valueDest.Kind() != reflect.Pointer || valueDest.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("destination must be a pointer to a struct")
	}
	typeDestElem := valueDest.Elem().Type()
	destTypeFields := []reflect.StructField{}
	for i := 0; i < typeDestElem.NumField(); i++ {
		tmpField := typeDestElem.Field(i)
		if tmpField.IsExported() && tmpField.Name != "DecodeStoreCbor" {
			destTypeFields = append(destTypeFields, tmpField)
		}
	}
	// Create temporary object with the type created above
	tmpDest := reflect.New(reflect.StructOf(destTypeFields))
	// Decode CBOR into temporary object
	if _, err := Decode(cborData, tmpDest.Interface()); err != nil {
		return err
	}
	// Copy values from temporary object into destination object
	if err := copier.Copy(dest, tmpDest.Interface()); err != nil {
		return err
	}
	// Store a copy of the original CBOR data
	// This must be done after we copy from the temp object above, or it gets wiped out
	// when using struct embedding and the DecodeStoreCbor struct is embedded at a deeper level
	d.cborData = make([]byte, len(cborData))
	copy(d.cborData, cborData)
	return nil
}
