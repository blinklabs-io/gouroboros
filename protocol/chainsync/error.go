package chainsync

// IntersectNotFoundError represents a failure to find a chain intersection
type IntersectNotFoundError struct {
}

func (e IntersectNotFoundError) Error() string {
	return "chain intersection not found"
}
