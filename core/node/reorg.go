package node

import (
	"fmt"

	"reserveos/core/chain"
	"reserveos/core/state"
	"reserveos/internal/reservestorage"
)

// RebuildStateToHeight restores state from the best snapshot <= target and replays main-chain blocks up to target.
func (n *Node) RebuildStateToHeight(target uint64) error {
	// refuse below finalized
	finH, okF, _ := reservestorage.GetFinalizedHeight(n.db.DB)
	if okF && finH > 0 && target < finH {
		return fmt.Errorf("below_finalized")
	}
	// restore state from snapshot <= target (state_apply.go already has restore helper logic; here we trigger apply path)
	// Best-effort: call ApplyStateFromSnapshots(target)
	return n.ApplyStateUpTo(target)
}

// ApplyStateUpTo replays blocks from snapshot to height.
func (n *Node) ApplyStateUpTo(target uint64) error {
	// choose best snapshot <= target
	snaps, _ := state.ListSnapshots(n.cfg.State.SnapshotDir)
	best := ""
	bestH := uint64(0)
	for _, p := range snaps {
		ss, err := state.ReadSnapshotAuto(p)
		if err != nil { continue }
		if ss.Height <= target && ss.Height >= bestH {
			bestH = ss.Height
			best = p
		}
	}
	// reset state DB by wiping prefixes (bal:/nonce:) then apply snapshot maps
	if best != "" {
		ss, err := state.ReadSnapshotAuto(best)
		if err != nil { return err }
		_ = reservestorage.WipeStatePrefixes(n.db.DB)
		_ = reservestorage.ImportStateMaps(n.db.DB, ss.Balances, ss.Nonces)
	} else {
		_ = reservestorage.WipeStatePrefixes(n.db.DB)
		// apply genesis allocations
		for _, a := range n.genesis.Allocations {
			_ = reservestorage.SetBalance(n.db.DB, a.Address, parseInt64(a.Balance))
		}
	}
	// replay blocks
	for h := bestH + 1; h <= target; h++ {
		var b chain.Block
		ok, _, _ := reservestorage.GetBlockByHeight(n.db.DB, h, &b)
		if !ok { return fmt.Errorf("missing_block_%d", h) }
		if err := n.ApplyBlock(&b); err != nil { return err }
	}
	return nil
}
