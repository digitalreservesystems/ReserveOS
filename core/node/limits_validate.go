package node

import (
	"fmt"
	"reserveos/core/chain"
)

func (n *Node) ValidateLimits(b *chain.Block) error {
	if n.cfg.Limits.MaxBlockTxs > 0 && len(b.Txs) > n.cfg.Limits.MaxBlockTxs { return fmt.Errorf("too_many_txs") }
	if n.cfg.Limits.MaxBlockBytes > 0 && b.SizeHint() > n.cfg.Limits.MaxBlockBytes { return fmt.Errorf("block_too_large") }
	for _, tx := range b.Txs {
		if n.cfg.Limits.MaxTxOutputs > 0 && len(tx.Outputs) > n.cfg.Limits.MaxTxOutputs { return fmt.Errorf("tx_too_many_outputs") }
	}
	return nil
}
