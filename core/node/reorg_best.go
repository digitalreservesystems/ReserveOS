package node

import (
	"fmt"

	"reserveos/core/chain"
	"reserveos/internal/reservestorage"
)

// ReorgToBestHeaderChain attempts to move canonical chain to the best header tip (by cumwork),
// without violating finalized boundary. Requires blocks for the target chain to be present.
func (n *Node) ReorgToBestHeaderChain() error {
	// Determine current tip
	curH, curHash, okCur, _ := reservestorage.GetTip(n.db.DB)
	if !okCur { curH = 0; curHash = "" }

	// Determine best header tip
	hH, hHash, okH, _ := reservestorage.GetHeaderTip(n.db.DB)
	if !okH || hHash == "" { return fmt.Errorf("no_header_tip") }

	// If already on same tip hash, nothing to do
	if curHash != "" && curHash == hHash { return nil }

	// Enforce finalized boundary on candidate tip
	if err := n.enforceFinalizedOnCandidate(hHash); err != nil { return err }

	// Find fork point by walking header ancestors until we hit a block on current chain
	forkH, forkHash, err := n.findForkPointCached(curH, curHash, hH, hHash)
	if err != nil { return err }

	// Collect txs from old branch (best-effort) to reinsert after reorg
	oldTxs := make([]chain.Tx, 0, 1024)
	if curH > forkH {
		for h := forkH + 1; h <= curH; h++ {
			var ob chain.Block
			okb, _, _ := reservestorage.GetBlockByHeight(n.db.DB, h, &ob)
			if !okb { break }
			for _, tx := range ob.Txs { if tx.Type != "coinbase" { oldTxs = append(oldTxs, tx) } }
		}
	}

	// Rebuild state to fork height
	if err := n.RebuildStateToHeight(forkH); err != nil { return err }

	// Now advance from forkH+1 to header tip, applying blocks in order from header chain
	path, err := n.headerPathFromTo(forkH+1, hH, hHash)
	if err != nil { return err }
	for _, bh := range path {
		// require block exists
		var b any
		_ = b
		var blk chain.Block
		ok, _, _ := reservestorage.GetBlockByHash(n.db.DB, bh, &blk)
		if !ok { return fmt.Errorf("missing_block_%s", bh) }
		// Apply block to state (updates balances/nonces etc)
		if err := n.BlockMatchesStoredHeader(&blk); err != nil { return err }
		if err := n.ApplyBlock(&blk); err != nil { return err }
		_ = reservestorage.SetTip(n.db.DB, blk.Header.Height, bh)
	}

	n.ReinsertTxs(oldTxs)
	return nil
}

func (n *Node) findForkPoint(curH uint64, curHash string, headH uint64, headHash string) (uint64, string, error) {
	// if current chain empty
	if curHash == "" {
		return 0, "", nil
	}
	// walk down header chain until we find a height where header hash equals current chain hash at that height
	// We can test membership by fetching main-chain block at that height and comparing hash.
	cur := headHash
	for {
		hdr, ok, err := reservestorage.GetHeaderByHash(n.db.DB, cur)
		if err != nil { return 0, "", err }
		if !ok { return 0, "", fmt.Errorf("unknown_header") }
		h := hdr.Height
		if h == 0 { return 0, "", nil }
		if h <= curH {
			var b chain.Block
			okb, _, _ := reservestorage.GetBlockByHeight(n.db.DB, h, &b)
			if okb {
				if b.Hash().String() == cur {
					return h, cur, nil
				}
			}
		}
		cur = hdr.PrevHash
	}
}

func (n *Node) headerPathFromTo(fromH uint64, toH uint64, tipHash string) ([]string, error) {
	if toH < fromH { return []string{}, nil }
	seq := make([]string, 0, toH-fromH+1)
	cur := tipHash
	for {
		hdr, ok, err := reservestorage.GetHeaderByHash(n.db.DB, cur)
		if err != nil { return nil, err }
		if !ok { return nil, fmt.Errorf("unknown_header") }
		if hdr.Height < fromH { break }
		if hdr.Height <= toH { seq = append(seq, cur) }
		if hdr.Height == fromH { break }
		cur = hdr.PrevHash
	}
	// reverse seq
	for i, j := 0, len(seq)-1; i < j; i, j = i+1, j-1 { seq[i], seq[j] = seq[j], seq[i] }
	return seq, nil
}
