package chainsync

import (
	"fmt"
)

// IntersectNotFoundError represents a failure to find a chain intersection
type IntersectNotFoundError struct {
}

func (e IntersectNotFoundError) Error() string {
	return "chain intersection not found"
}

// StopChainSync is used as a special return value from a RollForward or RollBackward handler function
// to signify that the sync process should be stopped
var StopSyncProcessError = fmt.Errorf("stop sync process")
