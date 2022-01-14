package block

import (
	"encoding/hex"
)

type Blake2b256 [32]byte

func (b Blake2b256) String() string {
	return hex.EncodeToString([]byte(b[:]))
}

type Blake2b224 [28]byte

func (b Blake2b224) String() string {
	return hex.EncodeToString([]byte(b[:]))
}
