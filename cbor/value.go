package cbor

// Helpful wrapper for parsing arbitrary CBOR data which may contain types that
// cannot be easily represented in Go (such as maps with bytestring keys)
type Value struct {
	Value interface{}
}

func (v *Value) UnmarshalCBOR(data []byte) error {
	cborType := data[0] & CBOR_TYPE_MASK
	switch cborType {
	case CBOR_TYPE_MAP:
		tmpValue := map[Value]Value{}
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		// Extract actual value from each child value
		newValue := map[interface{}]interface{}{}
		for key, value := range tmpValue {
			newValue[key.Value] = value.Value
		}
		v.Value = newValue
	case CBOR_TYPE_ARRAY:
		tmpValue := []Value{}
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		// Extract actual value from each child value
		newValue := []interface{}{}
		for _, value := range tmpValue {
			newValue = append(newValue, value.Value)
		}
		v.Value = newValue
	case CBOR_TYPE_TEXT_STRING:
		var tmpValue string
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		v.Value = tmpValue
	case CBOR_TYPE_BYTE_STRING:
		// Use our custom type which stores the bytestring in a way that allows it to be used as a map key
		tmpValue := ByteString{}
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		v.Value = tmpValue
	default:
		var tmpValue interface{}
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		v.Value = tmpValue
	}
	return nil
}
