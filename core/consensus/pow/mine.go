package pow

import "reserveos/core/chain"

func MineHeader(h *chain.BlockHeader) (chain.Hash, uint64) {
	for {
		hash := h.Hash()
		if MeetsTarget(hash, h.Bits) { return hash, h.Nonce }
		h.Nonce++
	}
}
