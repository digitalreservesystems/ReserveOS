package pow

import (
	"math/big"
)

// Work64 returns a monotonic estimate of work for a given compact target.
// It maps smaller target -> larger work, using a 64-bit approximation.
func Work64(bits uint32) uint64 {
	t := CompactToBig(bits)
	if t.Sign() <= 0 {
		return 0
	}
	// take top 64 bits of target to approximate difficulty
	bb := t.Bytes()
	var top uint64
	if len(bb) >= 8 {
		for i := 0; i < 8; i++ {
			top = (top << 8) | uint64(bb[i])
		}
	} else {
		for i := 0; i < len(bb); i++ {
			top = (top << 8) | uint64(bb[i])
		}
		top = top << uint(8*(8-len(bb)))
	}
	if top == 0 {
		return ^uint64(0)
	}
	return ^uint64(0) / (top + 1)
}

// Cumulative adds work; saturates on overflow.
func Cumulative(parent uint64, add uint64) uint64 {
	sum := parent + add
	if sum < parent {
		return ^uint64(0)
	}
	return sum
}

var _ = big.NewInt
