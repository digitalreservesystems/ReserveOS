package node

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"

	"reserveos/core/consensus/finality"
	"reserveos/internal/reservestorage"
)

func (n *Node) startFinalityGossip() {
	interval := time.Duration(n.cfg.Finality.GossipIntervalSeconds) * time.Second
	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for range t.C {
			if !n.finalityEnabled { continue }
			_ = n.ensureVotesUpToTip()
			_ = n.pullVotesFromPeers()
		}
	}()
}

func (n *Node) ensureVotesUpToTip() error {
	tipH, _, ok, err := reservestorage.GetTip(n.db.DB)
	if err != nil || !ok { return err }

	finH, _, finOK, _ := reservestorage.GetFinalized(n.db.DB)
	start := uint64(0)
	if finOK { start = finH }

	lastCp := (tipH / n.checkpointInterval) * n.checkpointInterval
	for h := start + n.checkpointInterval; h <= lastCp; h += n.checkpointInterval {
		_ = n.ensureVoteForHeight(h)
	}
	return nil
}

func (n *Node) ensureVoteForHeight(h uint64) error {
	has, err := reservestorage.HasVote(n.db.DB, h, n.localPubHex)
	if err != nil { return err }
	if has { return nil }

	var blk any
	ok, hashHex, err := reservestorage.GetBlockByHeight(n.db.DB, h, &blk)
	if err != nil || !ok { return err }

	cp := finalityCheckpoint{Height: h, HashHex: hashHex}
	_ = n.storeAndBroadcastVote(cp)
	_ = n.tryFinalizeHeight(h, hashHex)
	return nil
}

type finalityCheckpoint struct {
	Height  uint64
	HashHex string
}

func (n *Node) storeAndBroadcastVote(cp finalityCheckpoint) error {
	priv, err := finality.ParsePriv(n.localPrivHex)
	if err != nil { return err }
	sigHex := finality.SignVote(priv, n.genesis.ChainID, cp.Height, cp.HashHex)

	vr := reservestorage.VoteRecord{
		Height: cp.Height, BlockHash: cp.HashHex, ChainID: n.genesis.ChainID,
		PubkeyHex: n.localPubHex, SigHex: sigHex, TimeUnix: time.Now().Unix(),
	}
	_ = reservestorage.PutVote(n.db.DB, vr)

	for _, peer := range n.cfg.Finality.Peers {
		_ = postJSON(peer+"/finality/submit_vote", vr)
	}
	return nil
}

func (n *Node) pullVotesFromPeers() error {
	if len(n.cfg.Finality.Peers) == 0 { return nil }
	tipH, _, ok, err := reservestorage.GetTip(n.db.DB)
	if err != nil || !ok { return err }
	lastCp := (tipH / n.checkpointInterval) * n.checkpointInterval
	start := uint64(0)
	if lastCp >= n.checkpointInterval*3 { start = lastCp - n.checkpointInterval*3 }

	for h := start; h <= lastCp; h += n.checkpointInterval {
		for _, peer := range n.cfg.Finality.Peers {
			votes, err := getVotes(peer, h)
			if err != nil { continue }
			for _, vr := range votes {
				_ = n.ingestVote(vr)
			}
		}
	}
	return nil
}

func (n *Node) ingestVote(vr reservestorage.VoteRecord) error {
	if !n.finalityEnabled { return nil }
	if vr.ChainID != n.genesis.ChainID { return nil }

	valsReg, _ := reservestorage.ListValidators(n.db.DB, 5000)
	useReg := len(valsReg) > 0

	var valPub string
	if useReg {
		for _, v := range valsReg {
			if v.PubkeyHex == vr.PubkeyHex && !v.Slashed { valPub = v.PubkeyHex; break }
		}
	} else {
		for i := range n.validators { if n.validators[i].PubkeyHex == vr.PubkeyHex { valPub = n.validators[i].PubkeyHex; break } }
	}
	if valPub == "" { return nil }

	// verify
	pub, err := finality.ParsePub(vr.PubkeyHex)
	if err != nil { return nil }
	if !finality.VerifyVote(pub, vr.SigHex, vr.ChainID, vr.Height, vr.BlockHash) { return nil }

	
	for i := range n.validators {
		if n.validators[i].PubkeyHex == vr.PubkeyHex {
			val = &n.validators[i]
			break
		}
	}
	if val == nil { return nil }

	pub, err := finality.ParsePub(vr.PubkeyHex)
	if err != nil { return nil }
	if !finality.VerifyVote(pub, vr.SigHex, vr.ChainID, vr.Height, vr.BlockHash) { return nil }

	has, err := reservestorage.HasVote(n.db.DB, vr.Height, vr.PubkeyHex)
	if err != nil { return err }
	if !has {
		_ = reservestorage.PutVote(n.db.DB, vr)
	}
	_ = n.tryFinalizeHeight(vr.Height, vr.BlockHash)
	return nil
}

func postJSON(url string, obj any) error {
	b, _ := json.Marshal(obj)
	hc := &http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Post(url, "application/json", bytes.NewReader(b))
	if err != nil { return err }
	resp.Body.Close()
	return nil
}

func getVotes(peer string, height uint64) ([]reservestorage.VoteRecord, error) {
	hc := &http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Get(peer + "/finality/votes?height=" + itoaU(height))
	if err != nil { return nil, err }
	defer resp.Body.Close()
	var out struct {
		Height uint64 `json:"height"`
		Votes  []reservestorage.VoteRecord `json:"votes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
	return out.Votes, nil
}

func itoaU(v uint64) string {
	if v == 0 { return "0" }
	buf := make([]byte, 0, 20)
	for v > 0 {
		d := byte(v % 10)
		buf = append(buf, '0'+d)
		v /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
