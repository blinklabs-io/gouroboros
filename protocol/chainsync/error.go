package chainsync

type IntersectNotFoundError struct {
}

func (e IntersectNotFoundError) Error() string {
	return "chain intersection not found"
}
