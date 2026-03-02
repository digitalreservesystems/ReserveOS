package node

import (
	"errors"

	"reserveos/core/chain"
	"reserveos/core/state"
	"reserveos/internal/reservestorage"
)

func (n *Node) applyBlockTxs(b *chain.Block) error {
	var feeSum int64
	for _, tx := range b.Txs {
		_ = reservestorage.PutTx(n.db.DB, tx)
		// debit sender: outputs + fee (or claim)
		if tx.Type == "" { tx.Type = "transfer" }
		if tx.Type == "otap_claim" && tx.OTAPClaim != nil {
			// pay fee from sender
			_ = state.SetNonce(n.db.DB, tx.From, tx.Nonce)
			bucket := "otap:" + tx.OTAPClaim.P
			// deduct amount + fee from bucket
			if _, err := state.AddBalance(n.db.DB, bucket, -(tx.OTAPClaim.Amount+tx.Fee)); err != nil { return err }
			if _, err := state.AddBalance(n.db.DB, tx.OTAPClaim.To, tx.OTAPClaim.Amount); err != nil { return err }
			continue
		}

		// debit sender: outputs + fee
		var outTotal int64
		for _, o := range tx.Outputs { outTotal += o.Amount }
		if tx.From != "" {
			if _, err := state.AddBalance(n.db.DB, tx.From, -(outTotal+tx.Fee)); err != nil { return err }
			_ = state.SetNonce(n.db.DB, tx.From, tx.Nonce)
		}
		// credit outputs
		for _, o := range tx.Outputs {
			if o.Address != "" {
				_, err := state.AddBalance(n.db.DB, o.Address, o.Amount)
				if err != nil { return err }
				continue
			}
			// OTAP output: credit to a claim bucket keyed by P (receiver can later produce a claim tx)
			if o.P != "" {
				_, err := state.AddBalance(n.db.DB, "otap:"+o.P, o.Amount)
				if err != nil { return err }
			}
		}
	}
	reservestorage.PutBlockFeeSum(n.db.DB, b.Header.Height, feeSum)
	return nil
}

func (n *Node) rebuildStateFromFinalized() error {
	// WARNING: demo implementation: rebuild balances/nonces from genesis each time.
	// Attempt to restore from latest snapshot <= tip to speed up rebuilds.
	var snapHeight uint64 = 0
	snaps, _ := state.ListSnapshots(n.cfg.State.SnapshotDir)
	if len(snaps) > 0 {
		// pick last snapshot file
		path := snaps[len(snaps)-1]
		ss, err := state.ReadSnapshotAuto(path)
		if err == nil {
			_ = state.RestoreSnapshot(n.db.DB, ss)
			snapHeight = ss.Height
		}
	}
	if snapHeight == 0 {
		if err := state.ResetState(n.db.DB); err != nil { return err }
		// Apply genesis allocations as balances
		for _, a := range n.genesis.Allocations {
			_, _ = state.AddBalance(n.db.DB, a.Address, parseInt64(a.Balance))
		}
	}

	// Replay selected main chain blocks from height snapHeight+1..tip
	tipH, _, ok, err := reservestorage.GetTip(n.db.DB)
	if err != nil || !ok { return err }

	for h := uint64(snapHeight+1); h <= tipH; h++ {
		var b chain.Block
		okb, _, err := reservestorage.GetBlockByHeight(n.db.DB, h, &b)
		if err != nil || !okb { return errors.New("missing block on main chain") }
		if err := n.applyBlockTxs(&b); err != nil { return err }
	}
	reservestorage.PutBlockFeeSum(n.db.DB, b.Header.Height, feeSum)
	return nil
}

func parseInt64(s string) int64 {
	var n int64
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' { continue }
		n = n*10 + int64(c-'0')
	}
	return n
}
