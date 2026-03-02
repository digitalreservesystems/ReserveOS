package node

import "reserveos/core/chain"

// ReinsertTxs best-effort reinserts txs back into mempool after a reorg/orphaning.
func (n *Node) ReinsertTxs(txs []chain.Tx) {
	for _, tx := range txs {
		_, _ = n.mp.Add(n.db.DB, tx)
	}
}
