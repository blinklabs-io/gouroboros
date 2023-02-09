package localstatequery

// AcquireFailurePointTooOldError indicates a failure to acquire a point due to it being too old
type AcquireFailurePointTooOldError struct {
}

func (e AcquireFailurePointTooOldError) Error() string {
	return "acquire failure: point too old"
}

// AcquireFailurePointNotOnChainError indicates a failure to acquire a point due to it not being present on the chain
type AcquireFailurePointNotOnChainError struct {
}

func (e AcquireFailurePointNotOnChainError) Error() string {
	return "acquire failure: point not on chain"
}
