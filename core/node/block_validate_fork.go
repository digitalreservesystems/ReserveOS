package node

import (
	"fmt"
	"strconv"
	"strings"

	"reserveos/core/chain"
	"reserveos/core/state"
	"reserveos/internal/reservestorage"
)

func (n *Node) ValidateBlockStateOnMainParent(b *chain.Block) error {
	if !n.cfg.Validation.ValidateForkState { return nil }
	if b.Header.Height == 0 { return nil }
	parentH := b.Header.Height - 1

	var parent chain.Block
	ok, _, _ := reservestorage.GetBlockByHeight(n.db.DB, parentH, &parent)
	if !ok { return nil }
	if parent.Hash().String() != b.Header.PrevHash { return nil }

	tipH, _, _, _ := reservestorage.GetTip(n.db.DB)
	if tipH > parentH && (tipH-parentH) > n.cfg.Validation.MaxReplayBlocks { return nil }

	simBal := map[string]int64{}
	simNonce := map[string]uint64{}
	snapH := uint64(0)

	snaps, _ := state.ListSnapshots(n.cfg.State.SnapshotDir)
	best := pickSnapshotForHeight(snaps, parentH)
	if best != "" {
		ss, err := state.ReadSnapshot(best)
		if err == nil {
			snapH = ss.Height
			for k,v := range ss.Balances { simBal[k] = v }
			for k,v := range ss.Nonces { simNonce[k] = v }
		}
	}
	if snapH == 0 {
		for _, a := range n.genesis.Allocations { simBal[a.Address] += parseInt64(a.Balance) }
	}

	getBal := func(id string) int64 { return simBal[id] }
	setBal := func(id string, v int64) { simBal[id] = v }
	getNonce := func(id string) uint64 { return simNonce[id] }
	setNonce := func(id string, v uint64) { simNonce[id] = v }

	for h := snapH + 1; h <= parentH; h++ {
		var blk chain.Block
		okb, _, _ := reservestorage.GetBlockByHeight(n.db.DB, h, &blk)
		if !okb { break }
		if err := applyBlockToSim(n, &blk, getBal, setBal, getNonce, setNonce); err != nil {
			return fmt.Errorf("replay_invalid:%w", err)
		}
	}
	return applyBlockToSim(n, b, getBal, setBal, getNonce, setNonce)
}

func applyBlockToSim(n *Node, b *chain.Block,
	getBal func(string) int64, setBal func(string,int64),
	getNonce func(string) uint64, setNonce func(string,uint64),
) error {
	if err := n.ValidateLimits(b); err != nil { return err }
	if err := n.ValidateBlockBasic(b); err != nil { return err }
	for _, tx := range b.Txs {
		if tx.Type == "coinbase" {
			for _, o := range tx.Outputs { setBal(o.Address, getBal(o.Address)+o.Amount) }
			continue
		}
		from := tx.From
		exp := getNonce(from) + 1
		if tx.Nonce != exp { return fmt.Errorf("bad_nonce") }
		if tx.Type == "otap_claim" && tx.OTAPClaim != nil {
			bucket := "otap:" + tx.OTAPClaim.P
			need := tx.OTAPClaim.Amount + tx.Fee
			bb := getBal(bucket)
			if bb < need { return fmt.Errorf("insufficient_otap_bucket") }
			setBal(bucket, bb-need)
			setBal(tx.OTAPClaim.To, getBal(tx.OTAPClaim.To)+tx.OTAPClaim.Amount)
			setNonce(from, tx.Nonce)
			continue
		}
		var outTotal int64
		for _, o := range tx.Outputs { outTotal += o.Amount }
		bal := getBal(from)
		need := outTotal + tx.Fee
		if bal < need { return fmt.Errorf("insufficient_funds") }
		setBal(from, bal-need)
		setNonce(from, tx.Nonce)
		for _, o := range tx.Outputs {
			if o.Address != "" { setBal(o.Address, getBal(o.Address)+o.Amount) } else if o.P != "" { k := "otap:" + o.P; setBal(k, getBal(k)+o.Amount) }
		}
	}
	return nil
}

func pickSnapshotForHeight(paths []string, h uint64) string {
	best := ""
	bestH := uint64(0)
	for _, p := range paths {
		base := baseName(p)
		s, ok := parseSnapshotHeight(base)
		if ok && s <= h && s >= bestH { bestH = s; best = p }
	}
	return best
}
func baseName(p string) string { for i:=len(p)-1;i>=0;i--{ if p[i]=='/'||p[i]=='\\' { return p[i+1:] } }; return p }
func parseSnapshotHeight(base string) (uint64,bool) {
	if !strings.HasPrefix(base, "state_") { return 0,false }
	base = strings.TrimPrefix(base, "state_")
	base = strings.TrimSuffix(base, ".json.gz")
	hh, err := strconv.ParseUint(base, 10, 64)
	if err != nil { return 0,false }
	return hh,true
}
