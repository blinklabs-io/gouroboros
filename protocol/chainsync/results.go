package chainsync

type RequestNextResult struct {
	Rollback  bool
	Block     interface{}
	BlockType uint8
}
