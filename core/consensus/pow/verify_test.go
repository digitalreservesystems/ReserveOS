package pow

import (
	"testing"
	"reserveos/core/chain"
)

func TestMeetsTargetDoesNotPanic(t *testing.T) {
	var h chain.Hash
	_ = MeetsTarget(h, 1)
}
