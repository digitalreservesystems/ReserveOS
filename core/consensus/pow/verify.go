package pow

import (
	"math/big"
	"reserveos/core/chain"
)

func HashToBig(h chain.Hash) *big.Int { return new(big.Int).SetBytes(h[:]) }

func MeetsTarget(hash chain.Hash, bits uint32) bool {
	target := CompactToBig(bits)
	if target.Sign() <= 0 { return false }
	return HashToBig(hash).Cmp(target) <= 0
}
