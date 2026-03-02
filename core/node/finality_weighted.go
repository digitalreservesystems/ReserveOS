package node

import (
	"reserveos/core/consensus/finality"
	"reserveos/internal/reservestorage"
)

// TryFinalizeHeight tallies votes for a checkpoint height using validator registry weights.
// Quorum rule: >= 2/3 of total (non-slashed) validator weight.
func (n *Node) TryFinalizeHeight(height uint64) {
	votes, err := reservestorage.ListVotes(n.db.DB, height, 5000)
	if err != nil { return }
	vals, err := reservestorage.ListValidators(n.db.DB, 5000)
	if err != nil || len(vals) == 0 { return }

	weightByPub := map[string]int64{}
	var totalW int64
	for _, v := range vals {
		if v.Slashed || v.Weight <= 0 { continue }
		weightByPub[v.PubkeyHex] = v.Weight
		totalW += v.Weight
	}
	if totalW <= 0 { return }

	tally := map[string]int64{}
	for _, vr := range votes {
		w := weightByPub[vr.PubkeyHex]
		if w <= 0 { continue }
		tally[vr.BlockHash] += w
	}

	var bestHash string
	var bestW int64
	for h, wv := range tally {
		if wv > bestW { bestW = wv; bestHash = h }
	}

	if bestW*3 < totalW*2 { return }

	finH, _, _ := reservestorage.GetFinalizedHeight(n.db.DB)
	if height > finH {
		_ = reservestorage.SetFinalizedHeight(n.db.DB, height)
		_ = reservestorage.SetFinalizedHash(n.db.DB, bestHash)
		_ = reservestorage.PutCheckpoint(n.db.DB, height, finality.Checkpoint{Height: height, BlockHash: bestHash})
	}
}
