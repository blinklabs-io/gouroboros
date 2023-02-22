package protocol

import (
	"fmt"
)

var ProtocolShuttingDownError = fmt.Errorf("protocol is shutting down")
