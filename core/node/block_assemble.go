package node

import (
	"sort"
	"reserveos/core/chain"
	"reserveos/core/state"
)

// FilterTxsBySimState applies txs sequentially in-memory and returns only those that remain valid.
// NOTE: This is a lightweight safety net for block assembly. Full block validation still happens in SubmitBlock.
func (n *Node) FilterTxsBySimState(txs []chain.Tx) []chain.Tx {
	simBal := map[string]int64{}
	simNonce := map[string]uint64{}

	getBal := func(id string) int64 {
		if v, ok := simBal[id]; ok { return v }
		b, _, _ := state.GetBalance(n.db.DB, id)
		simBal[id] = b
		return b
	}
	setBal := func(id string, v int64) { simBal[id] = v }
	getNonce := func(id string) uint64 {
		if v, ok := simNonce[id]; ok { return v }
		n, _, _ := state.GetNonce(n.db.DB, id)
		simNonce[id] = n
		return n
	}
	setNonce := func(id string, v uint64) { simNonce[id] = v }

	sort.Slice(txs, func(i,j int) bool {
		if txs[i].Fee != txs[j].Fee { return txs[i].Fee > txs[j].Fee }
		return txs[i].ID() < txs[j].ID()
	})

	out := make([]chain.Tx, 0, len(txs))
	for _, tx := range txs {
		if tx.Type == "coinbase" {
			out = append(out, tx)
			// credit outputs
			for _, o := range tx.Outputs {
				to := o.Address
				setBal(to, getBal(to)+o.Amount)
			}
			continue
		}
		from := tx.From
		// require nonce monotonic
		exp := getNonce(from) + 1
		if tx.Nonce != exp { continue }

		if tx.Type == "otap_claim" && tx.OTAPClaim != nil {
			bucket := "otap:" + tx.OTAPClaim.P
			bb := getBal(bucket)
			need := tx.OTAPClaim.Amount + tx.Fee
			if bb < need { continue }
			// deduct bucket, credit to, nonce update
			setBal(bucket, bb-need)
			to := tx.OTAPClaim.To
			setBal(to, getBal(to)+tx.OTAPClaim.Amount)
			setNonce(from, tx.Nonce)
			out = append(out, tx)
			continue
		}

		// normal transfer
		var outTotal int64
		for _, o := range tx.Outputs { outTotal += o.Amount }
		bal := getBal(from)
		need := outTotal + tx.Fee
		if bal < need { continue }
		setBal(from, bal-need)
		setNonce(from, tx.Nonce)
		for _, o := range tx.Outputs {
			if o.Address != "" {
				setBal(o.Address, getBal(o.Address)+o.Amount)
			} else if o.P != "" {
				k := "otap:" + o.P
				setBal(k, getBal(k)+o.Amount)
			}
		}
		out = append(out, tx)
	}
	return out
}
