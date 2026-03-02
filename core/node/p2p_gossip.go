package node

import (
	"encoding/json"
	"net/url"
	"sync"
	"time"

	"reserveos/core/chain"
	"reserveos/core/consensus/pow"
	"reserveos/internal/reservep2p"
	"reserveos/internal/reservestorage"
)

// small per-peer token bucket limiter
type peerLimiter struct {
	mu sync.Mutex
	m map[string]struct{ tokens float64; last int64 }
	rate float64
	burst float64
}

func newPeerLimiter(rate, burst float64) *peerLimiter {
	return &peerLimiter{m: map[string]struct{ tokens float64; last int64 }{}, rate: rate, burst: burst}
}

func (rl *peerLimiter) allow(peer string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now().UnixNano()
	rec, ok := rl.m[peer]
	if !ok { rec.tokens = rl.burst; rec.last = now }
	dt := float64(now - rec.last) / 1e9
	rec.tokens = minf(rl.burst, rec.tokens + dt*rl.rate)
	rec.last = now
	if rec.tokens < 1.0 {
		rl.m[peer] = rec
		return false
	}
	rec.tokens -= 1.0
	rl.m[peer] = rec
	return true
}

func minf(a,b float64) float64 { if a<b { return a }; return b }

func (n *Node) startP2PGossip() {
	if n.p2p == nil { return }

	lim := newPeerLimiter(30, 60) // 30 msgs/sec per peer

	// simple caches to avoid loops
	seenTx := map[string]time.Time{}
	seenBlk := map[string]time.Time{}
	var seenMu sync.Mutex

	n.p2p.SetHandler(func(peer string, m reservep2p.Msg) {
		if len(n.cfg.P2P.Allowlist) > 0 {
			ok := false
			for _, a := range n.cfg.P2P.Allowlist { if a == peer { ok = true; break } }
			if !ok { return }
		}
		if banned, _ := reservestorage.IsBanned(n.db.DB, peer); banned { return }
		if !lim.allow(peer) { return }

		type headersReq struct{ From uint64 `json:"from"`; Count int `json:"count"`; Locator []string `json:"locator"` }
	type headersResp struct{ From uint64 `json:"from"`; Headers []chain.BlockHeader `json:"headers"`; Hashes []string `json:"hashes"` }

	switch m.Type {
		case "get_hello":
			b, _ := json.Marshal(map[string]any{"ver":"1","chain_id":n.cfg.Chain.ChainID,"genesis_hash":n.cfg.Chain.GenesisHash,"caps":[]string{"headers","blocks","votes"}})
			_ = n.p2p.SendTo(peer, reservep2p.Msg{Type:"hello", Data: b})
		case "hello":
			// best-effort parse; mismatch chain_id => ban
			var h map[string]any
			_ = json.Unmarshal(m.Data, &h)
			if cid, ok := h["chain_id"].(string); ok && cid != "" && cid != n.cfg.Chain.ChainID { n.penalizePeer(peer, -100); return }
			if gh, ok := h["genesis_hash"].(string); ok && n.cfg.Chain.GenesisHash != "" && gh != "" && gh != n.cfg.Chain.GenesisHash { n.penalizePeer(peer, -100); return }

		case "get_anchor":
			finH, ok, _ := reservestorage.GetFinalizedHeight(n.db.DB)
			var hash string
			if ok && finH > 0 {
				var b chain.Block
				okb, _, _ := reservestorage.GetBlockByHeight(n.db.DB, finH, &b)
				if okb { hash = b.Hash().String() }
			}
			payload, _ := json.Marshal(map[string]any{"height": finH, "hash": hash, "set": ok})
			_ = n.p2p.SendTo(peer, reservep2p.Msg{Type:"anchor", Data: payload})
		case "anchor":
			var a struct{ Height uint64 `json:"height"`; Hash string `json:"hash"`; Set bool `json:"set"` }
			if err := json.Unmarshal(m.Data, &a); err != nil { n.penalizePeer(peer, -2); return }
			// compare to local finalized anchor
			lH, lOk, _ := reservestorage.GetFinalizedHeight(n.db.DB)
			lHash := ""
			if lOk && lH > 0 {
				var lb chain.Block
				okb, _, _ := reservestorage.GetBlockByHeight(n.db.DB, lH, &lb)
				if okb { lHash = lb.Hash().String() }
			}
			okMatch := true
			if lOk && lH > 0 {
				okMatch = (a.Set && a.Height == lH && a.Hash == lHash)
			}
			if n.sm != nil { n.sm.setPeerAnchorOK(peer, okMatch) }
			if !okMatch { n.penalizePeer(peer, -10); return }

		// basic per-type size caps
		cap := 1<<20
			switch m.Type {
			case "tx": cap = 200<<10
			case "block":
			finH, finOk, _ := reservestorage.GetFinalizedHeight(n.db.DB)
			if finOk && finH > 0 && n.sm != nil && !n.sm.isPeerAnchorOK(peer) { n.penalizePeer(peer, -5); return } cap = 1<<20
			case "peers": cap = 64<<10
			case "headers":
			finH, finOk, _ := reservestorage.GetFinalizedHeight(n.db.DB)
			if finOk && finH > 0 && n.sm != nil && !n.sm.isPeerAnchorOK(peer) { n.penalizePeer(peer, -5); return } cap = 256<<10
			}
			if len(m.Data) > cap { n.penalizePeer(peer, -10); return }

		case "get_headers":
			var req headersReq
			_ = json.Unmarshal(m.Data, &req)
			if req.Count <= 0 { req.Count = 200 }
			if req.Count > 2000 { req.Count = 2000 }
			start := req.From
			if len(req.Locator) > 0 {
				// find first locator hash we have
				for _, h := range req.Locator {
					var b chain.Block
					okb, _, _ := reservestorage.GetBlockByHash(n.db.DB, h, &b)
					if okb {
						start = b.Header.Height + 1
						break
					}
				}
			}
			req.From = start

			tipH, _, ok, _ := reservestorage.GetTip(n.db.DB)
			if !ok { tipH = 0 }
			end := req.From + uint64(req.Count) - 1
			if end > tipH { end = tipH }
			resp := headersResp{From: req.From, Headers: make([]chain.BlockHeader,0,req.Count), Hashes: make([]string,0,req.Count)}
			for h := req.From; h <= end; h++ {
				var b chain.Block
				okb, _, _ := reservestorage.GetBlockByHeight(n.db.DB, h, &b)
				if !okb { break }
				resp.Headers = append(resp.Headers, b.Header)
				resp.Hashes = append(resp.Hashes, b.Hash().String())
			}
			bb, _ := json.Marshal(resp)
			_ = n.p2p.SendTo(peer, reservep2p.Msg{Type:"headers", Data: bb})
		case "headers":
			finH, finOk, _ := reservestorage.GetFinalizedHeight(n.db.DB)
			if finOk && finH > 0 && n.sm != nil && !n.sm.isPeerAnchorOK(peer) { n.penalizePeer(peer, -5); return }
			var resp struct{ From uint64 `json:"from"`; Headers []any `json:"headers"` }
			_ = json.Unmarshal(m.Data, &resp)
			if n.sm != nil { n.sm.setHeadersCount(len(resp.Headers)); n.sm.setLastHashes(resp.Hashes) }
			_ = reservestorage.AddPeerScore(n.db.DB, peer, 0)

		case "get_peers":
			ps, err := reservestorage.ListPeers(n.db.DB, 2000)
			if err != nil { return }
			b, _ := json.Marshal(ps)
			_ = n.p2p.SendTo(peer, reservep2p.Msg{Type:"peers", Data: b})
		case "peers":
			// best-effort ingest: store peer base URLs (addr)
			var ps []reservestorage.Peer
			if err := json.Unmarshal(m.Data, &ps); err != nil { n.penalizePeer(peer, -2); return }
			for _, p := range ps {
				if p.Addr == "" { continue }
				_ = reservestorage.PutPeer(n.db.DB, p)
			}

		case "inv_tx":
			txid := m.ID
			if txid == "" { return }
			seenMu.Lock()
			if reservestorage.HasSeenTx(n.db.DB, txid) { return }
			_, ok := seenTx[txid]
			if !ok { seenTx[txid] = time.Now(); reservestorage.MarkSeenTx(n.db.DB, txid) }
			seenMu.Unlock()
			if ok { return }
			// request tx
			_ = n.p2p.SendTo(peer, reservep2p.Msg{Type:"get_tx", ID: txid})
		case "get_tx":
			txid := m.ID
			if txid == "" { return }
						// Reply from tx index if present
			tx, ok, _ := reservestorage.GetTx(n.db.DB, txid)
			if ok {
				b, _ := json.Marshal(tx)
				n.p2p.SendTo(peer, reservep2p.Msg{Type:"tx", ID: txid, Data: b})
				return
			}
		case "tx":
			var tx chain.Tx
			if err := json.Unmarshal(m.Data, &tx); err != nil { n.penalizePeer(peer, -2); return }
			if tx.Type == "" { tx.Type = "transfer" }
			if err := n.validateTxAdmission(&tx); err != nil { n.penalizePeer(peer, -5); return }
			if _, err := n.mp.Add(n.db.DB, tx); err != nil { n.penalizePeer(peer, -3); return }
		_ = reservestorage.AddPeerScore(n.db.DB, peer, 1)
			reservestorage.PutMempoolTxWithOrigin(n.db.DB, tx, 3600, peer)
			// forward inv only
			n.p2p.SendAll(reservep2p.Msg{Type:"inv_tx", ID: tx.ID()})
		case "inv_block":
			h := m.Hash
			if h == "" { return }
			seenMu.Lock()
			if reservestorage.HasSeenBlk(n.db.DB, h) { return }
			_, ok := seenBlk[h]
			if !ok { seenBlk[h] = time.Now(); reservestorage.MarkSeenBlk(n.db.DB, h) }
			seenMu.Unlock()
			if ok { return }
			_ = n.p2p.SendTo(peer, reservep2p.Msg{Type:"get_block", Hash: h})
		case "get_block":
			h := m.Hash
			if h == "" { return }
			// reply only if we have block by hash in db
			var b chain.Block
			ok, _, _ := reservestorage.GetBlockByHash(n.db.DB, h, &b)
			if !ok { return }
			bb, _ := json.Marshal(b)
			n.p2p.SendTo(peer, reservep2p.Msg{Type:"block", Hash: h, Data: bb})
		case "block":
			finH, finOk, _ := reservestorage.GetFinalizedHeight(n.db.DB)
			if finOk && finH > 0 && n.sm != nil && !n.sm.isPeerAnchorOK(peer) { n.penalizePeer(peer, -5); return }
			if n.sm != nil { /* best-effort clear by hash if provided */ if m.Hash != "" { n.sm.clearInFlight(m.Hash) } }
			var b chain.Block
			if err := json.Unmarshal(m.Data, &b); err != nil { n.penalizePeer(peer, -2); return }
			if err := n.SubmitBlock(&b); err != nil { if n.sm != nil { n.sm.markFail(peer) }; n.penalizePeer(peer, -10); return }
		_ = reservestorage.AddPeerScore(n.db.DB, peer, 1)
			reservestorage.PutMempoolTxWithOrigin(n.db.DB, tx, 3600, peer)
			// forward inv only
			n.p2p.SendAll(reservep2p.Msg{Type:"inv_block", Hash: b.Hash().String()})
		}
	})

	// dial loop from gossip peers (HTTP URLs) -> tcp :18444
	go func() {
		for {
			for _, p := range n.cfg.Gossip.Peers {
				u, err := url.Parse(p)
				if err != nil { continue }
				host := u.Hostname()
				if host == "" { continue }
				addr := host + ":18444"
				_ = n.p2p.Dial(addr)
			}
			time.Sleep(10 * time.Second)
			// request peers
			n.p2p.SendAll(reservep2p.Msg{Type:"get_peers"})
		}
	}()
}

func (n *Node) p2pBroadcastTx(tx chain.Tx) {
	if n.p2p == nil { return }
	// inv first
	n.p2p.SendAll(reservep2p.Msg{Type:"inv_tx", ID: tx.ID()})
	// body will be requested via get_tx
}

func (n *Node) p2pBroadcastBlock(b chain.Block) {
	if n.p2p == nil { return }
	n.p2p.SendAll(reservep2p.Msg{Type:"inv_block", Hash: b.Hash().String()})
	// body will be requested via get_block
}


func (n *Node) penalizePeer(pub string, delta int64) {
	ps, _ := reservestorage.AddPeerScore(n.db.DB, pub, delta)
	if ps.Score <= -50 {
		_ = reservestorage.BanPeer(n.db.DB, pub, 600)
	}
}





func validateHeadersChain(tipHash string, headers []chain.BlockHeader, hashes []string) bool {
	if len(headers) == 0 { return true }
	if len(hashes) > 0 && len(hashes) != len(headers) { return false }
	// first must connect to tipHash
	if headers[0].PrevHash != tipHash { return false }
	if len(hashes) > 0 {
		for i := 1; i < len(headers); i++ {
			if headers[i].PrevHash != hashes[i-1] { return false }
		}
	}
	return true
}
