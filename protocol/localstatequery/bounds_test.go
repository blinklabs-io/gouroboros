package localstatequery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateLocalStateQuerySetRejectsOversizedInput(t *testing.T) {
	items := make([]int, maxLocalStateQuerySetItems+1)
	require.Error(t, validateLocalStateQuerySet(items, "items"))
}

func TestValidateLocalStateQuerySetRejectsDuplicateInput(t *testing.T) {
	require.Error(t, validateLocalStateQuerySet([]int{1, 2, 1}, "items"))
}

func TestValidateLocalStateQuerySetPreservesDistinctInput(t *testing.T) {
	require.NoError(t, validateLocalStateQuerySet([]int{1, 2, 3}, "items"))
}
