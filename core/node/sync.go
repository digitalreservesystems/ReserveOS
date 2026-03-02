package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"reserveos/core/chain"
	"reserveos/core/consensus/pow"
	"reserveos/internal/reservestorage"
)

func (n *Node) startSyncLoop() {
	if !n.cfg.Sync.Enabled || len(n.cfg.Sync.Peers) == 0 {
		return
	}
	interval := time.Duration(n.cfg.Sync.IntervalSeconds) * time.Second
	t := time.NewTicker(interval)
	go func() {
		defer t.Stop()
		for range t.C {
			_ = n.syncOnce()
		}
	}()
}

func (n *Node) syncOnce() error {
	bestPeer := ""
	bestHeight := uint64(0)
	bestTipHash := ""

	for _, p := range n.cfg.Sync.Peers {
		ci, err := fetchChainInfo(p)
		if err != nil { continue }
		if ci.ChainID != n.genesis.ChainID { continue }
		if ci.Height > bestHeight {
			bestHeight = ci.Height
			bestPeer = p
			bestTipHash = ci.Tip
		}
	}
	if bestPeer == "" { return nil }

	localH, _, ok, err := reservestorage.GetTip(n.db.DB)
	if err != nil || !ok { return err }
	if bestHeight <= localH { return nil }

	curWork, okTW, _ := reservestorage.GetTipWork(n.db.DB)
	if !okTW { curWork = 0 }

	// Fast path: headers from tip+1
	maxBlocks := n.cfg.Sync.MaxBlocksPerRound
	start := localH + 1
	need := int(bestHeight - localH)
	if need > maxBlocks { need = maxBlocks }

	_, headers, err := fetchHeaders(bestPeer, start, need)
	if err == nil && len(headers) > 0 {
		hashSeq := make([]string, 0, len(headers))
		var tipCandidateWork uint64
		for _, hdr := range headers {
			h := hdr.Hash()
			if !pow.MeetsTarget(h, hdr.Bits) { break }
			hashHex := h.String()
			hashSeq = append(hashSeq, hashHex)

			prevHex := hdr.PrevHash.String()
			parentWork, okW, err := reservestorage.GetWork(n.db.DB, prevHex)
			if err != nil || !okW { break }
			cum := pow.Cumulative(parentWork, pow.Work64(hdr.Bits))
			_ = reservestorage.PutHeader(n.db.DB, hashHex, hdr)
			_ = reservestorage.PutWork(n.db.DB, hashHex, cum)
			tipCandidateWork = cum
		}
		if tipCandidateWork > curWork && len(hashSeq) > 0 {
			for _, hh := range hashSeq {
				br, err := fetchBlockByHash(bestPeer, hh)
				if err != nil { break }
				b := &chain.Block{Header: br.Block.Header, Txs: br.Block.Txs}
				if err := n.SubmitBlock(b); err != nil { break }
			}
			return nil
		}
	}

	// Locator path: find common ancestor quickly and then request forward headers.
	if bestTipHash == "" { return nil }

	// Enforce finalized anchor agreement (cheap)
	finH, finHash, okFin, err := reservestorage.GetFinalized(n.db.DB)
	if err != nil { return err }
	if !okFin {
		finH = 0
		var gb chain.Block
		okb, ghash, _ := reservestorage.GetBlockByHeight(n.db.DB, 0, &gb)
		if !okb { return errors.New("missing genesis") }
		finHash = ghash
	}
	pb, err := fetchBlockByHeight(bestPeer, finH)
	if err != nil { return err }
	if pb.Hash != finHash { return errors.New("peer finalized anchor mismatch") }

	locator, err := n.BuildBlockLocator(32)
	if err != nil { return err }
	anc, err := postLocate(bestPeer, locator)
	if err != nil || !anc.Found { return nil }
	if anc.Height < finH { return errors.New("ancestor below finalized") }

	from := anc.Height + 1
	count := int(bestHeight - anc.Height)
	if count > maxBlocks { count = maxBlocks }
	_, hdrs, err := fetchHeaders(bestPeer, from, count)
	if err != nil { return err }
	if len(hdrs) == 0 { return nil }

	// Score and fetch by hash
	hashSeq := make([]string, 0, len(hdrs))
	var tipCandidateWork uint64
	for _, hdr := range hdrs {
		h := hdr.Hash()
		if !pow.MeetsTarget(h, hdr.Bits) { break }
		hashHex := h.String()
		hashSeq = append(hashSeq, hashHex)

		prevHex := hdr.PrevHash.String()
		parentWork, okW, err := reservestorage.GetWork(n.db.DB, prevHex)
		if err != nil || !okW { break }
		cum := pow.Cumulative(parentWork, pow.Work64(hdr.Bits))
		_ = reservestorage.PutHeader(n.db.DB, hashHex, hdr)
		_ = reservestorage.PutWork(n.db.DB, hashHex, cum)
		tipCandidateWork = cum
	}
	if tipCandidateWork <= curWork { return nil }

	for _, hh := range hashSeq {
		br, err := fetchBlockByHash(bestPeer, hh)
		if err != nil { return err }
		b := &chain.Block{Header: br.Block.Header, Txs: br.Block.Txs}
		if err := n.SubmitBlock(b); err != nil {
			if rerr := n.reorgFromPeer(bestPeer, bestHeight); rerr == nil { return nil }
			return err
		}
	}
	return nil
}

type locateResp struct {
	Found bool `json:"found"`
	Hash string `json:"hash"`
	Height uint64 `json:"height"`
}

func postLocate(peer string, locator []string) (*locateResp, error) {
	b, _ := json.Marshal(map[string]any{"locator": locator})
	hc := &http.Client{Timeout: 5 * time.Second}
	resp, err := hc.Post(peer+"/chain/locate", "application/json", bytes.NewReader(b))
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, errors.New("bad status") }
	var out locateResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
	return &out, nil
}

type chainInfoResp struct {
	ChainID string `json:"chain_id"`
	Height  uint64 `json:"height"`
	Tip     string `json:"tip"`
	TipWork uint64 `json:"tip_work"`
}

func fetchChainInfo(peer string) (*chainInfoResp, error) {
	hc := &http.Client{Timeout: 3 * time.Second}
	resp, err := hc.Get(peer + "/chain/info")
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, errors.New("bad status") }
	var out chainInfoResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
	return &out, nil
}

func fetchHeaders(peer string, from uint64, count int) (uint64, []chain.BlockHeader, error) {
	hc := &http.Client{Timeout: 5 * time.Second}
	url := peer + "/chain/headers?from=" + itoaU(from) + "&count=" + itoaI(count)
	resp, err := hc.Get(url)
	if err != nil { return from, nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return from, nil, errors.New("bad status") }
	var out struct {
		From uint64 `json:"from"`
		Count int `json:"count"`
		Headers []chain.BlockHeader `json:"headers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return from, nil, err }
	return out.From, out.Headers, nil
}

type blockResp struct {
	Height uint64 `json:"height"`
	Hash   string `json:"hash"`
	Block  chain.Block `json:"block"`
}

func fetchBlockByHeight(peer string, height uint64) (*blockResp, error) {
	hc := &http.Client{Timeout: 8 * time.Second}
	resp, err := hc.Get(peer + "/chain/block?height=" + itoaU(height))
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, errors.New("bad status") }
	var out blockResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
	return &out, nil
}

type blockByHashResp struct {
	Hash  string     `json:"hash"`
	Block chain.Block `json:"block"`
}

func fetchBlockByHash(peer string, hashHex string) (*blockByHashResp, error) {
	hc := &http.Client{Timeout: 10 * time.Second}
	resp, err := hc.Get(peer + "/chain/block_by_hash?hash=" + hashHex)
	if err != nil { return nil, err }
	defer resp.Body.Close()
	if resp.StatusCode != 200 { return nil, errors.New("bad status") }
	var out blockByHashResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil { return nil, err }
	return &out, nil
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

func itoaI(v int) string {
	if v <= 0 { return "0" }
	return itoaU(uint64(v))
}
