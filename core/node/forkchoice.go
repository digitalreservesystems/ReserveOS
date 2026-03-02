package node

import (
	"errors"

	"reserveos/core/chain"
	"reserveos/internal/reservestorage"
)

// trySwitchMainChain rewrites the height->hash index and tip metadata to follow newTipHash,
// but ONLY if new chain contains the locally finalized anchor at finalized height.
func (n *Node) trySwitchMainChain(newTipHash string, newTipHeight uint64) error {
	finH, finHash, ok, err := reservestorage.GetFinalized(n.db.DB)
	if err != nil { return err }
	if !ok {
		finH = 0
		var gb chain.Block
		okb, ghash, _ := reservestorage.GetBlockByHeight(n.db.DB, 0, &gb)
		if !okb { return errors.New("missing genesis") }
		finHash = ghash
	}

	// Walk back from new tip to finalized height, collecting path
	path := make([]string, 0, int(newTipHeight-finH)+1)
	h := newTipHash
	curHeight := newTipHeight
	for {
		path = append(path, h)
		if curHeight == finH {
			break
		}
		var b chain.Block
		okb, err := reservestorage.GetBlockByHash(n.db.DB, h, &b)
		if err != nil || !okb { return errors.New("missing block in candidate chain") }
		h = b.Header.PrevHash.String()
		if curHeight == 0 { break }
		curHeight--
		if curHeight < finH { return errors.New("candidate does not reach finalized height") }
	}
	// Verify anchor matches
	if path[len(path)-1] != finHash {
		return errors.New("candidate chain anchor mismatch")
	}

	// Rewrite main-chain height mapping from finH..newTipHeight
	// path currently is tip->...->fin in reverse order
	for i := len(path)-1; i >= 0; i-- {
		height := finH + uint64(len(path)-1-i)
		_ = reservestorage.PutHashByHeightOverwrite(n.db.DB, height, path[i])
	}
	_ = reservestorage.SetTip(n.db.DB, newTipHeight, newTipHash)
	_ = n.rebuildStateFromFinalized()
	return nil
}
