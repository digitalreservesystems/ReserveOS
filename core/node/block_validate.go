package node

import (
	"errors"

	"reserveos/core/chain"
	"reserveos/core/crypto/sig"
	"reserveos/core/economics/fees"
	"reserveos/core/state"
	"reserveos/core/crypto/schnorr"

	"filippo.io/edwards25519"
)

// ValidateBlockBasic enforces deterministic block rules that do not depend on fork state.
func (n *Node) ValidateBlockBasic(b *chain.Block) error {
	// Coinbase rules
	if n.cfg.Issuance.Enabled {
		if len(b.Txs) == 0 { return errors.New("missing coinbase") }
		if b.Txs[0].Type != "coinbase" { return errors.New("coinbase must be first") }
		for i := 1; i < len(b.Txs); i++ {
			if b.Txs[i].Type == "coinbase" { return errors.New("multiple coinbase") }
		}
		// coinbase amount must match configured reward (v1)
		if len(b.Txs[0].Outputs) != 1 { return errors.New("coinbase must have exactly 1 output") }
		if b.Txs[0].Outputs[0].Amount != n.cfg.Issuance.BlockReward { return errors.New("bad coinbase amount") }
	} else {
		for _, tx := range b.Txs {
			if tx.Type == "coinbase" { return errors.New("coinbase disabled") }
		}
	}

	// Tx signature validity (syntactic). Full balance/nonce simulation is only enforced when extending current main tip.
	for i, tx := range b.Txs {
		if n.cfg.Issuance.Enabled && i == 0 {
			continue
		}
		if tx.Type == "" { tx.Type = "transfer" }
		if tx.Type == "coinbase" { return errors.New("coinbase forbidden in non-first position") }
		// fee sanity (must meet policy)
		isOTAP := false
		for _, o := range tx.Outputs { if o.P != "" && o.R != "" { isOTAP = true; break } }
		sz := fees.TxSizeHint(tx.ID(), len(tx.Outputs))
		est := fees.EstimateFee(sz, isOTAP, fees.Params{GasAsset: n.cfg.Fees.GasAsset, BaseFeeMin: n.cfg.Fees.BaseFeeMin, PerByte: n.cfg.Fees.PerByte, OTAPMultiplier: n.cfg.Fees.OTAPMultiplier, Mode: fees.Mode(n.cfg.Fees.Mode)})
		if tx.Fee < est { return errors.New("tx fee below policy") }

		if tx.From == "" || tx.PubKey == "" || tx.SigHex == "" { return errors.New("missing tx signature") }
		if tx.From != tx.PubKey { return errors.New("from/pub mismatch") }
		if !sig.Verify(tx.PubKey, tx.SigHex, tx.SigningBytes()) { return errors.New("bad tx signature") }

		if tx.Type == "otap_claim" {
			if tx.OTAPClaim == nil || tx.OTAPClaim.P == "" || tx.OTAPClaim.To == "" { return errors.New("bad otap_claim") }
			if tx.OTAPClaim.To != tx.From { return errors.New("claim_to must equal from") }
			if tx.OTAPClaim.ClaimSigHex == "" { return errors.New("missing claim_sig") }
			Ppt := new(edwards25519.Point)
			if _, err := Ppt.SetBytes(mustHex32(tx.OTAPClaim.P)); err != nil { return errors.New("bad claim P") }
			if !schnorr.Verify(Ppt, tx.SigningBytes(), tx.OTAPClaim.ClaimSigHex) { return errors.New("bad claim signature") }
		}
	}
	feeSum := sumFees(b.Txs)
	if len(b.Txs) > 0 && b.Txs[0].Type == "coinbase" {
		cb := b.Txs[0]
		var cbOut int64
		for _, o := range cb.Outputs { cbOut += o.Amount }
		// If issuance configured, coinbase must cover reward + fees
		reward := int64(n.cfg.Issuance.Reward)
		if cbOut < reward+feeSum { return fmt.Errorf("coinbase_missing_fees") }
	}
	return nil
}

// ValidateBlockStateTip runs an in-memory balance/nonce simulation against current DB state.
// Only safe to call when the block extends the current main tip.
func (n *Node) ValidateBlockStateTip(b *chain.Block) error {
	// local working set
	bal := map[string]int64{}
	non := map[string]uint64{}

	getBal := func(id string) int64 {
		if v, ok := bal[id]; ok { return v }
		v, _, _ := state.GetBalance(n.db.DB, id)
		bal[id] = v
		return v
	}
	setBal := func(id string, v int64) { bal[id] = v }
	getNon := func(id string) uint64 {
		if v, ok := non[id]; ok { return v }
		v, _, _ := state.GetNonce(n.db.DB, id)
		non[id] = v
		return v
	}
	setNon := func(id string, v uint64) { non[id] = v }

	// apply coinbase credit first (if present)
	idx := 0
	if n.cfg.Issuance.Enabled {
		cb := b.Txs[0]
		to := cb.Outputs[0].Address
		setBal(to, getBal(to)+cb.Outputs[0].Amount)
		idx = 1
	}

	for ; idx < len(b.Txs); idx++ {
		tx := b.Txs[idx]
		if tx.Type == "" { tx.Type = "transfer" }

		// nonce must be exact next in-chain
		cur := getNon(tx.From)
		if tx.Nonce != cur+1 {
			return errors.New("bad nonce in block") 
		}
		setNon(tx.From, tx.Nonce)

		if tx.Type == "otap_claim" {
			bucket := "otap:" + tx.OTAPClaim.P
			bBal := getBal(bucket)
			need := tx.OTAPClaim.Amount + tx.Fee
			if bBal < need { return errors.New("insufficient otap bucket") }
			setBal(bucket, bBal-need)
			toBal := getBal(tx.OTAPClaim.To)
			setBal(tx.OTAPClaim.To, toBal+tx.OTAPClaim.Amount)
			continue
		}

		// normal transfer
		var outTotal int64
		for _, o := range tx.Outputs { outTotal += o.Amount }
		fromBal := getBal(tx.From)
		if fromBal < outTotal+tx.Fee { return errors.New("insufficient funds") }
		setBal(tx.From, fromBal-(outTotal+tx.Fee))
		for _, o := range tx.Outputs {
			if o.Address != "" {
				setBal(o.Address, getBal(o.Address)+o.Amount)
			} else if o.P != "" {
				setBal("otap:"+o.P, getBal("otap:"+o.P)+o.Amount)
			}
		}
	}
	feeSum := sumFees(b.Txs)
	if len(b.Txs) > 0 && b.Txs[0].Type == "coinbase" {
		cb := b.Txs[0]
		var cbOut int64
		for _, o := range cb.Outputs { cbOut += o.Amount }
		// If issuance configured, coinbase must cover reward + fees
		reward := int64(n.cfg.Issuance.Reward)
		if cbOut < reward+feeSum { return fmt.Errorf("coinbase_missing_fees") }
	}
	return nil
}


func sumFees(txs []chain.Tx) int64 {
	var s int64
	for _, tx := range txs {
		if tx.Type == "coinbase" { continue }
		s += tx.Fee
	}
	return s
}
