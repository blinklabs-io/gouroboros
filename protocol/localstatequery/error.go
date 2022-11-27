package localstatequery

type AcquireFailurePointTooOldError struct {
}

func (e AcquireFailurePointTooOldError) Error() string {
	return "acquire failure: point too old"
}

type AcquireFailurePointNotOnChainError struct {
}

func (e AcquireFailurePointNotOnChainError) Error() string {
	return "acquire failure: point not on chain"
}
